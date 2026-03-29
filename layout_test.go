package page

import (
	"io"
	"strings"
	"testing"
	"testing/fstest"
)


func TestNewLayout_ParsesTemplates(t *testing.T) {
	fsys := fstest.MapFS{
		"_layouts/default.html": {Data: []byte(`{{define "default"}}<html>{{.Content}}</html>{{end}}`)},
		"_layouts/post.html":    {Data: []byte(`{{define "post"}}<article>{{.Content}}</article>{{end}}`)},
	}

	l, err := NewLayout(fsys, "_layouts/*.html")
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	if l == nil {
		t.Fatal("Layout should not be nil")
	}

	// Execute "default" template.
	var buf strings.Builder
	if err := l.Template().ExecuteTemplate(&buf, "default", RenderContext{Content: "hello"}); err != nil {
		t.Fatalf("ExecuteTemplate default: %v", err)
	}
	if got := buf.String(); got != "<html>hello</html>" {
		t.Errorf("default output = %q", got)
	}

	// Execute "post" template.
	buf.Reset()
	if err := l.Template().ExecuteTemplate(&buf, "post", RenderContext{Content: "world"}); err != nil {
		t.Fatalf("ExecuteTemplate post: %v", err)
	}
	if got := buf.String(); got != "<article>world</article>" {
		t.Errorf("post output = %q", got)
	}
}

func TestNewLayout_UnknownNameReturnsError(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{define "default"}}ok{{end}}`)},
	}
	l, err := NewLayout(fsys, "tmpl.html")
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	if err := l.Template().ExecuteTemplate(io.Discard, "nonexistent", nil); err == nil {
		t.Error("expected error for unknown template name")
	}
}

func TestNewLayout_EmptyPatternReturnsError(t *testing.T) {
	fsys := fstest.MapFS{}
	if _, err := NewLayout(fsys, "*.html"); err == nil {
		t.Error("expected error when no files match pattern")
	}
}

func TestNewLayout_NoPatternReturnsError(t *testing.T) {
	fsys := fstest.MapFS{}
	if _, err := NewLayout(fsys); err == nil {
		t.Error("expected error when no patterns given")
	}
}

func TestNewLayout_ParseError(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.html": {Data: []byte(`{{define "x"}}{{.Unclosed`)},
	}
	if _, err := NewLayout(fsys, "bad.html"); err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestBuiltinParentPath(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{define "t"}}{{parentPath .}}{{end}}`)},
	}
	l, err := NewLayout(fsys, "tmpl.html")
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	tmpl := l.Template()

	cases := []struct {
		input string
		want  string
	}{
		{"/a/b/", "/a/"},
		{"/a/", "/"},
		{"/", ""},
		{"/a/b.md", "/a/"},
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
		"tmpl.html": {Data: []byte(`{{define "t"}}{{range sortByPath .}}{{.URLPath}}{{end}}{{end}}`)},
	}
	l, err := NewLayout(fsys, "tmpl.html")
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	pages := []Page{
		&markdownPage{urlPath: "/c/"},
		&markdownPage{urlPath: "/a/"},
		&markdownPage{urlPath: "/b/"},
	}
	var buf strings.Builder
	if err := l.Template().ExecuteTemplate(&buf, "t", pages); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if got, want := buf.String(), "/a//b//c/"; got != want {
		t.Errorf("sortByPath output = %q, want %q", got, want)
	}
}
