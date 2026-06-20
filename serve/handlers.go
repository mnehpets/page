package pageserve

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	mnfs "github.com/mnehpets/fs"
	"github.com/mnehpets/http/endpoint"
	"github.com/mnehpets/page"
	"github.com/zserge/metric"
	"gopkg.in/yaml.v3"
)

// Endpoint is the common return type for HandlerBuilder.Build.
// build.go is the sole site that wraps an Endpoint with processors
// and registers the resulting http.Handler on the mux.
//
// Ideally we would use a generic Endpoint type like endpoint.EndpointFunc[T any],
// however, the common HandlerFactory map would need to have dynamic types
// so that the call to endpoint.HandleFunc() in Build() gets the correct param type.
// For now, the only handlers that use params are pages and files, and they
// both use endpoint.FileSystemParams,
type Params = endpoint.FileSystemParams
type Endpoint = endpoint.EndpointFunc[Params]

// RouteEntry pairs a ServeMux pattern with the endpoint to register at that pattern.
// HandlerBuilder.Build returns one entry for most handlers, or multiple for handlers
// (like auth) that own several related endpoints under the same route prefix.
type RouteEntry struct {
	Pattern  string
	Endpoint Endpoint
}

// HandlerBuilder is returned by a HandlerFactory. It holds the decoded config
// for a single route and knows how to validate and build its Endpoint.
type HandlerBuilder interface {
	Validate(cfg Config) error
	// Build constructs the route's endpoints. routePath is the ServeMux pattern
	// from RouteConfig.Path; builders use it to compute sub-patterns for any
	// additional entries they register (e.g. logout, me under an auth prefix).
	Build(cfg Config, srv *Server, routePath string) ([]RouteEntry, error)
}

// HandlerFactory parses a route's YAML node into a HandlerBuilder.
// Both built-in and custom handlers use this type.
type HandlerFactory func(node *yaml.Node) (HandlerBuilder, error)

// --- pages ---

type pagesBuilder struct {
	Path          string `yaml:"path"`
	Dir           string `yaml:"dir"`
	IncludeDrafts bool   `yaml:"include_drafts"`
	DirList       bool   `yaml:"dir_list"`
	Dotfiles      bool   `yaml:"dotfiles"`
	Symlinks      bool   `yaml:"symlinks"`
	Watch         bool   `yaml:"watch"`
}

func (b *pagesBuilder) Validate(cfg Config) error { return nil }

func (b *pagesBuilder) Build(cfg Config, srv *Server, routePath string) ([]RouteEntry, error) {
	dir := b.Dir
	if dir == "" {
		dir = "."
	}
	fsys := os.DirFS(dir)

	opts := []page.SiteOption{
		page.WithConfig(page.SiteConfig{
			BaseURL: cfg.Site.BaseURL,
			Name:    cfg.Site.Name,
			Lang:    cfg.Site.Lang,
		}),
	}
	if b.IncludeDrafts {
		opts = append(opts, page.WithIncludeDrafts())
	}

	site, err := page.NewSite(fsys, opts...)
	if err != nil {
		return nil, fmt.Errorf("pages (path=%q): %w", b.Path, err)
	}

	// Per-route counter for refreshed files, mirroring the requests counter in
	// wrapWithStats. Visible at /debug/vars under pageserve.route.<name>.refreshed_files.
	refreshed := metric.NewCounter("15m1m", "1h5m", "24h1h")
	expvarMap("pageserve.route."+sanitizeExpvarName(routePath)).Set("refreshed_files", refreshed)

	if b.Watch {
		if r, ok := site.(page.Refreshable); ok {
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return nil, fmt.Errorf("pages (path=%q): watch abs dir: %w", b.Path, err)
			}
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				return nil, fmt.Errorf("pages (path=%q): watch: %w", b.Path, err)
			}
			// Add the root and every existing subdirectory — fsnotify is non-recursive.
			if err := addWatchDirs(watcher, absDir); err != nil {
				watcher.Close()
				return nil, fmt.Errorf("pages (path=%q): watch %s: %w", b.Path, absDir, err)
			}
			go runWatcher(srv.ctx, watcher, absDir, r, refreshed)
		}
	}

	// Block the entire _layouts/ tree (including subdirs like _layouts/base/)
	// from being served as static files.
	noLayouts := mnfs.WithRule(func(p string, _ func() (fs.FileInfo, error)) (mnfs.FilterMode, error) {
		if p == "_layouts" || strings.HasPrefix(p, "_layouts/") {
			return mnfs.Disallowed, nil
		}
		return 0, nil
	})
	serveFS := &filteredFS{
		base:     dir,
		dotfiles: b.Dotfiles,
		symlinks: b.Symlinks,
	}
	public := mnfs.NewFilterFS(serveFS, noLayouts)

	ep := (&endpoint.FileSystem{
		FS: func(_ context.Context, _ *http.Request) (fs.FS, error) {
			return public, nil
		},
		IndexHTML:        true,
		DirectoryListing: b.DirList,
		DirTemplate:      endpoint.FancyDirTemplate,
		FileRenderer:     site.FileRenderer(),
		DirRenderer:      site.DirRenderer(),
	}).Endpoint
	return []RouteEntry{{Pattern: withSubPath(routePath), Endpoint: ep}}, nil
}

// addWatchDirs adds root and every subdirectory under it to w.
// fsnotify is non-recursive on Linux; we must add each directory explicitly.
//
// On macOS (kqueue) w.Add registers a watch on every child file to mimic
// inotify, so a directory containing a dangling symlink (e.g. a broken
// .venv/bin/python) fails with ErrNotExist. Such a directory is skipped
// silently rather than aborting the whole watch.
func addWatchDirs(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if err := w.Add(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // dangling symlink under this dir — skip silently
			}
			return err
		}
		return nil
	})
}

// runWatcher reads fsnotify events and calls UpdateFile/DeleteFile on the site
// with per-file debounce. It exits when ctx is done.
func runWatcher(ctx context.Context, w *fsnotify.Watcher, dir string, r page.Refreshable, counter metric.Metric) {
	defer w.Close()

	// debounce: filePath → pending timer
	timers := make(map[string]*time.Timer)
	var mu sync.Mutex

	fire := func(filePath string, remove bool) {
		mu.Lock()
		delete(timers, filePath)
		mu.Unlock()
		if remove {
			r.DeleteFile(filePath)
			counter.Add(1)
		} else {
			if err := r.UpdateFile(filePath); err == nil {
				counter.Add(1)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, t := range timers {
				t.Stop()
			}
			mu.Unlock()
			return
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			// New directory: add it so files created inside are watched too.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.Add(event.Name) //nolint:errcheck
					continue
				}
			}
			// Only process write/create/remove/rename; skip Chmod etc.
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) &&
				!event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
				continue
			}
			// Convert absolute path to FS-relative path.
			rel, err := filepath.Rel(dir, event.Name)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			// Normalise to forward slashes (Windows).
			rel = filepath.ToSlash(rel)

			remove := event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)

			mu.Lock()
			if t, exists := timers[rel]; exists {
				t.Stop()
			}
			timers[rel] = time.AfterFunc(400*time.Millisecond, func() {
				fire(rel, remove)
			})
			mu.Unlock()
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
			// Watcher errors are non-fatal; the site continues serving the last index.
		}
	}
}

func pagesHandlerFactory() HandlerFactory {
	return func(node *yaml.Node) (HandlerBuilder, error) {
		b := new(pagesBuilder)
		if node != nil {
			if err := node.Decode(b); err != nil {
				return nil, fmt.Errorf("pages: decode config: %w", err)
			}
		}
		return b, nil
	}
}

// --- files ---

type filesBuilder struct {
	Path      string `yaml:"path"`
	Dir       string `yaml:"dir"`
	IndexHTML *bool  `yaml:"index_html"`
	DirList   bool   `yaml:"dir_list"`
	Dotfiles  bool   `yaml:"dotfiles"`
	Symlinks  bool   `yaml:"symlinks"`
	// WebDAV, if true, additionally serves WebDAV PROPFIND (read-only file
	// metadata) and OPTIONS (DAV class-1 advertisement) alongside GET/HEAD.
	WebDAV bool `yaml:"webdav"`
}

func (b *filesBuilder) Validate(cfg Config) error {
	if b.Dir == "" {
		return fmt.Errorf("files (path=%q): dir is required", b.Path)
	}
	return nil
}

func (b *filesBuilder) Build(cfg Config, srv *Server, routePath string) ([]RouteEntry, error) {
	indexHTML := true
	if b.IndexHTML != nil {
		indexHTML = *b.IndexHTML
	}

	fsys := &filteredFS{
		base:     b.Dir,
		dotfiles: b.Dotfiles,
		symlinks: b.Symlinks,
	}

	get := staticFileEndpoint(fsys, indexHTML, b.DirList)

	pattern := withSubPath(routePath)
	if !b.WebDAV {
		// Plain static serving: one entry matching all methods.
		return []RouteEntry{{Pattern: pattern, Endpoint: get}}, nil
	}

	// WebDAV mode: dispatch methods through an inner ServeMux mounted at the same
	// pattern, rather than registering method-specific patterns on the main mux.
	// A method-specific pattern like "GET /{path...}" is incomparable to a
	// method-agnostic one like "/gen/{path...}" (narrower method, broader path),
	// which makes http.ServeMux panic on a routing conflict. Keeping the main-mux
	// pattern method-agnostic avoids that; the inner mux also gives us automatic
	// HEAD-from-GET handling and 405 + Allow responses for unsupported methods.
	dav := http.NewServeMux()
	dav.Handle("GET "+pattern, endpoint.HandleFunc(get))
	dav.Handle("PROPFIND "+pattern, endpoint.HandleFunc(webdavPropfindEndpoint(fsys, b.Dotfiles, b.Symlinks)))
	dav.Handle("OPTIONS "+pattern, endpoint.HandleFunc(webdavOptionsEndpoint()))
	return []RouteEntry{{Pattern: pattern, Endpoint: opaqueHandlerEndpoint(dav)}}, nil
}

// staticFileEndpoint builds the FileSystem GET endpoint: direct file mapping,
// optional index.html resolution, and optional directory listings rendered with
// FancyDirTemplate.
func staticFileEndpoint(fsys fs.FS, indexHTML, dirList bool) Endpoint {
	return (&endpoint.FileSystem{
		FS: func(_ context.Context, _ *http.Request) (fs.FS, error) {
			return fsys, nil
		},
		IndexHTML:        indexHTML,
		DirectoryListing: dirList,
		DirTemplate:      endpoint.FancyDirTemplate,
	}).Endpoint
}

func filesHandlerFactory() HandlerFactory {
	return func(node *yaml.Node) (HandlerBuilder, error) {
		b := new(filesBuilder)
		if node != nil {
			if err := node.Decode(b); err != nil {
				return nil, fmt.Errorf("files: decode config: %w", err)
			}
		}
		return b, nil
	}
}

// webdavOptionsEndpoint answers OPTIONS for a webdav-enabled files route,
// advertising WebDAV class-1 compliance so clients will mount the tree.
func webdavOptionsEndpoint() Endpoint {
	return func(http.ResponseWriter, *http.Request, Params) (endpoint.Renderer, error) {
		return endpoint.RendererFunc(func(w http.ResponseWriter, _ *http.Request) error {
			w.Header().Set("DAV", "1")
			w.Header().Set("Allow", "OPTIONS, GET, HEAD, PROPFIND")
			w.WriteHeader(http.StatusOK)
			return nil
		}), nil
	}
}

// webdavPropfindEndpoint answers a WebDAV PROPFIND with a 207 Multi-Status
// document describing the requested resource and, for a Depth: 1 request on a
// collection, its immediate children. Only the size, last-modified time, and
// resource type (file vs collection) are reported.
//
// dotfiles and symlinks mirror the filter applied to filteredFS.Open so that
// child listings do not expose resources that a GET could not fetch.
func webdavPropfindEndpoint(fsys fs.FS, dotfiles, symlinks bool) Endpoint {
	return func(w http.ResponseWriter, r *http.Request, params Params) (endpoint.Renderer, error) {
		// RFC 4918 defaults an absent Depth to infinity; we cap unspecified
		// requests at 1 and reject infinity rather than walking the whole tree.
		depth := r.Header.Get("Depth")
		if depth == "" {
			depth = "1"
		}
		if depth != "0" && depth != "1" {
			return nil, endpoint.Error(http.StatusForbidden, "PROPFIND Depth must be 0 or 1",
				fmt.Errorf("webdav: unsupported Depth %q", depth))
		}

		// Normalise into an fs.FS path the same way FileSystem.Endpoint does.
		p := path.Clean("/" + params.Path)
		if p == "/" {
			p = "."
		} else {
			p = strings.TrimPrefix(p, "/")
		}

		file, err := fsys.Open(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, endpoint.Error(http.StatusNotFound, "not found", err)
			}
			return nil, endpoint.Error(http.StatusInternalServerError, "internal server error", err)
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return nil, endpoint.Error(http.StatusInternalServerError, "internal server error", err)
		}

		// href values mirror the request path so clients can navigate from them.
		base := r.URL.EscapedPath()
		ms := davMultistatus{Responses: []davResponse{davResponseFor(base, info)}}

		if info.IsDir() && depth == "1" {
			if rdf, ok := file.(fs.ReadDirFile); ok {
				entries, err := rdf.ReadDir(-1)
				if err != nil {
					return nil, endpoint.Error(http.StatusInternalServerError, "failed to read directory", err)
				}
				dirHref := base
				if !strings.HasSuffix(dirHref, "/") {
					dirHref += "/"
				}
				for _, e := range entries {
					// Honour the same dotfile/symlink filter applied to Open.
					if !dotfiles && strings.HasPrefix(e.Name(), ".") {
						continue
					}
					if !symlinks && e.Type()&fs.ModeSymlink != 0 {
						continue
					}
					ci, err := e.Info()
					if err != nil {
						continue
					}
					href := dirHref + url.PathEscape(e.Name())
					if e.IsDir() {
						href += "/"
					}
					ms.Responses = append(ms.Responses, davResponseFor(href, ci))
				}
			}
		}

		return &multiStatusRenderer{ms: ms}, nil
	}
}

// davResponseFor builds the <response> element for a single resource, reporting
// resource type plus size/content-type (files only) and last-modified time.
func davResponseFor(href string, info fs.FileInfo) davResponse {
	prop := davProp{
		LastModified: info.ModTime().UTC().Format(http.TimeFormat),
	}
	if info.IsDir() {
		prop.ResourceType.Collection = &struct{}{}
	} else {
		size := info.Size()
		prop.ContentLength = &size
		prop.ContentType = mime.TypeByExtension(path.Ext(info.Name()))
	}
	return davResponse{
		Href:     href,
		Propstat: davPropstat{Prop: prop, Status: "HTTP/1.1 200 OK"},
	}
}

// davMultistatus models the subset of the RFC 4918 multistatus document needed
// to report file metadata. The "DAV:" namespace is declared once on the root
// element; child elements inherit it as the default namespace, so only the root
// tag carries the namespace in its struct tag.
type davMultistatus struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string      `xml:"href"`
	Propstat davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop   davProp `xml:"prop"`
	Status string  `xml:"status"`
}

type davProp struct {
	ResourceType  davResourceType `xml:"resourcetype"`
	ContentLength *int64          `xml:"getcontentlength,omitempty"`
	LastModified  string          `xml:"getlastmodified,omitempty"`
	ContentType   string          `xml:"getcontenttype,omitempty"`
}

// davResourceType is empty for files and <collection/> for directories.
type davResourceType struct {
	Collection *struct{} `xml:"collection"`
}

// multiStatusRenderer writes a 207 Multi-Status WebDAV response. The document is
// marshalled into a buffer first so a marshalling error surfaces before any
// status or body is committed.
type multiStatusRenderer struct {
	ms davMultistatus
}

func (mr *multiStatusRenderer) Render(w http.ResponseWriter, _ *http.Request) error {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(mr.ms); err != nil {
		return err
	}
	buf.WriteByte('\n')

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, err := w.Write(buf.Bytes())
	return err
}

// --- redirect ---

type redirectBuilder struct {
	Path string `yaml:"path"`
	To   string `yaml:"to"`
	Code int    `yaml:"code"`
	// PreservePath controls whether the sub-path is appended to To.
	// Only meaningful when Path ends with "/" (a tree route).
	// Default true: /old/foo/bar → /new/foo/bar.
	// Set false: all requests under /old/ redirect to the same fixed To target.
	// Has no effect on exact-match routes (no trailing slash, or ending with {$}).
	PreservePath *bool `yaml:"preserve_path"`
}

func (b *redirectBuilder) Validate(cfg Config) error {
	if b.To == "" {
		return fmt.Errorf("redirect (path=%q): to is required", b.Path)
	}
	return nil
}

func (b *redirectBuilder) Build(cfg Config, srv *Server, routePath string) ([]RouteEntry, error) {
	code := b.Code
	if code == 0 {
		code = http.StatusFound
	}
	preservePath := b.PreservePath == nil || *b.PreservePath
	toBase := strings.TrimSuffix(b.To, "/")
	ep := func(w http.ResponseWriter, r *http.Request, params Params) (endpoint.Renderer, error) {
		to := b.To
		if preservePath && params.Path != "" {
			to = toBase + "/" + params.Path
		}
		return &endpoint.RedirectRenderer{URL: to, Status: code}, nil
	}
	// If the route pattern ends with a slash, append {path...} so the sub-path
	// is available in params for sub-path preservation.
	p := routePathOnly(routePath)
	pattern := routePath
	if strings.HasSuffix(p, "/") {
		pattern = withSubPath(routePath)
	}
	return []RouteEntry{{Pattern: pattern, Endpoint: ep}}, nil
}

func redirectHandlerFactory() HandlerFactory {
	return func(node *yaml.Node) (HandlerBuilder, error) {
		b := new(redirectBuilder)
		if node != nil {
			if err := node.Decode(b); err != nil {
				return nil, fmt.Errorf("redirect: decode config: %w", err)
			}
		}
		return b, nil
	}
}

// --- proxy ---

type proxyBuilder struct {
	Path string `yaml:"path"`
	To   string `yaml:"to"`
}

func (b *proxyBuilder) Validate(cfg Config) error {
	if b.To == "" {
		return fmt.Errorf("proxy (path=%q): to is required", b.Path)
	}
	if u, err := url.Parse(b.To); err != nil || !u.IsAbs() {
		return fmt.Errorf("proxy (path=%q): to must be an absolute URL", b.Path)
	}
	return nil
}

func (b *proxyBuilder) Build(cfg Config, srv *Server, routePath string) ([]RouteEntry, error) {
	target, _ := url.Parse(b.To) // already validated

	ep := func(w http.ResponseWriter, r *http.Request, params Params) (endpoint.Renderer, error) {
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme = target.Scheme
				pr.Out.URL.Host = target.Host
				pr.Out.URL.Path = "/" + params.Path
				pr.Out.URL.RawPath = ""
				pr.Out.Host = target.Host
			},
		}
		return &endpoint.ProxyRenderer{Proxy: proxy}, nil
	}
	return []RouteEntry{{Pattern: withSubPath(routePath), Endpoint: ep}}, nil
}

func proxyHandlerFactory() HandlerFactory {
	return func(node *yaml.Node) (HandlerBuilder, error) {
		b := new(proxyBuilder)
		if node != nil {
			if err := node.Decode(b); err != nil {
				return nil, fmt.Errorf("proxy: decode config: %w", err)
			}
		}
		return b, nil
	}
}

// --- defaultmux ---

type defaultMuxBuilder struct{}

func (b *defaultMuxBuilder) Validate(cfg Config) error { return nil }
func (b *defaultMuxBuilder) Build(cfg Config, srv *Server, routePath string) ([]RouteEntry, error) {
	return []RouteEntry{{Pattern: routePath, Endpoint: opaqueHandlerEndpoint(http.DefaultServeMux)}}, nil
}

func defaultMuxHandlerFactory() HandlerFactory {
	return func(*yaml.Node) (HandlerBuilder, error) {
		return new(defaultMuxBuilder), nil
	}
}

// --- helpers ---

// opaqueHandlerEndpoint wraps an http.Handler as an Endpoint for handlers that
// own their full response lifecycle (e.g. srv.AuthHandler, http.DefaultServeMux).
// Do NOT use this for handlers that should participate in the endpoint renderer
// or processor contract — they must be written as endpoints from the start.
func opaqueHandlerEndpoint(h http.Handler) Endpoint {
	return func(w http.ResponseWriter, r *http.Request, _ Params) (endpoint.Renderer, error) {
		return endpoint.RendererFunc(func(w http.ResponseWriter, r *http.Request) error {
			h.ServeHTTP(w, r)
			return nil
		}), nil
	}
}

// withSubPath appends /{path...} to a mux pattern, handling optional method prefixes.
// "/notes/" → "/notes/{path...}", "GET /notes/" → "GET /notes/{path...}"
func withSubPath(pattern string) string {
	if method, path, ok := strings.Cut(pattern, " "); ok {
		return method + " " + strings.TrimSuffix(path, "/") + "/{path...}"
	}
	return strings.TrimSuffix(pattern, "/") + "/{path...}"
}

// filteredFS wraps an OS directory, optionally blocking dotfiles and symlinks.
type filteredFS struct {
	base     string
	dotfiles bool // if false, paths with dotfile components return ErrNotExist
	symlinks bool // if false, symlinks return ErrNotExist
}

func (f *filteredFS) Open(name string) (fs.File, error) {
	if !f.dotfiles {
		for part := range strings.SplitSeq(name, "/") {
			if part != "." && part != ".." && strings.HasPrefix(part, ".") {
				return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
			}
		}
	}
	if !f.symlinks {
		osPath := filepath.Join(f.base, filepath.FromSlash(name))
		info, err := os.Lstat(osPath)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	return os.DirFS(f.base).Open(name)
}

// routePathOnly returns the path part of a ServeMux pattern, stripping any
// leading method prefix (e.g. "GET /notes/" → "/notes/").
func routePathOnly(pattern string) string {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}
