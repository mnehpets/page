package page

import (
	"html/template"
	"net/http"
	"time"

	"github.com/mnehpets/http/endpoint"
)

// Page is implemented by content files that can be rendered through a Layout.
// The primary responsibility of a Page is rendering its content; URLPath and
// Meta are secondary, supporting the Site index and template queries.
type Page interface {
	URLPath() string
	Meta() Meta
	Renderer(site Site, layout *Layout) (endpoint.Renderer, error)
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
	Site    Site
	Request *http.Request
}
