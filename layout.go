package page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/mnehpets/http/endpoint"
)

// builtinFuncs are available in all layout templates.
//
//   - json:        marshals any value to a JSON literal (returned as template.JS
//     so html/template does not double-escape it).
//   - sortByDate:  wraps SortByDate for use in range pipelines.
var builtinFuncs = template.FuncMap{
	"json": func(v any) (template.JS, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return template.JS(b), nil
	},
	"sortByDate": SortByDate,
	// hasTag reports whether p has the given tag. Use the noindex tag to
	// exclude a page from sitemaps and other automated indexes.
	"hasTag": func(p Page, tag string) bool {
		for _, t := range p.Meta().Tags {
			if t == tag {
				return true
			}
		}
		return false
	},
	// safeHTML marks s as safe HTML, bypassing html/template's contextual
	// escaping. Use only with trusted, statically-known strings — never with
	// user-supplied content.
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	// sortByPath returns a new []Page sorted lexicographically by URLPath.
	"sortByPath": SortByPath,
	// parentPath returns the parent URL path of urlPath.
	// parentPath("/a/b/") → "/a/"
	// parentPath("/a/")   → "/"
	// parentPath("/")     → ""
	// Pure string operation; does not consult the site index.
	"parentPath": parentURLPath,
}

// Layout wraps a parsed html/template set containing one or more named layout
// templates. Template names are determined by {{define "name"}} declarations
// within the template files.
type Layout struct {
	tmpl *template.Template
}

// NewLayout parses all files from fsys that match any of the given glob
// patterns and returns a Layout. Template names come from {{define}}
// declarations inside the files. Returns an error if no files match.
func NewLayout(fsys fs.FS, patterns ...string) (*Layout, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("page: NewLayout requires at least one pattern")
	}

	var files []string
	for _, p := range patterns {
		matches, err := fs.Glob(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("page: glob %q: %w", p, err)
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("page: no template files matched patterns %v", patterns)
	}

	tmpl, err := template.New("").Funcs(builtinFuncs).ParseFS(fsys, files...)
	if err != nil {
		return nil, fmt.Errorf("page: parse layout templates: %w", err)
	}

	return &Layout{tmpl: tmpl}, nil
}

// Template returns the underlying html/template set.
func (l *Layout) Template() *template.Template { return l.tmpl }

// layoutChain returns the rendering pipeline for m. Each name except the last
// renders into Content for the next; the last template renders to the response.
// Defaults to ["default"] when no layouts are declared.
func layoutChain(m Meta) []string {
	if len(m.Layouts) == 0 {
		return []string{"default"}
	}
	return m.Layouts
}

// renderChain executes the templates named by chain in order. Each template
// except the last renders into a buffer; its output becomes RenderContext.Content
// for the next step. Only Content changes between steps — Meta, Head, JSONLD,
// Site, and Request are unchanged throughout. The final template is rendered to
// the HTTP response via endpoint.HTMLTemplateRenderer.
func renderChain(w http.ResponseWriter, r *http.Request, tmpl *template.Template, chain []string, ctx RenderContext) error {
	for _, name := range chain[:len(chain)-1] {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, name, ctx); err != nil {
			return err
		}
		ctx.Content = template.HTML(buf.String())
	}
	final := chain[len(chain)-1]
	if ct := ctx.Meta.ContentType; ct != "" {
		// HTMLTemplateRenderer defaults to text/html; bypass it when the page
		// declares a different content type (e.g. application/xml for sitemaps).
		w.Header().Set("Content-Type", ct)
		return tmpl.ExecuteTemplate(w, final, ctx)
	}
	return (&endpoint.HTMLTemplateRenderer{
		Template: tmpl,
		Name:     final,
		Values:   ctx,
	}).Render(w, r)
}
