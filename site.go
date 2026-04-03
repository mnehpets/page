package page

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/mnehpets/http/endpoint"
)

// Site indexes pages by URL path and exposes query methods for use in templates.
type Site interface {
	Get(sitePath string) (Page, bool)
	All() []Page
	ByTag(tag string) []Page
	ByCollection(name string) []Page
	// AncestorsOf returns the pages on the path from the root down to — but not
	// including — sitePath, ordered from shallowest to deepest (root first).
	// Pages for intermediate site paths absent from the site index are skipped.
	AncestorsOf(sitePath string) []Page
	// ChildrenOf returns pages that are exactly one path segment deeper than
	// sitePath, i.e. pages whose parent site path equals sitePath.
	ChildrenOf(sitePath string) []Page
	Config() SiteConfig
	FileRenderer() endpoint.FileRendererHook
	DirRenderer() endpoint.FileRendererHook
}

// SiteConfig holds operator-supplied configuration for the site.
// It is available to all templates via .Config.
type SiteConfig struct {
	BaseURL string // canonical base URL, e.g. "https://example.com" (no trailing slash)
	Name    string // site name, e.g. "My Blog"
	Lang    string // BCP 47 language tag, e.g. "en"; defaults to "en" if empty
}

// SiteOption is a functional option for NewSite.
type SiteOption func(*siteConfig)

type siteConfig struct {
	layout        *Layout
	includeDrafts bool
	config        SiteConfig
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

// WithConfig sets the operator-supplied site configuration made available to
// all templates via .Config.
func WithConfig(cfg SiteConfig) SiteOption {
	return func(c *siteConfig) { c.config = cfg }
}

type fsSite struct {
	pages    map[string]Page   // sitePath → Page (all pages, including drafts)
	children map[string][]Page // parent sitePath → direct child pages (all, including drafts)
	layout   *Layout
	drafts   bool // include drafts in query results
	config   SiteConfig
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

	children := buildChildIndex(pages)

	return &fsSite{pages: pages, children: children, layout: layout, drafts: cfg.includeDrafts, config: cfg.config}, nil
}

// Layout returns the site's layout, which may be nil if no _layouts/ directory
// was found and no WithLayout option was provided.
func (s *fsSite) Layout() *Layout { return s.layout }

func (s *fsSite) Config() SiteConfig { return s.config }

func (s *fsSite) Get(sitePath string) (Page, bool) {
	p, ok := s.pages[sitePath]
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

// AncestorsOf returns the pages on the path from the root down to (but not
// including) sitePath, ordered root-first. Intermediate paths absent from the
// site index are skipped silently.
func (s *fsSite) AncestorsOf(sitePath string) []Page {
	var paths []string
	cur := sitePath
	for {
		par := parentPath(cur)
		if par == "" || par == cur {
			break
		}
		paths = append(paths, par)
		cur = par
	}
	// paths is child-to-root; reverse to root-first.
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	var out []Page
	for _, p := range paths {
		if pg, ok := s.pages[p]; ok {
			out = append(out, pg)
		}
	}
	return out
}

// ChildrenOf returns pages that are exactly one path segment deeper than
// sitePath. Draft pages are excluded unless WithIncludeDrafts was used.
func (s *fsSite) ChildrenOf(sitePath string) []Page {
	all := s.children[sitePath]
	if s.drafts {
		out := make([]Page, len(all))
		copy(out, all)
		return out
	}
	out := make([]Page, 0, len(all))
	for _, p := range all {
		if !p.Meta().Draft {
			out = append(out, p)
		}
	}
	return out
}

// buildChildIndex builds a parent-sitePath → direct-children map from the page
// index. All pages (including drafts) are stored; draft filtering is applied at
// query time.
func buildChildIndex(pages map[string]Page) map[string][]Page {
	children := make(map[string][]Page)
	for sitePath, pg := range pages {
		parent := parentPath(sitePath)
		if parent != "" {
			children[parent] = append(children[parent], pg)
		}
	}
	return children
}

// parentPath returns the parent of p in the site-relative path scheme.
//
//	parentPath("a/b/c.md") → "a/b"
//	parentPath("a/b")      → "a"
//	parentPath("a.md")     → "."
//	parentPath(".")        → ""
func parentPath(p string) string {
	if p == "." {
		return ""
	}
	return path.Dir(p)
}

// relURL returns a relative URL from the page at fromPath to the page at
// toPath. Both are site-relative paths (as returned by Page.SitePath).
// Directory pages (no file extension, including the root ".") are linked with
// a trailing slash so the browser resolves further relative links correctly.
func relURL(fromPath, toPath string) string {
	// Directory pages (no extension, root ".") are their own directory.
	// File pages use their containing directory.
	var fromDir string
	if isDirPath(fromPath) {
		fromDir = fromPath
	} else {
		fromDir = path.Dir(fromPath)
	}

	fromSegs := splitPath(fromDir)
	toSegs := splitPath(toPath)

	// Find length of common prefix.
	n := min(len(fromSegs), len(toSegs))
	common := 0
	for common < n && fromSegs[common] == toSegs[common] {
		common++
	}

	// Go up from fromDir to the common ancestor, then down to toPath.
	parts := make([]string, 0, len(fromSegs)-common+len(toSegs)-common)
	for range fromSegs[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, toSegs[common:]...)

	rel := strings.Join(parts, "/")
	if rel == "" {
		rel = "."
	}
	if isDirPath(toPath) {
		rel += "/"
	}
	return rel
}

// absURL joins a canonical base URL and a site-relative path into an absolute
// URL. The root path "." maps to a trailing slash. Directory paths are
// emitted with a trailing slash.
func absURL(baseURL, sitePath string) string {
	base := strings.TrimRight(baseURL, "/")
	p := strings.TrimPrefix(sitePath, "/")
	if p == "" || p == "." {
		return base + "/"
	}
	if isDirPath(p) {
		return base + "/" + p + "/"
	}
	return base + "/" + p
}

// isDirPath reports whether p is a directory-type site-relative path: either
// the root "." or any path whose last segment has no file extension.
// path.Ext(".") returns "." (not ""), so "." must be handled explicitly.
func isDirPath(p string) bool {
	return p == "." || path.Ext(path.Base(p)) == ""
}

// splitPath splits a site-relative path into segments.
// The root path "." and empty string both return nil.
func splitPath(p string) []string {
	if p == "." || p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// SortByPath returns a new slice sorted lexicographically by SitePath ascending.
func SortByPath(pages []Page) []Page {
	out := make([]Page, len(pages))
	copy(out, pages)
	slices.SortStableFunc(out, func(a, b Page) int {
		return strings.Compare(a.SitePath(), b.SitePath())
	})
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

// SortByLastMod returns a new slice sorted by Meta.LastMod descending (most recently modified first).
// Pages with a zero LastMod sort after all others.
func SortByLastMod(pages []Page) []Page {
	out := make([]Page, len(pages))
	copy(out, pages)
	slices.SortStableFunc(out, func(a, b Page) int {
		am, bm := a.Meta().LastMod, b.Meta().LastMod
		switch {
		case am.IsZero() && bm.IsZero():
			return 0
		case am.IsZero():
			return 1
		case bm.IsZero():
			return -1
		case am.After(bm):
			return -1 // descending
		case bm.After(am):
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

		sitePath, isIndex, priority := fileSitePath(filePath)

		pg, err := newPageFromFS(sitePath, fsys, filePath)
		if err != nil {
			return fmt.Errorf("page: %s: %w", filePath, err)
		}
		if pg == nil {
			return nil // unrecognised file type
		}

		if isIndex {
			if existing, ok := dirIndexes[sitePath]; !ok || priority < existing.priority {
				dirIndexes[sitePath] = indexEntry{page: pg, priority: priority}
			}
		} else {
			pages[sitePath] = pg
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for sitePath, entry := range dirIndexes {
		pages[sitePath] = entry.page
	}
	return pages, nil
}

// newPageFromFS constructs a Page from the FS using the correct internal
// constructor so the page can re-read its body at render time.
func newPageFromFS(sitePath string, fsys fs.FS, filePath string) (Page, error) {
	switch ext := fileExt(filePath); ext {
	case ".md":
		return newMarkdownPageFromFS(sitePath, fsys, filePath)
	case ".html", ".htm", ".xml":
		pg, err := newHTMLPageFromFS(sitePath, fsys, filePath)
		if err != nil || pg == nil {
			return nil, err
		}
		return pg, nil
	default:
		return nil, nil
	}
}

// deriveSlug returns the last path segment of sitePath with content extensions
// stripped. Returns "" for the root path ".".
func deriveSlug(sitePath string) string {
	if sitePath == "." {
		return ""
	}
	base := path.Base(sitePath)
	ext := path.Ext(base)
	switch ext {
	case ".md", ".html", ".htm", ".xml":
		return strings.TrimSuffix(base, ext)
	}
	return base
}

// fileSitePath derives the site-relative path and index priority for a file
// path within an fs.FS. Index files (index.html, index.htm, index.md) map to
// their parent directory path (e.g. "blog/index.md" → "blog", "index.md" →
// "."); all other files keep their full relative path.
// priority is 0 for non-index files; lower values beat higher for index files.
func fileSitePath(filePath string) (sitePath string, isIndex bool, priority int) {
	dir := path.Dir(filePath)
	base := path.Base(filePath)
	switch base {
	case "index.html":
		return dir, true, 1
	case "index.htm":
		return dir, true, 2
	case "index.md":
		return dir, true, 3
	}
	return filePath, false, 0
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
	return func(filePath string, fsys fs.FS, f fs.File) (endpoint.Renderer, error) {
		pg, ok := s.Get(filePath)
		if !ok {
			return nil, nil
		}
		f.Close() // page re-reads from its own FS reference at render time
		return pg.Renderer(s, s.layout)
	}
}

// DirRenderer returns an endpoint.FileRendererHook for directory requests.
// Index files (index.html > index.htm > index.md) are registered under the
// parent directory path at NewSite time, so the hook calls site.Get(path)
// with no ambiguity. On nil, nil the hook does not call ReadDir on the file.
func (s *fsSite) DirRenderer() endpoint.FileRendererHook {
	return func(filePath string, fsys fs.FS, f fs.File) (endpoint.Renderer, error) {
		pg, ok := s.Get(filePath)
		if !ok {
			return nil, nil
		}
		f.Close() // page re-reads from its own FS reference at render time
		return pg.Renderer(s, s.layout)
	}
}
