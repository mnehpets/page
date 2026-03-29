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

func TestSortByLastMod(t *testing.T) {
	t1, _ := time.Parse("2006-01-02", "2024-01-01")
	t2, _ := time.Parse("2006-01-02", "2024-06-15")

	pages := []Page{
		&markdownPage{urlPath: "/a", meta: Meta{LastMod: t1}},
		&markdownPage{urlPath: "/b", meta: Meta{LastMod: t2}},
		&markdownPage{urlPath: "/c", meta: Meta{}}, // zero LastMod
	}

	sorted := SortByLastMod(pages)
	if sorted[0].URLPath() != "/b" {
		t.Errorf("first page should be most recently modified (/b), got %q", sorted[0].URLPath())
	}
	if sorted[1].URLPath() != "/a" {
		t.Errorf("second page should be /a, got %q", sorted[1].URLPath())
	}
	if sorted[2].URLPath() != "/c" {
		t.Errorf("zero-lastmod page should be last, got %q", sorted[2].URLPath())
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

func makeNavSiteFS() fstest.MapFS {
	return fstest.MapFS{
		"index.md":           {Data: []byte("---\ntitle: Home\n---\n")},
		"about.md":           {Data: []byte("---\ntitle: About\n---\n")},
		"blog/index.md":      {Data: []byte("---\ntitle: Blog\n---\n")},
		"blog/post-a.md":     {Data: []byte("---\ntitle: Post A\n---\n")},
		"blog/post-b.md":     {Data: []byte("---\ntitle: Post B\n---\n")},
		"blog/drafts/index.md": {Data: []byte("---\ntitle: Drafts\ndraft: true\n---\n")},
		"blog/go/index.md":   {Data: []byte("---\ntitle: Go Posts\n---\n")},
		"blog/go/intro.md":   {Data: []byte("---\ntitle: Intro\n---\n")},
	}
}

func TestAncestorsOf(t *testing.T) {
	fsys := makeNavSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	t.Run("deep path has root, blog, and go ancestors", func(t *testing.T) {
		ancestors := site.AncestorsOf("/blog/go/intro.md")
		if len(ancestors) != 3 {
			t.Fatalf("want 3 ancestors, got %d: %v", len(ancestors), urlPaths(ancestors))
		}
		if ancestors[0].URLPath() != "/" {
			t.Errorf("ancestors[0] = %q, want /", ancestors[0].URLPath())
		}
		if ancestors[1].URLPath() != "/blog/" {
			t.Errorf("ancestors[1] = %q, want /blog/", ancestors[1].URLPath())
		}
		if ancestors[2].URLPath() != "/blog/go/" {
			t.Errorf("ancestors[2] = %q, want /blog/go/", ancestors[2].URLPath())
		}
	})

	t.Run("root-first ordering", func(t *testing.T) {
		ancestors := site.AncestorsOf("/blog/post-a.md")
		if len(ancestors) != 2 {
			t.Fatalf("want 2 ancestors (/ and /blog/), got %d", len(ancestors))
		}
		if ancestors[0].URLPath() != "/" {
			t.Errorf("ancestors[0] = %q, want /", ancestors[0].URLPath())
		}
		if ancestors[1].URLPath() != "/blog/" {
			t.Errorf("ancestors[1] = %q, want /blog/", ancestors[1].URLPath())
		}
	})

	t.Run("intermediate missing path is skipped", func(t *testing.T) {
		// /blog/go/ exists but /blog/go/intro.md's /blog/ is present, /blog/go/ also present.
		// Test with a path whose intermediate dir has no index page.
		fsys2 := fstest.MapFS{
			"index.md":          {Data: []byte("---\ntitle: Home\n---\n")},
			"docs/guide/page.md": {Data: []byte("---\ntitle: Page\n---\n")},
			// /docs/ has no index page — should be skipped
		}
		s2, err := NewSite(fsys2)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}
		ancestors := s2.AncestorsOf("/docs/guide/page.md")
		// Only / is in the index; /docs/ is absent.
		if len(ancestors) != 1 {
			t.Fatalf("want 1 ancestor (only /), got %d: %v", len(ancestors), urlPaths(ancestors))
		}
		if ancestors[0].URLPath() != "/" {
			t.Errorf("ancestor = %q, want /", ancestors[0].URLPath())
		}
	})

	t.Run("root has no ancestors", func(t *testing.T) {
		ancestors := site.AncestorsOf("/")
		if len(ancestors) != 0 {
			t.Errorf("root ancestors: want 0, got %d", len(ancestors))
		}
	})

	t.Run("direct child of root has no ancestors in index-less site", func(t *testing.T) {
		fsys3 := fstest.MapFS{
			"page.md": {Data: []byte("---\ntitle: Page\n---\n")},
		}
		s3, err := NewSite(fsys3)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}
		ancestors := s3.AncestorsOf("/page.md")
		if len(ancestors) != 0 {
			t.Errorf("want 0 ancestors when / absent, got %d", len(ancestors))
		}
	})
}

func TestChildrenOf(t *testing.T) {
	fsys := makeNavSiteFS()
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	t.Run("children of root", func(t *testing.T) {
		children := site.ChildrenOf("/")
		paths := urlPaths(children)
		if !containsPath(paths, "/about.md") {
			t.Errorf("expected /about.md in children of /: %v", paths)
		}
		if !containsPath(paths, "/blog/") {
			t.Errorf("expected /blog/ in children of /: %v", paths)
		}
		// Root index is not a child of itself.
		if containsPath(paths, "/") {
			t.Errorf("root should not appear as its own child: %v", paths)
		}
	})

	t.Run("children of blog", func(t *testing.T) {
		children := site.ChildrenOf("/blog/")
		paths := urlPaths(children)
		if !containsPath(paths, "/blog/post-a.md") {
			t.Errorf("expected /blog/post-a.md: %v", paths)
		}
		if !containsPath(paths, "/blog/post-b.md") {
			t.Errorf("expected /blog/post-b.md: %v", paths)
		}
		if !containsPath(paths, "/blog/go/") {
			t.Errorf("expected /blog/go/: %v", paths)
		}
		// Grandchild /blog/go/intro.md is not a direct child.
		if containsPath(paths, "/blog/go/intro.md") {
			t.Errorf("/blog/go/intro.md should not be a direct child of /blog/: %v", paths)
		}
	})

	t.Run("draft pages excluded", func(t *testing.T) {
		children := site.ChildrenOf("/blog/")
		for _, p := range children {
			if p.Meta().Draft {
				t.Errorf("draft page %q should not appear in ChildrenOf", p.URLPath())
			}
		}
	})

	t.Run("draft pages included with WithIncludeDrafts", func(t *testing.T) {
		s2, err := NewSite(fsys, WithIncludeDrafts())
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}
		children := s2.ChildrenOf("/blog/")
		paths := urlPaths(children)
		if !containsPath(paths, "/blog/drafts/") {
			t.Errorf("expected /blog/drafts/ with WithIncludeDrafts: %v", paths)
		}
	})

	t.Run("leaf page has no children", func(t *testing.T) {
		children := site.ChildrenOf("/blog/post-a.md")
		if len(children) != 0 {
			t.Errorf("leaf page should have no children, got %v", urlPaths(children))
		}
	})
}

func TestParentURLPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/a/b/", "/a/"},
		{"/a/", "/"},
		{"/", ""},
		{"/a/b.md", "/a/"},
		{"/blog/hello-world.md", "/blog/"},
	}
	for _, c := range cases {
		got := parentURLPath(c.input)
		if got != c.want {
			t.Errorf("parentURLPath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSortByPath(t *testing.T) {
	pages := []Page{
		&markdownPage{urlPath: "/c/"},
		&markdownPage{urlPath: "/a/"},
		&markdownPage{urlPath: "/b/"},
	}
	sorted := SortByPath(pages)
	want := []string{"/a/", "/b/", "/c/"}
	for i, p := range sorted {
		if p.URLPath() != want[i] {
			t.Errorf("sorted[%d] = %q, want %q", i, p.URLPath(), want[i])
		}
	}
}

func urlPaths(pages []Page) []string {
	paths := make([]string, len(pages))
	for i, p := range pages {
		paths[i] = p.URLPath()
	}
	return paths
}

func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
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
