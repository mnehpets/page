package page

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"

	"github.com/mnehpets/http/endpoint"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type markdownPage struct {
	sitePath string
	meta     Meta
	fsys     fs.FS
	filePath string
}

func (p *markdownPage) SitePath() string { return p.sitePath }
func (p *markdownPage) Meta() Meta       { return p.meta }

func (p *markdownPage) Renderer(site *site, layout *Layout) (endpoint.Renderer, error) {
	src, err := p.source()
	if err != nil {
		return nil, fmt.Errorf("page: read %s: %w", p.sitePath, err)
	}

	var buf bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := md.Convert(src, &buf); err != nil {
		return nil, fmt.Errorf("page: render markdown %s: %w", p.sitePath, err)
	}

	content := template.HTML(buf.String())
	var cfg SiteConfig
	if site != nil {
		cfg = site.Config()
	}
	meta, jsonld := p.meta, metaToJSONLD(p.meta)
	ctx := RenderContext{Content: content, JSONLD: jsonld, Config: cfg, Meta: meta, SitePath: p.sitePath, Site: site}
	return layout.renderer(layoutName(meta), ctx), nil
}

func (p *markdownPage) source() ([]byte, error) {
	f, err := p.fsys.Open(p.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	_, body, _ := extractFrontMatterBlock(data)
	return body, nil
}

// newMarkdownPageFromFS creates a markdownPage backed by the given FS.
// Metadata is parsed immediately; the body is re-read from the FS at render time.
func newMarkdownPageFromFS(sitePath string, fsys fs.FS, filePath string) (*markdownPage, error) {
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
		meta.Slug = deriveSlug(sitePath)
	}
	return &markdownPage{sitePath: sitePath, meta: meta, fsys: fsys, filePath: filePath}, nil
}

func applyFSFallbacks(m *Meta, info fs.FileInfo) {
	if m.Date.IsZero() {
		m.Date = info.ModTime()
	}
	if m.LastMod.IsZero() {
		m.LastMod = info.ModTime()
	}
}
