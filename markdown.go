package page

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"

	"github.com/mnehpets/http/endpoint"
	"github.com/yuin/goldmark"
	gmeta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
)

type markdownPage struct {
	urlPath  string
	meta     Meta
	fsys     fs.FS
	filePath string
}

func (p *markdownPage) URLPath() string { return p.urlPath }
func (p *markdownPage) Meta() Meta      { return p.meta }

func (p *markdownPage) Renderer(site Site, layout *Layout) (endpoint.Renderer, error) {
	if layout == nil {
		return nil, fmt.Errorf("page: layout is nil")
	}

	src, err := p.source()
	if err != nil {
		return nil, fmt.Errorf("page: read %s: %w", p.urlPath, err)
	}

	// Convert markdown → HTML eagerly. goldmark-meta strips front matter from output.
	md := goldmark.New(goldmark.WithExtensions(gmeta.Meta))
	ctx := parser.NewContext()
	var buf bytes.Buffer
	if err := md.Convert(src, &buf, parser.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("page: render markdown %s: %w", p.urlPath, err)
	}

	content := template.HTML(buf.String())
	chain := layoutChain(p.meta)
	var cfg SiteConfig
	if site != nil {
		cfg = site.Config()
	}
	meta, jsonld, tmpl := p.meta, metaToJSONLD(p.meta), layout.Template()

	return endpoint.RendererFunc(func(w http.ResponseWriter, r *http.Request) error {
		ctx := RenderContext{Content: content, JSONLD: jsonld, Config: cfg, Meta: meta, Site: site, Request: r}
		return renderChain(w, r, tmpl, chain, ctx)
	}), nil
}

func (p *markdownPage) source() ([]byte, error) {
	f, err := p.fsys.Open(p.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// newMarkdownPageFromFS creates a markdownPage backed by the given FS.
// Metadata is parsed immediately; the body is re-read from the FS at render time.
func newMarkdownPageFromFS(urlPath string, fsys fs.FS, filePath string) (*markdownPage, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	meta, _, err := ParseFrontMatter(f)
	if err != nil {
		return nil, fmt.Errorf("page: parse %s: %w", filePath, err)
	}

	applyFSFallbacks(&meta, info)
	if meta.Slug == "" {
		meta.Slug = deriveSlug(urlPath)
	}
	return &markdownPage{urlPath: urlPath, meta: meta, fsys: fsys, filePath: filePath}, nil
}

func applyFSFallbacks(m *Meta, info fs.FileInfo) {
	if m.Date.IsZero() {
		m.Date = info.ModTime()
	}
}
