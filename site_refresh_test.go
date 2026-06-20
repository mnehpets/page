package page

import (
	"net/http/httptest"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// asFSSite casts a SiteIndex to *fsSite for refresh test helpers.
func asFSSite(t *testing.T, site SiteRenderer) *fsSite {
	t.Helper()
	s, ok := site.(*fsSite)
	if !ok {
		t.Fatal("site is not *fsSite")
	}
	return s
}

func TestRefresh_ChangedFile(t *testing.T) {
	fsys := fstest.MapFS{
		"blog/hello.md": {
			Data:    []byte("---\ntitle: Original\n---\nBody.\n"),
			ModTime: time.Unix(1000, 0),
		},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	if pg := s.pages["blog/hello.md"]; pg == nil || pg.Meta().Title != "Original" {
		t.Fatal("precondition: expected 'Original' title")
	}

	// Simulate an edit: update content and advance ModTime.
	fsys["blog/hello.md"] = &fstest.MapFile{
		Data:    []byte("---\ntitle: Updated\n---\nNew body.\n"),
		ModTime: time.Unix(2000, 0),
	}

	n, err := s.Refresh()
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if n != 1 {
		t.Errorf("Refresh count = %d, want 1", n)
	}

	pg := s.pages["blog/hello.md"]
	if pg == nil {
		t.Fatal("page missing after refresh")
	}
	if pg.Meta().Title != "Updated" {
		t.Errorf("title after refresh = %q, want %q", pg.Meta().Title, "Updated")
	}
}

func TestRefresh_UnchangedFileNotReparsed(t *testing.T) {
	mod := time.Unix(1000, 0)
	fsys := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: A\n---\n"), ModTime: mod},
		"b.md": {Data: []byte("---\ntitle: B\n---\n"), ModTime: mod},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	// Change only b.md's data + mtime.
	fsys["b.md"] = &fstest.MapFile{
		Data:    []byte("---\ntitle: B-updated\n---\n"),
		ModTime: time.Unix(2000, 0),
	}

	n, err := s.Refresh()
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 file re-parsed, got %d", n)
	}
	if s.pages["a.md"].Meta().Title != "A" {
		t.Error("a.md should be unchanged")
	}
	if s.pages["b.md"].Meta().Title != "B-updated" {
		t.Error("b.md should reflect update")
	}
}

func TestRefresh_NewFile(t *testing.T) {
	fsys := fstest.MapFS{
		"existing.md": {Data: []byte("---\ntitle: Existing\n---\n"), ModTime: time.Unix(1000, 0)},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	if s.pages["new.md"] != nil {
		t.Fatal("precondition: new.md should not exist yet")
	}

	fsys["new.md"] = &fstest.MapFile{
		Data:    []byte("---\ntitle: New Page\n---\n"),
		ModTime: time.Unix(2000, 0),
	}

	if _, err := s.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	pg := s.pages["new.md"]
	if pg == nil {
		t.Fatal("new.md should appear after Refresh")
	}
	if pg.Meta().Title != "New Page" {
		t.Errorf("title = %q, want %q", pg.Meta().Title, "New Page")
	}

	if _, found := s.pages["new.md"]; !found {
		t.Error("new.md should appear in pages after Refresh")
	}
}

func TestRefresh_DeletedFile(t *testing.T) {
	fsys := fstest.MapFS{
		"keep.md":   {Data: []byte("---\ntitle: Keep\n---\n"), ModTime: time.Unix(1000, 0)},
		"delete.md": {Data: []byte("---\ntitle: Delete\n---\n"), ModTime: time.Unix(1000, 0)},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	// Remove the file from the FS.
	delete(fsys, "delete.md")

	if _, err := s.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if s.pages["delete.md"] != nil {
		t.Error("deleted page should be absent after Refresh")
	}
	if s.pages["keep.md"] == nil {
		t.Error("kept page should still be present after Refresh")
	}
}

func TestUpdateFile(t *testing.T) {
	fsys := fstest.MapFS{
		"page.md": {Data: []byte("---\ntitle: Before\n---\n"), ModTime: time.Unix(1000, 0)},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	fsys["page.md"] = &fstest.MapFile{
		Data:    []byte("---\ntitle: After\n---\n"),
		ModTime: time.Unix(2000, 0),
	}

	if err := s.UpdateFile("page.md"); err != nil {
		t.Fatalf("UpdateFile: %v", err)
	}

	pg := s.pages["page.md"]
	if pg == nil {
		t.Fatal("page missing after UpdateFile")
	}
	if pg.Meta().Title != "After" {
		t.Errorf("title = %q, want %q", pg.Meta().Title, "After")
	}
}

func TestDeleteFile_NonIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"keep.md":   {Data: []byte("---\ntitle: Keep\n---\n"), ModTime: time.Unix(1000, 0)},
		"delete.md": {Data: []byte("---\ntitle: Delete\n---\n"), ModTime: time.Unix(1000, 0)},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	delete(fsys, "delete.md")
	s.DeleteFile("delete.md")

	if s.pages["delete.md"] != nil {
		t.Error("deleted page should be absent")
	}
	if s.pages["keep.md"] == nil {
		t.Error("kept page should still be present")
	}
}

func TestDeleteFile_IndexFallback(t *testing.T) {
	// blog/index.html has priority 1; blog/README.md has priority 3.
	// After deleting index.html, README.md should become the directory index.
	fsys := fstest.MapFS{
		"blog/index.html": {
			Data:    []byte(`<!DOCTYPE html><html><head><script type="application/ld+json">{"site":{"layout":"default"}}</script><title>HTML Index</title></head><body></body></html>`),
			ModTime: time.Unix(1000, 0),
		},
		"blog/README.md": {
			Data:    []byte("---\ntitle: MD Index\n---\n"),
			ModTime: time.Unix(1000, 0),
		},
		"_layouts/default.html": {Data: []byte(`{{define "default"}}{{.Content}}{{end}}`)},
	}
	site, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, site)

	if pg := s.pages["blog"]; pg == nil || pg.Meta().Title != "HTML Index" {
		t.Fatalf("precondition: expected HTML Index, got %v", s.pages["blog"])
	}

	// Remove the higher-priority file.
	delete(fsys, "blog/index.html")
	s.DeleteFile("blog/index.html")

	pg := s.pages["blog"]
	if pg == nil {
		t.Fatal("blog/ index should still exist via fallback")
	}
	if pg.Meta().Title != "MD Index" {
		t.Errorf("fallback title = %q, want %q", pg.Meta().Title, "MD Index")
	}
}

func TestRefresh_Concurrency(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md":                  {Data: []byte("---\ntitle: A\n---\n"), ModTime: time.Unix(1000, 0)},
		"b.md":                  {Data: []byte("---\ntitle: B\n---\n"), ModTime: time.Unix(1000, 0)},
		"_layouts/default.html": {Data: []byte(`{{define "default"}}{{.Meta.Title}}{{end}}`)},
	}
	idx, err := NewSite(fsys)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	s := asFSSite(t, idx)

	// Update the FS before launching goroutines to avoid racing on the map.
	fsys["a.md"] = &fstest.MapFile{
		Data:    []byte("---\ntitle: A-concurrent\n---\n"),
		ModTime: time.Unix(2000, 0),
	}

	var wg sync.WaitGroup
	const goroutines = 8

	fileHook := idx.FileRenderer()

	// Concurrent Refresh calls.
	for range goroutines / 2 {
		wg.Go(func() {
			s.Refresh() //nolint:errcheck
		})
	}

	// Concurrent renders via the real FileRenderer hook, exercising the full
	// production path: RLock → page lookup → pg.Renderer → lockedRenderer.Render
	// (template execution) → RUnlock.
	for range goroutines / 2 {
		wg.Go(func() {
			f, err := fsys.Open("a.md")
			if err != nil {
				return
			}
			renderer, err := fileHook("a.md", fsys, f)
			if err != nil || renderer == nil {
				return
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/a.md", nil)
			renderer.Render(w, r) //nolint:errcheck
		})
	}

	wg.Wait()
}
