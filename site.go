package page

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/mnehpets/oneserve/endpoint"
)

// Site indexes pages by URL path and exposes query methods for use in templates.
type Site interface {
	Get(urlPath string) (Page, bool)
	All() []Page
	ByTag(tag string) []Page
	ByCollection(name string) []Page
	FileRenderer() endpoint.FileRendererHook
	DirRenderer() endpoint.FileRendererHook
}

// SiteOption is a functional option for NewSite.
type SiteOption func(*siteConfig)

type siteConfig struct {
	layout        *Layout
	includeDrafts bool
}

// WithLayout overrides the automatic _layouts/ discovery. The provided Layout
// is used for all pages; _layouts/ is ignored.
func WithLayout(l *Layout) SiteOption {
	return func(c *siteConfig) { c.layout = l }
}

// WithIncludeDrafts causes draft pages to appear in All(), ByTag(), and
// ByCollection() results. Draft pages are always retrievable via Get().
func WithIncludeDrafts() SiteOption {
	return func(c *siteConfig) { c.includeDrafts = true }
}

type fsSite struct {
	pages  map[string]Page // urlPath → Page (all pages, including drafts)
	layout *Layout
	drafts bool // include drafts in query results
}

// NewSite walks fsys, parses all .md, .html, and .htm files, and returns a
// Site indexed by URL path. _layouts/ is automatically parsed unless
// WithLayout is provided.
func NewSite(fsys fs.FS, opts ...SiteOption) (Site, error) {
	cfg := &siteConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	layout := cfg.layout
	if layout == nil {
		l, err := discoverLayouts(fsys)
		if err != nil {
			return nil, err
		}
		layout = l
	}

	pages, err := buildPageIndex(fsys)
	if err != nil {
		return nil, err
	}

	return &fsSite{pages: pages, layout: layout, drafts: cfg.includeDrafts}, nil
}

// Layout returns the site's layout, which may be nil if no _layouts/ directory
// was found and no WithLayout option was provided.
func (s *fsSite) Layout() *Layout { return s.layout }

func (s *fsSite) Get(urlPath string) (Page, bool) {
	p, ok := s.pages[urlPath]
	return p, ok
}

func (s *fsSite) All() []Page {
	out := make([]Page, 0, len(s.pages))
	for _, p := range s.pages {
		if !s.drafts && p.Meta().Draft {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *fsSite) ByTag(tag string) []Page {
	var out []Page
	for _, p := range s.pages {
		if !s.drafts && p.Meta().Draft {
			continue
		}
		for _, t := range p.Meta().Tags {
			if t == tag {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func (s *fsSite) ByCollection(name string) []Page {
	var out []Page
	for _, p := range s.pages {
		if !s.drafts && p.Meta().Draft {
			continue
		}
		if p.Meta().Collection == name {
			out = append(out, p)
		}
	}
	return out
}

// SortByDate returns a new slice sorted by Meta.Date descending (newest first).
// Pages with a zero Date sort after all dated pages.
func SortByDate(pages []Page) []Page {
	out := make([]Page, len(pages))
	copy(out, pages)
	slices.SortStableFunc(out, func(a, b Page) int {
		ad, bd := a.Meta().Date, b.Meta().Date
		switch {
		case ad.IsZero() && bd.IsZero():
			return 0
		case ad.IsZero():
			return 1 // undated goes last
		case bd.IsZero():
			return -1
		case ad.After(bd):
			return -1 // descending
		case bd.After(ad):
			return 1
		}
		return 0
	})
	return out
}

// Paginate returns the pageNum-th page of results (1-indexed) with pageSize
// items per page. The boolean indicates whether more pages follow.
func Paginate(pages []Page, pageSize, pageNum int) ([]Page, bool) {
	if pageSize <= 0 || pageNum <= 0 {
		return []Page{}, false
	}
	offset := (pageNum - 1) * pageSize
	if offset >= len(pages) {
		return []Page{}, false
	}
	end := offset + pageSize
	hasMore := end < len(pages)
	if end > len(pages) {
		end = len(pages)
	}
	return pages[offset:end], hasMore
}

// discoverLayouts looks for a _layouts/ directory in fsys and parses it.
// Returns nil layout (and no error) if the directory is absent.
func discoverLayouts(fsys fs.FS) (*Layout, error) {
	info, err := fs.Stat(fsys, "_layouts")
	if err != nil || !info.IsDir() {
		return nil, nil // absent or not a directory — fine
	}

	var files []string
	if err := fs.WalkDir(fsys, "_layouts", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files = append(files, p)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("page: walk _layouts: %w", err)
	}

	if len(files) == 0 {
		return nil, nil
	}
	return NewLayout(fsys, files...)
}

type indexEntry struct {
	page     Page
	priority int // 1 = index.html, 2 = index.htm, 3 = index.md
}

// buildPageIndex walks fsys and builds the URL-path → Page map. Index files
// compete per directory; the highest-priority winner is kept.
func buildPageIndex(fsys fs.FS) (map[string]Page, error) {
	pages := make(map[string]Page)
	dirIndexes := make(map[string]indexEntry) // dir URL path → best index entry

	err := fs.WalkDir(fsys, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		urlPath, isIndex, priority := fileURLPath(filePath)

		pg, err := newPageFromFS(urlPath, fsys, filePath)
		if err != nil {
			return fmt.Errorf("page: %s: %w", filePath, err)
		}
		if pg == nil {
			return nil // unrecognised file type
		}

		if isIndex {
			if existing, ok := dirIndexes[urlPath]; !ok || priority < existing.priority {
				dirIndexes[urlPath] = indexEntry{page: pg, priority: priority}
			}
		} else {
			pages[urlPath] = pg
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for urlPath, entry := range dirIndexes {
		pages[urlPath] = entry.page
	}
	return pages, nil
}

// newPageFromFS constructs a Page from the FS using the correct internal
// constructor so the page can re-read its body at render time.
func newPageFromFS(urlPath string, fsys fs.FS, filePath string) (Page, error) {
	switch ext := fileExt(filePath); ext {
	case ".md":
		return newMarkdownPageFromFS(urlPath, fsys, filePath)
	case ".html", ".htm":
		pg, err := newHTMLPageFromFS(urlPath, fsys, filePath)
		if err != nil || pg == nil {
			return nil, err
		}
		return pg, nil
	default:
		return nil, nil
	}
}

// deriveSlug returns the last path segment of urlPath with content extensions
// stripped. For directory paths (trailing slash) it returns the directory name.
// Returns "" for the root path.
func deriveSlug(urlPath string) string {
	base := path.Base(urlPath)
	if base == "/" || base == "." {
		return ""
	}
	ext := path.Ext(base)
	switch ext {
	case ".md", ".html", ".htm":
		return strings.TrimSuffix(base, ext)
	}
	return base
}

// fileURLPath derives the URL path and index priority for a file path within an
// fs.FS. Index files (index.html, index.htm, index.md) map to the parent
// directory path with a trailing slash; all other files keep their extension.
// priority is 0 for non-index files; lower values beat higher for index files.
func fileURLPath(filePath string) (urlPath string, isIndex bool, priority int) {
	dir := path.Dir(filePath)
	base := path.Base(filePath)

	dirURL := "/"
	if dir != "." {
		dirURL = "/" + dir + "/"
	}

	switch base {
	case "index.html":
		return dirURL, true, 1
	case "index.htm":
		return dirURL, true, 2
	case "index.md":
		return dirURL, true, 3
	}
	return "/" + filePath, false, 0
}

func fileExt(filePath string) string {
	for i := len(filePath) - 1; i >= 0; i-- {
		if filePath[i] == '.' {
			return filePath[i:]
		}
		if filePath[i] == '/' {
			break
		}
	}
	return ""
}

// FileRenderer returns an endpoint.FileRendererHook that looks up the URL path
// in the site index and, if found, returns a Renderer that calls page.Render.
// File ownership transfers to the hook on a non-nil return; on nil, nil the
// hook does not read or close the file.
func (s *fsSite) FileRenderer() endpoint.FileRendererHook {
	return func(urlPath string, f fs.File) (endpoint.Renderer, error) {
		pg, ok := s.Get(urlPath)
		if !ok {
			return nil, nil
		}
		f.Close() // page re-reads from its own FS reference at render time
		return pg.Renderer(s, s.layout)
	}
}

// DirRenderer returns an endpoint.FileRendererHook for directory requests.
// Index files (index.html > index.htm > index.md) are registered under the
// parent directory path at NewSite time, so the hook calls site.Get(urlPath)
// with no ambiguity. On nil, nil the hook does not call ReadDir on the file.
func (s *fsSite) DirRenderer() endpoint.FileRendererHook {
	return func(urlPath string, f fs.File) (endpoint.Renderer, error) {
		pg, ok := s.Get(urlPath)
		if !ok {
			return nil, nil
		}
		f.Close() // page re-reads from its own FS reference at render time
		return pg.Renderer(s, s.layout)
	}
}
