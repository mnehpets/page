package page

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// --- NewLayout ---

func TestNewLayout_ParsesTemplates(t *testing.T) {
	fsys := fstest.MapFS{
		"_layouts/default.html": {Data: []byte(`{{define "entry"}}<html>{{.Content}}</html>{{end}}`)},
		"_layouts/post.html":    {Data: []byte(`{{define "entry"}}<article>{{.Content}}</article>{{end}}`)},
	}

	l, err := NewLayout(fsys, nil, []string{"_layouts/*.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	// "default" layout: standalone, executes "entry" directly.
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	if err := l.renderer("default", RenderContext{Content: "hello"}).Render(rr, req); err != nil {
		t.Fatalf("renderer default: %v", err)
	}
	if got := rr.Body.String(); got != "<html>hello</html>" {
		t.Errorf("default output = %q", got)
	}

	// "post" layout.
	rr = httptest.NewRecorder()
	if err := l.renderer("post", RenderContext{Content: "world"}).Render(rr, req); err != nil {
		t.Fatalf("renderer post: %v", err)
	}
	if got := rr.Body.String(); got != "<article>world</article>" {
		t.Errorf("post output = %q", got)
	}
}

func TestNewLayout_BlockInheritance(t *testing.T) {
	fsys := fstest.MapFS{
		"_layouts/base/entry.html": {Data: []byte(`{{define "entry"}}<html><body>{{block "main" .}}{{end}}</body></html>{{end}}`)},
		"_layouts/default.html":    {Data: []byte(`{{define "main"}}{{.Content}}{{end}}`)},
		"_layouts/container.html":  {Data: []byte(`{{define "main"}}<div class="container">{{.Content}}</div>{{end}}`)},
	}

	l, err := NewLayout(fsys, []string{"_layouts/base/*"}, []string{"_layouts/default.html", "_layouts/container.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	// "default" via entry → main block renders bare content.
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	if err := l.renderer("default", RenderContext{Content: "hello"}).Render(rr, req); err != nil {
		t.Fatalf("renderer default: %v", err)
	}
	if got, want := rr.Body.String(), "<html><body>hello</body></html>"; got != want {
		t.Errorf("default output = %q, want %q", got, want)
	}

	// "container" via entry → main block wraps content in div.
	rr = httptest.NewRecorder()
	if err := l.renderer("container", RenderContext{Content: "hello"}).Render(rr, req); err != nil {
		t.Fatalf("renderer container: %v", err)
	}
	if got, want := rr.Body.String(), `<html><body><div class="container">hello</div></body></html>`; got != want {
		t.Errorf("container output = %q, want %q", got, want)
	}
}

func TestNewLayout_CustomEntryname(t *testing.T) {
	fsys := fstest.MapFS{
		"_layouts/base/entry.html":      {Data: []byte(`{{define "entry"}}<html><body>{{block "main" .}}{{end}}</body></html>{{end}}`)},
		"_layouts/base/wide-entry.html": {Data: []byte(`{{define "wide-entry"}}<div class="wide">{{block "main" .}}{{end}}</div>{{end}}`)},
		"_layouts/default.html":         {Data: []byte(`{{define "main"}}{{.Content}}{{end}}`)},
		"_layouts/wide.html":            {Data: []byte(`{{define "entryname"}}wide-entry{{end}}{{define "main"}}<main>{{.Content}}</main>{{end}}`)},
	}

	l, err := NewLayout(fsys, []string{"_layouts/base/*.html"}, []string{"_layouts/default.html", "_layouts/wide.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	// "default" uses "entry".
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	if err := l.renderer("default", RenderContext{Content: "hello"}).Render(rr, req); err != nil {
		t.Fatalf("renderer default: %v", err)
	}
	if got, want := rr.Body.String(), "<html><body>hello</body></html>"; got != want {
		t.Errorf("default output = %q, want %q", got, want)
	}

	// "wide" uses "wide-entry" via entryname.
	rr = httptest.NewRecorder()
	if err := l.renderer("wide", RenderContext{Content: "hello"}).Render(rr, req); err != nil {
		t.Fatalf("renderer wide: %v", err)
	}
	if got, want := rr.Body.String(), `<div class="wide"><main>hello</main></div>`; got != want {
		t.Errorf("wide output = %q, want %q", got, want)
	}
}

func TestNewLayout_ResolveLayoutFallsBackToDefault(t *testing.T) {
	fsys := fstest.MapFS{
		"_layouts/default.html": {Data: []byte(`{{define "entry"}}ok{{end}}`)},
	}
	l, err := NewLayout(fsys, nil, []string{"_layouts/default.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	tmpl, _ := l.resolveLayout("nonexistent")
	defaultTmpl, _ := l.resolveLayout("default")
	if tmpl != defaultTmpl {
		t.Error("resolveLayout nonexistent should fall back to default")
	}
}

func TestNewLayout_ResolveLayoutReturnsNilWhenNoDefault(t *testing.T) {
	fsys := fstest.MapFS{
		"_layouts/custom.html": {Data: []byte(`{{define "entry"}}ok{{end}}`)},
	}
	l, err := NewLayout(fsys, nil, []string{"_layouts/custom.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	tmpl, _ := l.resolveLayout("nonexistent")
	if tmpl != nil {
		t.Error("resolveLayout should return nil when neither name nor default exists")
	}
}

func TestNewLayout_NoLayoutFilesReturnsError(t *testing.T) {
	fsys := fstest.MapFS{}
	if _, err := NewLayout(fsys, nil, []string{"*.html"}); err == nil {
		t.Error("expected error when no layout files match")
	}
}

func TestNewLayout_NilLayoutPatternsReturnsError(t *testing.T) {
	fsys := fstest.MapFS{}
	if _, err := NewLayout(fsys, nil, nil); err == nil {
		t.Error("expected error when layout patterns is nil")
	}
}

func TestNewLayout_ParseError(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.html": {Data: []byte(`{{define "x"}}{{.Unclosed`)},
	}
	if _, err := NewLayout(fsys, nil, []string{"bad.html"}); err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestNewLayout_UnknownTemplateNameReturnsError(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{define "other"}}ok{{end}}`)},
	}
	l, err := NewLayout(fsys, nil, []string{"tmpl.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	// "tmpl" layout exists but its file defines "other", not "entry", so executing it errors.
	tmpl, entry := l.resolveLayout("tmpl")
	if err := tmpl.ExecuteTemplate(io.Discard, entry, nil); err == nil {
		t.Error("expected error when entry template is not defined")
	}
}

// --- builtin functions ---

func TestBuiltinParentPath(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{define "t"}}{{parentPath .}}{{end}}`)},
	}
	l, err := NewLayout(fsys, nil, []string{"tmpl.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	tmpl, _ := l.resolveLayout("tmpl")

	cases := []struct {
		input string
		want  string
	}{
		{"a/b/c.md", "a/b"},
		{"a/b", "a"},
		{"a.md", "."},
		{".", ""},
	}
	for _, c := range cases {
		var buf strings.Builder
		if err := tmpl.ExecuteTemplate(&buf, "t", c.input); err != nil {
			t.Fatalf("ExecuteTemplate(%q): %v", c.input, err)
		}
		if got := buf.String(); got != c.want {
			t.Errorf("parentPath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestBuiltinSortByPath(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{define "t"}}{{range sortByPath .}}{{.SitePath}}{{end}}{{end}}`)},
	}
	l, err := NewLayout(fsys, nil, []string{"tmpl.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	pages := []Page{
		&markdownPage{sitePath: "c"},
		&markdownPage{sitePath: "a"},
		&markdownPage{sitePath: "b"},
	}
	var buf strings.Builder
	tmpl, _ := l.resolveLayout("tmpl")
	if err := tmpl.ExecuteTemplate(&buf, "t", pages); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if got, want := buf.String(), "abc"; got != want {
		t.Errorf("sortByPath output = %q, want %q", got, want)
	}
}

// --- RenderContext link methods ---

func TestRenderContextHref(t *testing.T) {
	ctx := RenderContext{SitePath: "blog", Config: SiteConfig{BaseURL: "https://example.com/docs"}}
	targetPage := &markdownPage{sitePath: "blog/post.md"}

	cases := []struct {
		name   string
		target any
		want   string
	}{
		{name: "page", target: targetPage, want: "post.md"},
		{name: "path", target: "about.md", want: "../about.md"},
		{name: "context", target: RenderContext{SitePath: "."}, want: "../"},
	}

	for _, c := range cases {
		got, err := ctx.Href(c.target)
		if err != nil {
			t.Fatalf("Href(%s): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("Href(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRenderContextAbsURL(t *testing.T) {
	ctx := RenderContext{SitePath: "blog", Config: SiteConfig{BaseURL: "https://example.com/docs"}}
	targetPage := &markdownPage{sitePath: "blog/post.md"}

	cases := []struct {
		name   string
		target any
		want   string
	}{
		{name: "page", target: targetPage, want: "https://example.com/docs/blog/post.md"},
		{name: "path", target: "about.md", want: "https://example.com/docs/about.md"},
		{name: "context", target: RenderContext{SitePath: "."}, want: "https://example.com/docs/"},
	}

	for _, c := range cases {
		got, err := ctx.AbsURL(c.target)
		if err != nil {
			t.Fatalf("AbsURL(%s): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("AbsURL(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRenderContextLinkMethodsTemplateUsage(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{define "t"}}{{range .Pages}}{{$.Ctx.Href .}}|{{$.Ctx.AbsURL .}};{{end}}{{end}}`)},
	}
	l, err := NewLayout(fsys, nil, []string{"tmpl.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	data := struct {
		Ctx   RenderContext
		Pages []Page
	}{
		Ctx: RenderContext{SitePath: "blog", Config: SiteConfig{BaseURL: "https://example.com/docs"}},
		Pages: []Page{
			&markdownPage{sitePath: "blog/post.md"},
			&markdownPage{sitePath: "about.md"},
		},
	}

	tmpl, _ := l.resolveLayout("tmpl")
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "t", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if got, want := buf.String(), "post.md|https://example.com/docs/blog/post.md;../about.md|https://example.com/docs/about.md;"; got != want {
		t.Errorf("template output = %q, want %q", got, want)
	}
}
