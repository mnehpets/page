package page

import (
	"testing"
	"testing/fstest"
	"time"
)

func makeSiteFS() fstest.MapFS {
	return fstest.MapFS{
		"blog/hello-world.md": {Data: []byte("---\ntitle: Hello World\ntags:\n  - go\ncollection: blog\n---\nBody.\n")},
		"blog/draft.md":       {Data: []byte("---\ntitle: Draft\ndraft: true\ntags:\n  - go\ncollection: blog\n---\nDraft body.\n")},
		"blog/index.md":       {Data: []byte("---\ntitle: Blog Index\n---\nIndex.\n")},
		"blog/index.html":     {Data: []byte(`<!DOCTYPE html><html><head><script type="application/ld+json">{"site":{"layout":"default"}}</script><title>Blog HTML Index</title></head><body>HTML Index</body></html>`)},
		"about.html":          {Data: []byte(`<!DOCTYPE html><html><head><script type="application/ld+json">{"site":{"layout":"default"}}</script><title>About</title></head><body>About us</body></html>`)},
		"style.css":           {Data: []byte("body {}")},
		"_layouts/default.html": {Data: []byte(`{{define "default"}}{{.Content}}{{end}}`)},
	}
}

func TestNewSite_IndexBuilt(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	pg, ok := site.Get("/blog/hello-world.md")
	if !ok || pg == nil {
		t.Error("expected page at /blog/hello-world.md")
	}
	if pg.Meta().Title != "Hello World" {
		t.Errorf("Title = %q", pg.Meta().Title)
	}
}

func TestNewSite_URLPathsCorrect(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	if _, ok := site.Get("/blog/hello-world.md"); !ok {
		t.Error("missing /blog/hello-world.md")
	}
	if _, ok := site.Get("/about.html"); !ok {
		t.Error("missing /about.html")
	}
}

func TestNewSite_IndexFilePriority(t *testing.T) {
	// Both blog/index.html and blog/index.md exist; index.html wins.
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	pg, ok := site.Get("/blog/")
	if !ok {
		t.Fatal("expected page at /blog/")
	}
	// index.html has priority — its title comes from <title>.
	if pg.Meta().Title != "Blog HTML Index" {
		t.Errorf("Title = %q, want %q (index.html should win)", pg.Meta().Title, "Blog HTML Index")
	}
}

func TestNewSite_StaticFilesExcluded(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	for _, pg := range site.All() {
		if pg.URLPath() == "/style.css" {
			t.Error("static .css file should not appear in All()")
		}
	}
}

func TestNewSite_DraftFiltering(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	// Draft excluded from All().
	for _, pg := range site.All() {
		if pg.URLPath() == "/blog/draft.md" {
			t.Error("draft page should not appear in All()")
		}
	}

	// Draft still reachable via Get().
	pg, ok := site.Get("/blog/draft.md")
	if !ok || pg == nil {
		t.Error("draft page should be retrievable via Get()")
	}
}

func TestNewSite_WithIncludeDrafts(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys, WithIncludeDrafts())
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	found := false
	for _, pg := range site.All() {
		if pg.URLPath() == "/blog/draft.md" {
			found = true
		}
	}
	if !found {
		t.Error("draft page should appear in All() with WithIncludeDrafts")
	}
}

func TestNewSite_ByTag(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	pages := site.ByTag("go")
	if len(pages) == 0 {
		t.Error("expected pages tagged 'go'")
	}
	for _, pg := range pages {
		if pg.Meta().Draft {
			t.Errorf("draft page %q should not appear in ByTag", pg.URLPath())
		}
	}
}

func TestNewSite_ByCollection(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	pages := site.ByCollection("blog")
	if len(pages) == 0 {
		t.Error("expected pages in collection 'blog'")
	}
	for _, pg := range pages {
		if pg.Meta().Collection != "blog" {
			t.Errorf("unexpected collection %q", pg.Meta().Collection)
		}
	}
}

func TestNewSite_SlugDerivation(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	pg, _ := site.Get("/blog/hello-world.md")
	if pg.Meta().Slug != "hello-world" {
		t.Errorf("slug for regular file = %q, want %q", pg.Meta().Slug, "hello-world")
	}
}

func TestNewSite_WithLayout(t *testing.T) {
	fsys := makeSiteFS()
	layoutFS := fstest.MapFS{
		"custom.html": {Data: []byte(`{{define "default"}}custom{{end}}`)},
	}
	l, err := NewLayout(layoutFS, "custom.html")
	if err != nil {
		t.Fatalf("NewLayout: %v", err)
	}

	site, err := NewSite(fsys, WithLayout(l))
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	if site.(*fsSite).layout != l {
		t.Error("expected WithLayout to set the provided layout")
	}
}

func TestNewSite_LayoutDiscoveredFromFS(t *testing.T) {
	fsys := makeSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	if site.(*fsSite).layout == nil {
		t.Error("expected _layouts/ to be discovered and layout to be non-nil")
	}
}

func TestNewSite_NoLayoutsDir(t *testing.T) {
	fsys := fstest.MapFS{
		"post.md": {Data: []byte("Hello.\n")},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	if site.(*fsSite).layout != nil {
		t.Error("expected nil layout when no _layouts/ dir")
	}
}

func TestSortByDate(t *testing.T) {
	t1, _ := time.Parse("2006-01-02", "2024-01-01")
	t2, _ := time.Parse("2006-01-02", "2024-06-15")

	pages := []Page{
		&markdownPage{urlPath: "/a", meta: Meta{Date: t1}},
		&markdownPage{urlPath: "/b", meta: Meta{Date: t2}},
		&markdownPage{urlPath: "/c", meta: Meta{}}, // zero date
	}

	sorted := SortByDate(pages)
	if sorted[0].URLPath() != "/b" {
		t.Errorf("first page should be newest (/b), got %q", sorted[0].URLPath())
	}
	if sorted[1].URLPath() != "/a" {
		t.Errorf("second page should be /a, got %q", sorted[1].URLPath())
	}
	if sorted[2].URLPath() != "/c" {
		t.Errorf("zero-date page should be last, got %q", sorted[2].URLPath())
	}
}

func TestPaginate(t *testing.T) {
	pages := make([]Page, 25)
	for i := range pages {
		pages[i] = &markdownPage{urlPath: "/p"}
	}

	// First page.
	got, more := Paginate(pages, 10, 1)
	if len(got) != 10 || !more {
		t.Errorf("page 1: got %d items, more=%v", len(got), more)
	}

	// Last page (25 items, page size 10, page 3 → 5 items).
	got, more = Paginate(pages, 10, 3)
	if len(got) != 5 || more {
		t.Errorf("page 3: got %d items, more=%v", len(got), more)
	}

	// Beyond range.
	got, more = Paginate(pages, 10, 5)
	if len(got) != 0 || more {
		t.Errorf("page 5 (out of range): got %d items, more=%v", len(got), more)
	}
}

func TestDeriveSlug(t *testing.T) {
	cases := []struct {
		urlPath string
		want    string
	}{
		{"/blog/hello-world.md", "hello-world"},
		{"/blog/post.html", "post"},
		{"/blog/post.htm", "post"},
		{"/blog/", "blog"},
		{"/blog/staff/", "staff"},
		{"/", ""},
	}
	for _, c := range cases {
		got := deriveSlug(c.urlPath)
		if got != c.want {
			t.Errorf("deriveSlug(%q) = %q, want %q", c.urlPath, got, c.want)
		}
	}
}

func TestFileURLPath(t *testing.T) {
	cases := []struct {
		filePath string
		urlPath  string
		isIndex  bool
		priority int
	}{
		{"blog/hello-world.md", "/blog/hello-world.md", false, 0},
		{"blog/index.html", "/blog/", true, 1},
		{"blog/index.htm", "/blog/", true, 2},
		{"blog/index.md", "/blog/", true, 3},
		{"index.html", "/", true, 1},
		{"index.md", "/", true, 3},
		{"style.css", "/style.css", false, 0},
	}
	for _, c := range cases {
		urlPath, isIndex, priority := fileURLPath(c.filePath)
		if urlPath != c.urlPath || isIndex != c.isIndex || priority != c.priority {
			t.Errorf("fileURLPath(%q) = (%q, %v, %d), want (%q, %v, %d)",
				c.filePath, urlPath, isIndex, priority,
				c.urlPath, c.isIndex, c.priority)
		}
	}
}
