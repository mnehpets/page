package page

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/mnehpets/http/endpoint"
)

// Page is implemented by content files that can be rendered through a Layout.
// The primary responsibility of a Page is rendering its content; SitePath and
// Meta are secondary, supporting the Site index and template queries.
type Page interface {
	SitePath() string
	Meta() Meta
	Renderer(site *site, layout *Layout) (endpoint.Renderer, error)
}

// Meta holds the parsed metadata for a page.
type Meta struct {
	Title       string
	Author      string
	Date        time.Time
	Tags        []string
	Collection  string
	Layouts     []string // rendering pipeline: each template renders into Content for the next
	Draft       bool
	Description string
	Image       Image
	Slug        string
	LinkTitle   string    // short title for navigation links; falls back to Title when empty
	ContentType string    // MIME type for the HTTP response, e.g. "application/xml"; defaults to text/html
	LastMod     time.Time // last-modified time; set from lastmod frontmatter, falls back to file mod time
}

// Image holds an image reference for use in Meta.
type Image struct {
	URL string
	Alt string
}

// RenderContext is the data passed to the layout template when rendering a page.
type RenderContext struct {
	Content template.HTML
	Head    template.HTML
	JSONLD  template.JS // schema.org JSON-LD blob, ready to drop into <script type="application/ld+json">
	Config  SiteConfig
	Meta    Meta
	SitePath string // site-relative path of the current page, e.g. "blog/hello.md" or "." for root
	Site    *site
	Request *http.Request
}

// Href returns a relative URL from the current page to target. Target may be a
// Page, a RenderContext (or *RenderContext), or a site-relative path string.
func (c RenderContext) Href(target any) (string, error) {
	p, err := sitePathFromTarget(target)
	if err != nil {
		return "", err
	}
	return relURL(c.SitePath, p), nil
}

// AbsURL returns the canonical absolute URL for target. Target may be a Page,
// a RenderContext (or *RenderContext), or a site-relative path string.
func (c RenderContext) AbsURL(target any) (string, error) {
	p, err := sitePathFromTarget(target)
	if err != nil {
		return "", err
	}
	return absURL(c.Config.BaseURL, p), nil
}

func sitePathFromTarget(target any) (string, error) {
	switch t := target.(type) {
	case nil:
		return "", fmt.Errorf("page: link target is nil")
	case string:
		return t, nil
	case Page:
		return t.SitePath(), nil
	case RenderContext:
		return t.SitePath, nil
	case *RenderContext:
		if t == nil {
			return "", fmt.Errorf("page: link target is nil")
		}
		return t.SitePath, nil
	default:
		return "", fmt.Errorf("page: unsupported link target type %T", target)
	}
}
