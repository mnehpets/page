package page

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)


func makeTestLayout(t *testing.T, define string) *Layout {
	t.Helper()
	fsys := fstest.MapFS{
		"default.html": {Data: []byte(define)},
	}
	l, err := NewLayout(fsys, nil, []string{"default.html"})
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}
	return l
}

func TestMarkdownPage_FrontMatterParsed(t *testing.T) {
	src := "---\ntitle: My Post\nauthor: Bob\n---\nHello, world.\n"
	fsys := fstest.MapFS{
		"blog/post.md": {Data: []byte(src)},
	}

	pg, err := newMarkdownPageFromFS("blog/post.md", fsys, "blog/post.md")
	if err != nil {
		t.Fatalf("newMarkdownPageFromFS: %v", err)
	}

	if pg.Meta().Title != "My Post" {
		t.Errorf("Title = %q", pg.Meta().Title)
	}
	if pg.Meta().Author != "Bob" {
		t.Errorf("Author = %q", pg.Meta().Author)
	}
	if pg.SitePath() != "blog/post.md" {
		t.Errorf("SitePath = %q", pg.SitePath())
	}
}

func TestMarkdownPage_BodyRenderedViaGoldmark(t *testing.T) {
	src := "---\ntitle: Test\n---\n# Heading\n\nParagraph text.\n"
	fsys := fstest.MapFS{
		"post.md": {Data: []byte(src)},
	}
	layout := makeTestLayout(t, `{{define "entry"}}{{.Content}}{{end}}`)

	pg, err := newMarkdownPageFromFS("post.md", fsys, "post.md")
	if err != nil {
		t.Fatalf("newMarkdownPageFromFS: %v", err)
	}

	r := httptest.NewRequest("GET", "/post.md", nil)
	renderer, err := pg.Renderer(nil, layout)
	if err != nil {
		t.Fatalf("Renderer: %v", err)
	}
	w := httptest.NewRecorder()
	if err := renderer.Render(w, r); err != nil {
		t.Fatalf("renderer.Render: %v", err)
	}

	got := w.Body.String()
	if !strings.Contains(got, "<h1>") {
		t.Errorf("rendered output missing <h1>: %q", got)
	}
	if !strings.Contains(got, "Paragraph text") {
		t.Errorf("rendered output missing body text: %q", got)
	}
	// Front matter should not appear in output.
	if strings.Contains(got, "---") {
		t.Errorf("front matter delimiter appeared in rendered output: %q", got)
	}
}

func TestMarkdownPage_FencedCodeBlockLanguageClass(t *testing.T) {
	src := "---\ntitle: Code\n---\n```go\nfunc main() {}\n```\n"
	fsys := fstest.MapFS{
		"code.md": {Data: []byte(src)},
	}
	layout := makeTestLayout(t, `{{define "entry"}}{{.Content}}{{end}}`)

	pg, err := newMarkdownPageFromFS("code.md", fsys, "code.md")
	if err != nil {
		t.Fatalf("newMarkdownPageFromFS: %v", err)
	}

	r := httptest.NewRequest("GET", "/code.md", nil)
	renderer, err := pg.Renderer(nil, layout)
	if err != nil {
		t.Fatalf("Renderer: %v", err)
	}
	w := httptest.NewRecorder()
	if err := renderer.Render(w, r); err != nil {
		t.Fatalf("renderer.Render: %v", err)
	}

	if !strings.Contains(w.Body.String(), `class="language-go"`) {
		t.Errorf("expected language-go class in rendered output, got: %q", w.Body.String())
	}
}

func TestMarkdownPage_RenderWritesLayoutOutput(t *testing.T) {
	src := "---\ntitle: Layout Test\n---\nContent.\n"
	fsys := fstest.MapFS{
		"page.md": {Data: []byte(src)},
	}
	layout := makeTestLayout(t, `{{define "entry"}}<html><head><title>{{.Meta.Title}}</title></head><body>{{.Content}}</body></html>{{end}}`)

	pg, err := newMarkdownPageFromFS("page.md", fsys, "page.md")
	if err != nil {
		t.Fatalf("newMarkdownPageFromFS: %v", err)
	}

	r := httptest.NewRequest("GET", "/page.md", nil)
	renderer, err := pg.Renderer(nil, layout)
	if err != nil {
		t.Fatalf("Renderer: %v", err)
	}
	w := httptest.NewRecorder()
	if err := renderer.Render(w, r); err != nil {
		t.Fatalf("renderer.Render: %v", err)
	}

	got := w.Body.String()
	if !strings.Contains(got, "<title>Layout Test</title>") {
		t.Errorf("missing title in layout output: %q", got)
	}
	if !strings.Contains(got, "<body>") {
		t.Errorf("missing body in layout output: %q", got)
	}
}

func TestMarkdownPage_RawHTMLPassedThrough(t *testing.T) {
	src := "---\ntitle: Raw\n---\n<details><summary>Click</summary>body</details>\n"
	fsys := fstest.MapFS{
		"raw.md": {Data: []byte(src)},
	}
	layout := makeTestLayout(t, `{{define "entry"}}{{.Content}}{{end}}`)

	pg, err := newMarkdownPageFromFS("raw.md", fsys, "raw.md")
	if err != nil {
		t.Fatalf("newMarkdownPageFromFS: %v", err)
	}

	r := httptest.NewRequest("GET", "/raw.md", nil)
	renderer, err := pg.Renderer(nil, layout)
	if err != nil {
		t.Fatalf("Renderer: %v", err)
	}
	w := httptest.NewRecorder()
	if err := renderer.Render(w, r); err != nil {
		t.Fatalf("renderer.Render: %v", err)
	}

	got := w.Body.String()
	if !strings.Contains(got, "<details>") {
		t.Errorf("raw <details> was escaped/removed from output: %q", got)
	}
	if strings.Contains(got, "&lt;details&gt;") {
		t.Errorf("raw HTML was escaped in output: %q", got)
	}
}

func TestMarkdownPage_NilLayoutReturnsError(t *testing.T) {
	fsys := fstest.MapFS{
		"post.md": {Data: []byte("Hello.\n")},
	}
	pg, err := newMarkdownPageFromFS("post.md", fsys, "post.md")
	if err != nil {
		t.Fatalf("newMarkdownPageFromFS: %v", err)
	}

	renderer, err := pg.Renderer(nil, nil)
	if err != nil {
		t.Fatalf("Renderer: %v", err)
	}
	w := httptest.NewRecorder()
	if err := renderer.Render(w, httptest.NewRequest("GET", "/post.md", nil)); err == nil {
		t.Error("expected error with nil layout")
	}
}
