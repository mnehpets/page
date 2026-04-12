package page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

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
	"sortByDate":    SortByDate,
	"sortByLastMod": SortByLastMod,
	// hasTag reports whether m has the given tag. Use the noindex tag to
	// exclude a page from sitemaps and other automated indexes.
	"hasTag": func(m Meta, tag string) bool {
		for _, t := range m.Tags {
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
	// sortByPath returns a new []Page sorted lexicographically by SitePath.
	"sortByPath": SortByPath,
	// parentPath returns the parent path of p in the site-relative scheme.
	// parentPath("a/b/c.md") → "a/b"
	// parentPath("a/b")      → "a"
	// parentPath("a.md")     → "."
	// parentPath(".")        → ""
	// Pure string operation; does not consult the site index.
	"parentPath": parentPath,
}

// Layout holds one compiled template set per named layout. Each set combines
// the common base templates (from _layouts/base/) with a single entry-point
// template (from _layouts/). The execution entry point is always the template
// named "entry":
//
//   - With base templates: "entry" is defined in the base file and calls
//     {{block "main" .}}; the entry-point overrides that block with {{define "main"}}.
//   - Without base templates: "entry" is defined directly in the entry-point file,
//     which owns its full output with no shared outer shell.
//
// An entry-point may select a different entry-point name via {{define "entryname"}}.
type Layout struct {
	templates map[string]*template.Template // layout name → template set
}

// resolveLayout returns the compiled template set for name (falling back to
// "default") and the template name to pass to ExecuteTemplate. The entry-point
// name is always "entry" unless the layout defines an "entryname" template
// whose body names a different template. Returns (nil, "") when neither name
// nor "default" is registered.
func (l *Layout) resolveLayout(name string) (*template.Template, string) {
	tmpl, ok := l.templates[name]
	if !ok {
		tmpl = l.templates["default"]
	}
	if tmpl == nil {
		return nil, ""
	}
	if t := tmpl.Lookup("entryname"); t != nil {
		var buf bytes.Buffer
		if err := t.Execute(&buf, nil); err == nil {
			if s := strings.TrimSpace(buf.String()); s != "" {
				return tmpl, s
			}
		}
	}
	return tmpl, "entry"
}

// renderer returns a renderer that executes the named layout from l.
// All template execution is buffered so that errors are caught before any
// bytes are committed to the response.
//
// Content-Type is taken from ctx.Meta.ContentType when set, defaulting to
// "text/html; charset=utf-8". It is only written if not already present on
// the response header.
//
// The incoming *http.Request is injected into ctx at Render time so that
// templates can access the live request via .Request.
func (l *Layout) renderer(name string, ctx RenderContext) endpoint.Renderer {
	return endpoint.RendererFunc(func(w http.ResponseWriter, r *http.Request) error {
		if l == nil {
			return fmt.Errorf("page: layout is nil")
		}
		tmpl, entry := l.resolveLayout(name)
		if tmpl == nil {
			return fmt.Errorf("page: layout %q not found", name)
		}

		renderCtx := ctx
		renderCtx.Request = r

		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, entry, renderCtx); err != nil {
			return err
		}

		ct := renderCtx.Meta.ContentType
		if ct == "" {
			ct = "text/html; charset=utf-8"
		}
		if w.Header().Get("Content-Type") == "" || renderCtx.Meta.ContentType != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(http.StatusOK)
		_, err := io.Copy(w, &buf)
		return err
	})
}

// NewLayout builds a Layout from fsys.
//
// basePatterns are glob patterns (fs.Glob syntax) matching the common base
// templates. These files are parsed into every layout's template set. Base
// templates should define an "entry" template that calls {{block "main" .}}
// to delegate to the layout-specific content.
//
// layoutPatterns are glob patterns matching the entry-point templates. Each
// matched file becomes one named layout; the name is derived from the
// filename without extension (e.g. "_layouts/container.html" → "container").
// Entry-point templates should provide {{define "main"}} to override the base
// block (when base templates are present), or {{define "entry"}} to own their
// full output (when used standalone). A layout may override the entry-point
// name by defining {{define "entryname"}}other-entry{{end}}.
//
// Returns an error if no layout files match layoutPatterns.
func NewLayout(fsys fs.FS, basePatterns []string, layoutPatterns []string) (*Layout, error) {
	baseFiles, err := expandGlobs(fsys, basePatterns)
	if err != nil {
		return nil, err
	}

	layoutFiles, err := expandGlobs(fsys, layoutPatterns)
	if err != nil {
		return nil, err
	}
	if len(layoutFiles) == 0 {
		return nil, fmt.Errorf("page: NewLayout: no layout files matched patterns %v", layoutPatterns)
	}

	templates := make(map[string]*template.Template, len(layoutFiles))
	for _, lf := range layoutFiles {
		// Create a template set for this layout by combining the base templates with the entry-point template.
		name := layoutNameFromPath(lf)
		filesToParse := make([]string, 0, len(baseFiles)+1)
		filesToParse = append(filesToParse, baseFiles...)
		filesToParse = append(filesToParse, lf)

		tmpl, err := template.New("").Funcs(builtinFuncs).ParseFS(fsys, filesToParse...)
		if err != nil {
			return nil, fmt.Errorf("page: parse layout %q: %w", lf, err)
		}
		templates[name] = tmpl
	}

	return &Layout{templates: templates}, nil
}

// expandGlobs expands a list of glob patterns into deduplicated file paths.
func expandGlobs(fsys fs.FS, patterns []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	for _, p := range patterns {
		matches, err := fs.Glob(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("page: glob %q: %w", p, err)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				files = append(files, m)
			}
		}
	}
	return files, nil
}

// layoutNameFromPath derives the layout name from a template file path.
// The name is the base filename with its extension stripped.
//
//	"_layouts/container.html" → "container"
//	"tmpl.html"               → "tmpl"
func layoutNameFromPath(filePath string) string {
	base := path.Base(filePath)
	if ext := path.Ext(base); ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

// layoutName returns the resolved layout name for m, defaulting to "default".
func layoutName(m Meta) string {
	if m.Layout != "" {
		return m.Layout
	}
	return "default"
}
