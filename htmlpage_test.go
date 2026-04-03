package page

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const htmlWithJSONLD = `<!DOCTYPE html>
<html>
<head>
<script type="application/ld+json">
{
  "@context": {
    "@vocab": "https://schema.org/",
    "site": "https://github.com/mnehpets/page"
  },
  "name": "My Article",
  "author": "Alice",
  "description": "A description.",
  "keywords": "go, web",
  "site": {
    "layout": "post",
    "draft": false,
    "collection": "blog",
    "slug": "my-article"
  }
}
</script>
<link rel="stylesheet" href="/style.css">
</head>
<body><p>Hello from HTML.</p></body>
</html>`

func TestHTMLPage_JSONLDParsed(t *testing.T) {
	fsys := fstest.MapFS{
		"blog/post.html": {Data: []byte(htmlWithJSONLD)},
	}
	pg, err := newHTMLPageFromFS("blog/post.html", fsys, "blog/post.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}

	m := pg.Meta()
	if m.Title != "My Article" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Author != "Alice" {
		t.Errorf("Author = %q", m.Author)
	}
	if m.Description != "A description." {
		t.Errorf("Description = %q", m.Description)
	}
	if len(m.Layouts) != 1 || m.Layouts[0] != "post" {
		t.Errorf("Layouts = %v", m.Layouts)
	}
	if m.Collection != "blog" {
		t.Errorf("Collection = %q", m.Collection)
	}
	if m.Slug != "my-article" {
		t.Errorf("Slug = %q", m.Slug)
	}
	if len(m.Tags) != 2 || m.Tags[0] != "go" || m.Tags[1] != "web" {
		t.Errorf("Tags = %v", m.Tags)
	}
}

func TestHTMLPage_HTMLMetaFallback(t *testing.T) {
	src := `<!DOCTYPE html><html><head>
<script type="application/ld+json">{"site":{"layout":"default"}}</script>
<meta property="og:title" content="Meta Title">
<meta name="description" content="Meta description.">
<meta name="author" content="Bob">
</head><body>content</body></html>`

	fsys := fstest.MapFS{"page.html": {Data: []byte(src)}}
	pg, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}

	m := pg.Meta()
	if m.Title != "Meta Title" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Description != "Meta description." {
		t.Errorf("Description = %q", m.Description)
	}
	if m.Author != "Bob" {
		t.Errorf("Author = %q", m.Author)
	}
}

func TestHTMLPage_TitleFallback(t *testing.T) {
	src := `<!DOCTYPE html><html><head><script type="application/ld+json">{"site":{"layout":"default"}}</script><title>Title Element</title></head><body>x</body></html>`
	fsys := fstest.MapFS{"page.html": {Data: []byte(src)}}
	pg, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}
	if pg.Meta().Title != "Title Element" {
		t.Errorf("Title = %q", pg.Meta().Title)
	}
}

func TestHTMLPage_FSDateFallback(t *testing.T) {
	src := `<!DOCTYPE html><html><head><script type="application/ld+json">{"site":{"layout":"default"}}</script></head><body>x</body></html>`
	modTime, _ := time.Parse("2006-01-02", "2024-03-01")
	fsys := fstest.MapFS{
		"page.html": {Data: []byte(src), ModTime: modTime},
	}
	pg, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}
	if !pg.Meta().Date.Equal(modTime) {
		t.Errorf("Date = %v, want %v", pg.Meta().Date, modTime)
	}
	if !pg.Meta().LastMod.Equal(modTime) {
		t.Errorf("LastMod = %v, want %v", pg.Meta().LastMod, modTime)
	}
}

func TestHTMLPage_DateModified(t *testing.T) {
	src := `<!DOCTYPE html><html><head>
<script type="application/ld+json">{
  "site": {"layout": "default"},
  "datePublished": "2024-01-01",
  "dateModified": "2024-06-15"
}</script>
</head><body>x</body></html>`
	fsys := fstest.MapFS{"page.html": {Data: []byte(src)}}
	pg, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}
	wantDate, _ := time.Parse("2006-01-02", "2024-01-01")
	wantLastMod, _ := time.Parse("2006-01-02", "2024-06-15")
	if !pg.Meta().Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", pg.Meta().Date, wantDate)
	}
	if !pg.Meta().LastMod.Equal(wantLastMod) {
		t.Errorf("LastMod = %v, want %v", pg.Meta().LastMod, wantLastMod)
	}
}

func TestHTMLPage_ContentAndHeadPopulated(t *testing.T) {
	src := `<!DOCTYPE html><html>
<head>
<script type="application/ld+json">{"site":{"layout":"default"}}</script>
<link rel="stylesheet" href="/style.css">
<meta property="og:title" content="Title">
</head>
<body><p>Body content.</p></body>
</html>`
	fsys := fstest.MapFS{"page.html": {Data: []byte(src)}}
	layout := makeTestLayout(t, `{{define "default"}}HEAD:{{.Head}}|BODY:{{.Content}}{{end}}`)

	pg, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}

	r := httptest.NewRequest("GET", "/page.html", nil)
	renderer, err := pg.Renderer(nil, layout)
	if err != nil {
		t.Fatalf("Renderer: %v", err)
	}
	w := httptest.NewRecorder()
	if err := renderer.Render(w, r); err != nil {
		t.Fatalf("renderer.Render: %v", err)
	}

	got := w.Body.String()
	// The stylesheet link should appear in Head (not captured by Meta).
	if !strings.Contains(got, "HEAD:") || !strings.Contains(got, "stylesheet") {
		t.Errorf("head content missing stylesheet: %q", got)
	}
	// The og:title meta tag was captured into Meta and should not be in Head.
	if strings.Contains(got, "og:title") {
		t.Errorf("og:title meta tag should be excluded from Head: %q", got)
	}
	// Body content.
	if !strings.Contains(got, "Body content.") {
		t.Errorf("body content missing: %q", got)
	}
}

func TestHTMLPage_NoLayoutReturnsNil(t *testing.T) {
	// An HTML file with no explicit layout declaration is served as a static
	// file, even if it has other metadata like a title or og: tags.
	src := `<!DOCTYPE html><html><head><title>Has Title But No Layout</title></head><body>x</body></html>`
	fsys := fstest.MapFS{"page.html": {Data: []byte(src)}}
	pg, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err != nil {
		t.Fatalf("newHTMLPageFromFS: %v", err)
	}
	if pg != nil {
		t.Errorf("expected nil page for HTML without layout, got %+v", pg)
	}
}

func TestHTMLPage_InvalidJSONReturnsError(t *testing.T) {
	src := `<!DOCTYPE html><html><head>
<script type="application/ld+json">{ not valid json }</script>
</head><body></body></html>`
	fsys := fstest.MapFS{"page.html": {Data: []byte(src)}}
	_, err := newHTMLPageFromFS("page.html", fsys, "page.html")
	if err == nil {
		t.Error("expected error for invalid JSON-LD")
	}
}
