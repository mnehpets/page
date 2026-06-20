package page

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mnehpets/http/endpoint"
)

// SiteRenderer is returned by NewSite and used by the serve layer to wire up
// rendering. It provides the HTTP renderer hooks.
type SiteRenderer interface {
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

// Refreshable is implemented by sites that support incremental index refresh.
// Callers that need live-reload capability should type-assert the SiteIndex
// returned by NewSite to Refreshable.
type Refreshable interface {
	UpdateFile(filePath string) error
	DeleteFile(filePath string)
}

// fileRecord holds the change-detection fingerprint for a single content file.
type fileRecord struct {
	sitePath string
	modTime  time.Time
}

type fsSite struct {
	mu       sync.RWMutex
	pages    map[string]Page   // sitePath → Page (all pages, including drafts)
	children map[string][]Page // parent sitePath → direct child pages (all, including drafts)
	layout   *Layout
	drafts   bool // include drafts in query results
	config   SiteConfig

	// refresh support — immutable after construction
	fsys               fs.FS
	autoDiscoverLayout bool
	fileMeta           map[string]fileRecord // fsFilePath → {sitePath, modTime}
}

// NewSite walks fsys, parses all .md, .html, and .htm files, and returns a
// SiteIndex ready to wire up HTTP rendering. _layouts/ is automatically parsed
// unless WithLayout is provided.
func NewSite(fsys fs.FS, opts ...SiteOption) (SiteRenderer, error) {
	cfg := &siteConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	layout := cfg.layout
	autoDiscover := layout == nil
	if autoDiscover {
		l, err := discoverLayouts(fsys)
		if err != nil {
			return nil, err
		}
		layout = l
	}

	pages, fileMeta, err := buildPageIndex(fsys)
	if err != nil {
		return nil, err
	}

	children := buildChildIndex(pages)

	return &fsSite{
		pages:              pages,
		children:           children,
		layout:             layout,
		drafts:             cfg.includeDrafts,
		config:             cfg.config,
		fsys:               fsys,
		autoDiscoverLayout: autoDiscover,
		fileMeta:           fileMeta,
	}, nil
}

func (s *fsSite) Config() SiteConfig { return s.config }

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

// discoverLayouts looks for a _layouts/ directory in fsys and builds a Layout.
// Returns nil layout (and no error) if the directory is absent or empty.
//
// Files directly in _layouts/ are entry-point templates; their name without
// extension becomes the layout name (e.g. _layouts/default.html → "default").
//
// Files in _layouts/base/ are common base templates parsed into every layout's
// template set. When present, the "baseof" template they define is the
// execution entry point and entry-point templates should define {{define "main"}}.
// When absent, each layout is compiled alone and executed by its own template name.
func discoverLayouts(fsys fs.FS) (*Layout, error) {
	info, err := fs.Stat(fsys, "_layouts")
	if err != nil || !info.IsDir() {
		return nil, nil // absent or not a directory — fine
	}

	// Base templates: files directly inside _layouts/base/ (one level only).
	var basePatterns []string
	if baseInfo, err := fs.Stat(fsys, "_layouts/base"); err == nil && baseInfo.IsDir() {
		basePatterns = []string{"_layouts/base/*"}
	}

	// Entry-point templates: non-directory entries directly inside _layouts/.
	entries, err := fs.ReadDir(fsys, "_layouts")
	if err != nil {
		return nil, fmt.Errorf("page: read _layouts: %w", err)
	}
	var layoutFiles []string
	for _, e := range entries {
		if !e.IsDir() {
			layoutFiles = append(layoutFiles, "_layouts/"+e.Name())
		}
	}
	if len(layoutFiles) == 0 {
		return nil, nil
	}

	return NewLayout(fsys, basePatterns, layoutFiles)
}

type indexEntry struct {
	page     Page
	priority int // 1 = index.html, 2 = index.htm, 3 = README.md
}

// buildPageIndex walks fsys and builds the URL-path → Page map. Index files
// compete per directory; the highest-priority winner is kept.
// It also returns a fileMeta map recording the fsFilePath → {sitePath, modTime}
// fingerprint for every recognised file, used for incremental refresh.
func buildPageIndex(fsys fs.FS) (map[string]Page, map[string]fileRecord, error) {
	pages := make(map[string]Page)
	fileMeta := make(map[string]fileRecord)
	dirIndexes := make(map[string]indexEntry) // dir URL path → best index entry

	err := fs.WalkDir(fsys, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		sitePath, isIndex, priority := fileSitePath(filePath)

		pg, err := newPageFromFS(sitePath, fsys, filePath)
		if err != nil {
			return fmt.Errorf("page: %s: %w", filePath, err)
		}
		if pg == nil {
			return nil // unrecognised file type
		}

		fileMeta[filePath] = fileRecord{sitePath: sitePath, modTime: info.ModTime()}

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
		return nil, nil, err
	}

	for sitePath, entry := range dirIndexes {
		pages[sitePath] = entry.page
	}
	return pages, fileMeta, nil
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
// path within an fs.FS. Index files (index.html, index.htm, README.md) map to
// their parent directory path (e.g. "blog/README.md" → "blog", "README.md" →
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
	case "README.md":
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

// site is the template API type passed to page renderers. It holds a pointer
// to *fsSite and reads its fields directly; safety relies on the caller
// (FileRenderer/DirRenderer) holding s.mu.RLock for the duration of the render
// via lockedRenderer. All query methods called from templates therefore see a
// consistent index without their own locks — avoiding both per-call overhead
// and the recursive-RLock hazard that would arise if templates called *fsSite
// methods while lockedRenderer already holds the read lock.
type site struct{ s *fsSite }

func (v *site) Get(sitePath string) Page { return v.s.pages[sitePath] }

func (v *site) All() []Page {
	out := make([]Page, 0, len(v.s.pages))
	for _, p := range v.s.pages {
		if !v.s.drafts && p.Meta().Draft {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (v *site) ByTag(tag string) []Page {
	var out []Page
	for _, p := range v.s.pages {
		if !v.s.drafts && p.Meta().Draft {
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

func (v *site) ByCollection(name string) []Page {
	var out []Page
	for _, p := range v.s.pages {
		if !v.s.drafts && p.Meta().Draft {
			continue
		}
		if p.Meta().Collection == name {
			out = append(out, p)
		}
	}
	return out
}

func (v *site) AncestorsOf(sitePath string) []Page {
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
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	var out []Page
	for _, p := range paths {
		if pg, ok := v.s.pages[p]; ok {
			out = append(out, pg)
		}
	}
	return out
}

func (v *site) SiblingsOf(sitePath string) []Page {
	par := parentPath(sitePath)
	if par == "" {
		if pg, ok := v.s.pages[sitePath]; ok && (v.s.drafts || !pg.Meta().Draft) {
			return []Page{pg}
		}
		return nil
	}
	return v.ChildrenOf(par)
}

func (v *site) ChildrenOf(sitePath string) []Page {
	all := v.s.children[sitePath]
	if v.s.drafts {
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

func (v *site) Config() SiteConfig { return v.s.config }

// lockedRenderer wraps a Renderer and releases a read lock after Render returns.
// The read lock is acquired in FileRenderer/DirRenderer and held across the
// entire render pipeline — covering page lookup, layout selection, and template
// execution — so all query methods called from templates see a consistent
// index. Templates receive a *site (not *fsSite), so no recursive-RLock
// hazard arises.
type lockedRenderer struct {
	mu    sync.Locker // s.mu.RLocker(); Unlock() releases the read lock
	inner endpoint.Renderer
}

func (lr *lockedRenderer) Render(w http.ResponseWriter, r *http.Request) error {
	defer lr.mu.Unlock()
	return lr.inner.Render(w, r)
}

// lockedRendererHook builds an endpoint.Renderer for a resolved site path while holding a
// read lock through template execution via lockedRenderer.
func (s *fsSite) lockedRendererHook(filePath string, fsys fs.FS, f fs.File) (endpoint.Renderer, error) {
	s.mu.RLock()
	pg := s.pages[filePath]
	if pg == nil {
		s.mu.RUnlock()
		return nil, nil
	}
	view := &site{s: s}
	f.Close() // page re-reads from its own FS reference at render time
	renderer, err := pg.Renderer(view, s.layout)
	if err != nil {
		s.mu.RUnlock()
		return nil, err
	}
	return &lockedRenderer{mu: s.mu.RLocker(), inner: renderer}, nil
}

// FileRenderer returns an endpoint.FileRendererHook that looks up the URL path
// in the site index and, if found, returns a Renderer that calls page.Render.
// File ownership transfers to the hook on a non-nil return; on nil, nil the
// hook does not read or close the file.
func (s *fsSite) FileRenderer() endpoint.FileRendererHook {
	return s.lockedRendererHook
}

// DirRenderer returns an endpoint.FileRendererHook for directory requests.
// Index files (index.html > index.htm > README.md) are registered under the
// parent directory path at NewSite time, so the hook calls site.Get(path)
// with no ambiguity. On nil, nil the hook does not call ReadDir on the file.
func (s *fsSite) DirRenderer() endpoint.FileRendererHook {
	return s.lockedRendererHook
}

// indexCandidate pairs a filesystem path with its directory-index priority.
type indexCandidate struct {
	filePath string
	priority int
}

// indexCandidates returns the potential index filePaths for dirSitePath in
// priority order (lowest priority number = highest precedence).
func indexCandidates(dirSitePath string) []indexCandidate {
	if dirSitePath == "." {
		return []indexCandidate{
			{"index.html", 1},
			{"index.htm", 2},
			{"README.md", 3},
		}
	}
	return []indexCandidate{
		{dirSitePath + "/index.html", 1},
		{dirSitePath + "/index.htm", 2},
		{dirSitePath + "/README.md", 3},
	}
}

// Refresh walks the filesystem using stat-only calls, re-parses only files
// whose ModTime has changed, adds new files, and removes deleted files.
// It returns the number of files re-parsed (new + changed).
// On any parse error the index is left unchanged and the error is returned.
func (s *fsSite) Refresh() (int, error) {
	// Snapshot fileMeta under a brief read lock so the walk is lock-free.
	s.mu.RLock()
	snapshot := make(map[string]fileRecord, len(s.fileMeta))
	for k, v := range s.fileMeta {
		snapshot[k] = v
	}
	s.mu.RUnlock()

	// Walk FS: collect files whose mtime changed or are new.
	type pendingFile struct {
		filePath string
		sitePath string
		isIndex  bool
		priority int
		modTime  time.Time
	}
	seen := make(map[string]struct{})
	var toProcess []pendingFile

	if err := fs.WalkDir(s.fsys, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		seen[filePath] = struct{}{}
		info, err := d.Info()
		if err != nil {
			return err
		}
		existing, known := snapshot[filePath]
		if known && info.ModTime().Equal(existing.modTime) {
			return nil // unchanged
		}
		sp, isIdx, pri := fileSitePath(filePath)
		toProcess = append(toProcess, pendingFile{filePath, sp, isIdx, pri, info.ModTime()})
		return nil
	}); err != nil {
		return 0, err
	}

	// Detect deleted files and, for deleted index files, force-re-evaluate
	// sibling candidates so the next-best index can be promoted.
	var deleted []string
	for filePath := range snapshot {
		if _, ok := seen[filePath]; ok {
			continue
		}
		deleted = append(deleted, filePath)
		rec := snapshot[filePath]
		_, isIdx, _ := fileSitePath(filePath)
		if !isIdx {
			continue
		}
		// Force-check remaining candidates for this directory index.
		for _, cand := range indexCandidates(rec.sitePath) {
			if cand.filePath == filePath {
				continue // this is the one being deleted
			}
			if _, alreadySeen := seen[cand.filePath]; !alreadySeen {
				continue // also deleted or never existed
			}
			alreadyPending := false
			for _, p := range toProcess {
				if p.filePath == cand.filePath {
					alreadyPending = true
					break
				}
			}
			if alreadyPending {
				continue
			}
			// Re-parse this candidate even if its mtime hasn't changed,
			// so it can take over as the directory index.
			if rec2, ok := snapshot[cand.filePath]; ok {
				toProcess = append(toProcess, pendingFile{
					filePath: cand.filePath,
					sitePath: rec.sitePath,
					isIndex:  true,
					priority: cand.priority,
					modTime:  rec2.modTime,
				})
			}
		}
	}

	if len(toProcess) == 0 && len(deleted) == 0 {
		return 0, nil
	}

	// Parse changed/new files outside the lock.
	type parsedFile struct {
		pendingFile
		pg Page
	}
	parsed := make([]parsedFile, 0, len(toProcess))
	for _, f := range toProcess {
		pg, err := newPageFromFS(f.sitePath, s.fsys, f.filePath)
		if err != nil {
			return 0, fmt.Errorf("page: refresh %s: %w", f.filePath, err)
		}
		if pg != nil {
			parsed = append(parsed, parsedFile{f, pg})
		}
	}

	// Optionally re-discover layouts outside the lock.
	var newLayout *Layout
	if s.autoDiscoverLayout {
		var err error
		newLayout, err = discoverLayouts(s.fsys)
		if err != nil {
			return 0, fmt.Errorf("page: refresh layouts: %w", err)
		}
	}

	// Apply all changes under the write lock.
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, filePath := range deleted {
		rec := snapshot[filePath]
		delete(s.pages, rec.sitePath)
		delete(s.fileMeta, filePath)
	}

	// Resolve index file priority for any directory that has competing updates.
	dirIndexes := make(map[string]indexEntry)
	count := 0
	for _, r := range parsed {
		s.fileMeta[r.filePath] = fileRecord{sitePath: r.sitePath, modTime: r.modTime}
		if !r.isIndex {
			s.pages[r.sitePath] = r.pg
			count++
			continue
		}
		if existing, ok := dirIndexes[r.sitePath]; !ok || r.priority < existing.priority {
			dirIndexes[r.sitePath] = indexEntry{page: r.pg, priority: r.priority}
		}
		count++
	}
	for sitePath, entry := range dirIndexes {
		s.pages[sitePath] = entry.page
	}

	if s.autoDiscoverLayout && newLayout != nil {
		s.layout = newLayout
	}

	s.children = buildChildIndex(s.pages)
	return count, nil
}

// UpdateFile re-parses a single file and updates its entry in the index.
// If filePath is under _layouts/, layout templates are re-discovered.
// Intended for use by fsnotify write/create event handlers.
func (s *fsSite) UpdateFile(filePath string) error {
	sitePath, isIndex, priority := fileSitePath(filePath)

	info, err := fs.Stat(s.fsys, filePath)
	if err != nil {
		return fmt.Errorf("page: stat %s: %w", filePath, err)
	}

	pg, err := newPageFromFS(sitePath, s.fsys, filePath)
	if err != nil {
		return fmt.Errorf("page: update %s: %w", filePath, err)
	}

	var newLayout *Layout
	if s.autoDiscoverLayout && strings.HasPrefix(filePath, "_layouts/") {
		newLayout, err = discoverLayouts(s.fsys)
		if err != nil {
			return fmt.Errorf("page: update layouts: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if pg != nil {
		if isIndex {
			// Keep the entry only if this file wins or ties priority.
			if existing, ok := s.fileMeta[filePath]; ok {
				_ = existing // fileMeta updated below
			}
			// Check current winner's priority via dirIndexes scan.
			bestPri := priority
			for _, cand := range indexCandidates(sitePath) {
				if cand.filePath == filePath {
					continue
				}
				if _, ok := s.fileMeta[cand.filePath]; ok && cand.priority < bestPri {
					bestPri = cand.priority
				}
			}
			if priority <= bestPri {
				s.pages[sitePath] = pg
			}
		} else {
			s.pages[sitePath] = pg
		}
		s.fileMeta[filePath] = fileRecord{sitePath: sitePath, modTime: info.ModTime()}
	}

	if newLayout != nil {
		s.layout = newLayout
	}

	s.children = buildChildIndex(s.pages)
	return nil
}

// DeleteFile removes a single file's entry from the index.
// If the file was a directory index, the next-best candidate is promoted.
// Intended for use by fsnotify remove/rename event handlers.
func (s *fsSite) DeleteFile(filePath string) {
	s.mu.RLock()
	rec, ok := s.fileMeta[filePath]
	s.mu.RUnlock()
	if !ok {
		return
	}

	_, isIndex, _ := fileSitePath(filePath)

	// If an index file is deleted, find the next-best candidate outside the lock.
	var fallbackPg Page
	if isIndex {
		for _, cand := range indexCandidates(rec.sitePath) {
			if cand.filePath == filePath {
				continue
			}
			if _, statErr := fs.Stat(s.fsys, cand.filePath); statErr != nil {
				continue // doesn't exist
			}
			pg, err := newPageFromFS(rec.sitePath, s.fsys, cand.filePath)
			if err == nil && pg != nil {
				fallbackPg = pg
				break
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.pages, rec.sitePath)
	delete(s.fileMeta, filePath)

	if fallbackPg != nil {
		s.pages[rec.sitePath] = fallbackPg
	}

	s.children = buildChildIndex(s.pages)
}
