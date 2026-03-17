package page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/mnehpets/http/endpoint"
	"golang.org/x/net/html"
)

type htmlPage struct {
	urlPath  string
	meta     Meta
	fsys     fs.FS
	filePath string
}

func (p *htmlPage) URLPath() string { return p.urlPath }
func (p *htmlPage) Meta() Meta      { return p.meta }

func (p *htmlPage) Renderer(site Site, layout *Layout) (endpoint.Renderer, error) {
	if layout == nil {
		return nil, fmt.Errorf("page: layout is nil")
	}

	raw, err := p.source()
	if err != nil {
		return nil, fmt.Errorf("page: read %s: %w", p.urlPath, err)
	}

	_, jsonld, head, body, err := parseHTMLDocument(raw, nil)
	if err != nil {
		return nil, fmt.Errorf("page: parse html %s: %w", p.urlPath, err)
	}

	chain := layoutChain(p.meta)
	var cfg SiteConfig
	if site != nil {
		cfg = site.Config()
	}
	meta, tmpl := p.meta, layout.Template()

	return endpoint.RendererFunc(func(w http.ResponseWriter, r *http.Request) error {
		ctx := RenderContext{Content: body, Head: head, JSONLD: jsonld, Config: cfg, Meta: meta, Site: site, Request: r}
		return renderChain(w, r, tmpl, chain, ctx)
	}), nil
}

func (p *htmlPage) source() ([]byte, error) {
	f, err := p.fsys.Open(p.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// newHTMLPageFromFS creates an htmlPage backed by the FS.
func newHTMLPageFromFS(urlPath string, fsys fs.FS, filePath string) (*htmlPage, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	meta, _, _, _, err := parseHTMLDocument(data, info)
	if err != nil {
		return nil, fmt.Errorf("page: parse %s: %w", filePath, err)
	}
	if len(meta.Layouts) == 0 {
		return nil, nil // no layout declared — serve as static file
	}
	if meta.Slug == "" {
		meta.Slug = deriveSlug(urlPath)
	}
	return &htmlPage{urlPath: urlPath, meta: meta, fsys: fsys, filePath: filePath}, nil
}

// parseHTMLDocument parses raw HTML and returns (Meta, jsonld, headHTML, bodyHTML, error).
// jsonld is the cleaned JSON-LD blob (site-private fields stripped) ready for a
// <script type="application/ld+json"> tag, or "" if the document had none.
// headHTML contains the inner HTML of <head> excluding elements captured into
// Meta (JSON-LD script, <title>, and recognised <meta> tags).
// bodyHTML contains the inner HTML of <body>.
// If info is non-nil it is used as the FS fallback for Date.
func parseHTMLDocument(data []byte, info fs.FileInfo) (Meta, template.JS, template.HTML, template.HTML, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return Meta{}, "", "", "", err
	}

	var headNode, bodyNode *html.Node
	walkNodes(doc, func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "head":
				if headNode == nil {
					headNode = n
				}
			case "body":
				if bodyNode == nil {
					bodyNode = n
				}
			}
		}
	})

	var (
		jsonLD      map[string]any
		metaTags    = make(map[string]string)
		titleText   string
		capturedSet = make(map[*html.Node]bool)
	)

	if headNode != nil {
		for n := headNode.FirstChild; n != nil; n = n.NextSibling {
			if n.Type != html.ElementNode {
				continue
			}
			switch n.Data {
			case "script":
				if nodeAttr(n, "type") == "application/ld+json" {
					text := nodeText(n)
					if err := json.Unmarshal([]byte(text), &jsonLD); err != nil {
						return Meta{}, "", "", "", fmt.Errorf("page: parse JSON-LD: %w", err)
					}
					capturedSet[n] = true
				}
			case "title":
				titleText = nodeText(n)
				capturedSet[n] = true
			case "meta":
				name := nodeAttr(n, "name")
				prop := nodeAttr(n, "property")
				content := nodeAttr(n, "content")
				key := name
				if key == "" {
					key = prop
				}
				if key != "" && content != "" {
					metaTags[key] = content
					if isCapturableMetaTag(key) {
						capturedSet[n] = true
					}
				}
			}
		}
	}

	meta := buildHTMLMeta(jsonLD, metaTags, titleText, info)
	jsonld := cleanHTMLJSONLD(jsonLD)

	// Build head HTML excluding captured nodes.
	var headBuf bytes.Buffer
	if headNode != nil {
		for n := headNode.FirstChild; n != nil; n = n.NextSibling {
			if !capturedSet[n] {
				if err := html.Render(&headBuf, n); err != nil {
					return Meta{}, "", "", "", err
				}
			}
		}
	}

	// Build body inner HTML.
	var bodyBuf bytes.Buffer
	if bodyNode != nil {
		for n := bodyNode.FirstChild; n != nil; n = n.NextSibling {
			if err := html.Render(&bodyBuf, n); err != nil {
				return Meta{}, "", "", "", err
			}
		}
	}

	return meta, jsonld, template.HTML(headBuf.String()), template.HTML(bodyBuf.String()), nil
}

// cleanHTMLJSONLD strips the site-private "site" key from a parsed JSON-LD map
// and simplifies @context if it was a namespaced map. Returns "" if raw is nil.
func cleanHTMLJSONLD(raw map[string]any) template.JS {
	if len(raw) == 0 {
		return ""
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "site" {
			continue
		}
		out[k] = v
	}
	// {"@vocab": "https://schema.org/", "site": "..."} → "https://schema.org/"
	if ctx, ok := out["@context"].(map[string]any); ok {
		cleaned := make(map[string]any, len(ctx))
		for k, v := range ctx {
			if k != "site" {
				cleaned[k] = v
			}
		}
		if vocab, ok := cleaned["@vocab"].(string); ok && len(cleaned) == 1 {
			out["@context"] = vocab
		} else if len(cleaned) > 0 {
			out["@context"] = cleaned
		} else {
			delete(out, "@context")
		}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return ""
	}
	return template.JS(b)
}

// metaToJSONLD builds a schema.org JSON-LD blob from a Meta value.
// Used by markdown pages, which have no source JSON-LD to pass through.
// Returns "" if Meta has no meaningful fields to publish.
func metaToJSONLD(m Meta) template.JS {
	out := map[string]any{
		"@context": "https://schema.org/",
	}
	if m.Title != "" {
		out["name"] = m.Title
	}
	if m.Author != "" {
		out["author"] = m.Author
	}
	if m.Description != "" {
		out["description"] = m.Description
	}
	if len(m.Tags) > 0 {
		out["keywords"] = m.Tags
	}
	if !m.Date.IsZero() {
		out["datePublished"] = m.Date.Format("2006-01-02")
	}
	if m.Image.URL != "" {
		if m.Image.Alt != "" {
			out["image"] = map[string]any{"url": m.Image.URL, "alt": m.Image.Alt}
		} else {
			out["image"] = m.Image.URL
		}
	}
	if len(out) <= 1 {
		return "" // nothing beyond @context
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return template.JS(b)
}

func buildHTMLMeta(jsonLD map[string]any, metaTags map[string]string, titleText string, info fs.FileInfo) Meta {
	var meta Meta

	// Tier 1: JSON-LD.
	if jsonLD != nil {
		// Schema.org fields at top level.
		meta.Title, _ = jsonLD["name"].(string)
		meta.Description, _ = jsonLD["description"].(string)

		// author: string or {"name": string}
		switch a := jsonLD["author"].(type) {
		case string:
			meta.Author = a
		case map[string]any:
			meta.Author, _ = a["name"].(string)
		}

		// keywords: comma-separated string or array
		switch kw := jsonLD["keywords"].(type) {
		case string:
			meta.Tags = splitKeywords(kw)
		case []any:
			for _, k := range kw {
				if s, ok := k.(string); ok {
					meta.Tags = append(meta.Tags, s)
				}
			}
		}

		// image: string or {url, alt}
		switch img := jsonLD["image"].(type) {
		case string:
			meta.Image = Image{URL: img}
		case map[string]any:
			url, _ := img["url"].(string)
			alt, _ := img["alt"].(string)
			meta.Image = Image{URL: url, Alt: alt}
		}

		// date: datePublished preferred, dateModified as fallback
		for _, key := range []string{"datePublished", "dateModified"} {
			if s, ok := jsonLD[key].(string); ok {
				if t := parseHTMLDate(s); !t.IsZero() {
					meta.Date = t
					break
				}
			}
		}

		// Site-specific fields in the "site" namespace object.
		if siteLD, ok := jsonLD["site"].(map[string]any); ok {
			// layout: "name"  or  layouts: ["a", "b"]
			if s, ok := siteLD["layout"].(string); ok && s != "" {
				meta.Layouts = []string{s}
			}
			if seq, ok := siteLD["layouts"].([]any); ok {
				for _, v := range seq {
					if s, ok := v.(string); ok && s != "" {
						meta.Layouts = append(meta.Layouts, s)
					}
				}
			}
			meta.Collection, _ = siteLD["collection"].(string)
			meta.Slug, _ = siteLD["slug"].(string)
			meta.Draft, _ = siteLD["draft"].(bool)
		}
	}

	// Tier 2: HTML <meta> tags and <title>.
	if meta.Title == "" {
		if v := metaTags["og:title"]; v != "" {
			meta.Title = v
		} else if v := metaTags["title"]; v != "" {
			meta.Title = v
		} else {
			meta.Title = titleText
		}
	}
	if meta.Author == "" {
		meta.Author = metaTags["author"]
	}
	if meta.Description == "" {
		if v := metaTags["og:description"]; v != "" {
			meta.Description = v
		} else {
			meta.Description = metaTags["description"]
		}
	}
	if len(meta.Tags) == 0 {
		if v := metaTags["keywords"]; v != "" {
			meta.Tags = splitKeywords(v)
		}
	}
	if meta.Image.URL == "" {
		meta.Image.URL = metaTags["og:image"]
		if meta.Image.URL != "" {
			meta.Image.Alt = metaTags["og:image:alt"]
		}
	}

	// Tier 3: FS metadata.
	if meta.Date.IsZero() && info != nil {
		meta.Date = info.ModTime()
	}

	return meta
}

func isCapturableMetaTag(key string) bool {
	switch key {
	case "title", "description", "author", "keywords",
		"og:title", "og:description", "og:image", "og:image:alt":
		return true
	}
	return false
}

func nodeAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *html.Node) string {
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			buf.WriteString(c.Data)
		}
	}
	return buf.String()
}

func walkNodes(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNodes(c, fn)
	}
}

func parseHTMLDate(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

func splitKeywords(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
