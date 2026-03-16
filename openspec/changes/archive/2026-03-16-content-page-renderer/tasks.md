## Tasks

### 1. oneserve companion change

- [x] 1.1. Add `FileRendererHook` type `func(urlPath string, f fs.File) (Renderer, error)` to `endpoint` package
- [x] 1.2. Add `WithFileRenderer(FileRendererHook)` functional option to `endpoint.FileSystem`
- [x] 1.3. Add `WithDirRenderer(FileRendererHook)` functional option to `endpoint.FileSystem`
- [x] 1.4. Call `FileRendererHook` (when set) after path normalisation, before default file serving; fall through on `nil, nil`
- [x] 1.5. Call `DirRendererHook` (when set) after path normalisation, before `IndexHTML` lookup or directory listing; fall through on `nil, nil`
- [x] 1.6. Write tests: hook active, hook returns nil (fall-through), hooks omitted (existing behaviour unchanged)

---

### 2. `page` package — core types

- [x] 2.1. Define `Page` interface: `URLPath() string`, `Meta() Meta`, `Renderer(r *http.Request, site Site, layout *Layout) (endpoint.Renderer, error)`
- [x] 2.2. Define `Meta` struct: `Title`, `Author`, `Date`, `Tags`, `Collection`, `Layout`, `Draft`, `Description`, `Image`, `Slug`
- [x] 2.3. Define `Image` struct: `URL string`, `Alt string`
- [x] 2.4. Define `RenderContext` struct: `Content template.HTML`, `Head template.HTML`, `Meta Meta`, `Site Site`, `Request *http.Request`

---

### 3. `page` package — front matter parsing

- [x] 3.1. Implement `ParseFrontMatter(r io.Reader) (Meta, []byte, error)`: strips `---`-delimited YAML front matter, returns parsed `Meta` and remaining body bytes
- [x] 3.2. Handle `image` field as bare string (URL only, Alt `""`) or nested mapping `{url, alt}`
- [x] 3.3. Write tests: with front matter, without front matter, malformed YAML

---

### 4. `page` package — `markdownPage`

- [x] 4.1. Implement `markdownPage` (unexported): stores `urlPath`, `Meta`, and a reference to the FS for body re-read at render time
- [x] 4.2. `Renderer`: re-read body from FS, run through goldmark (no server-side highlighting), populate `RenderContext.Content`; populate `RenderContext.Head` from any head content goldmark extensions produce (empty for basic rendering)
- [x] 4.3. Return `endpoint.HTMLTemplateRenderer` for layout template named by `Meta.Layout` (fallback: `"default"`) with `RenderContext`
- [x] 4.4. Write tests: front matter parsed, body rendered via goldmark, fenced code block emits `language-*` class, render writes layout output

---

### 5. `page` package — `htmlPage`

- [x] 5.1. Implement `htmlPage` (unexported): stores `urlPath`, `Meta`, and FS reference
- [x] 5.2. Parse JSON-LD from `<script type="application/ld+json">` in `<head>`; map recognised properties to `Meta` fields (including `layout`)
- [x] 5.3. Apply three-tier fallback per field: JSON-LD → HTML `<meta>` tags / `<title>` → `fs.FileInfo.ModTime` (for `Date`)
- [x] 5.4. `Renderer`: re-read body from FS; extract `<body>` inner HTML → `RenderContext.Content`; extract remaining `<head>` inner HTML (excluding Meta-captured fields) → `RenderContext.Head`
- [x] 5.5. Return `endpoint.HTMLTemplateRenderer` for layout template with `RenderContext`
- [x] 5.6. Write tests: JSON-LD parsed, HTML meta fallback, title fallback, FS date fallback, Content/Head populated correctly, invalid JSON returns error

---

### 7. `page` package — `Layout`

- [x] 7.1. Implement `Layout` type wrapping `*html/template`
- [x] 7.2. Implement `NewLayout(fsys fs.FS, patterns ...string) (*Layout, error)`: parse all matching files; template names from `{{define}}` declarations
- [x] 7.3. `Template() *html/template.Template` accessor; template execution delegated to `endpoint.HTMLTemplateRenderer`
- [x] 7.4. Write tests: templates parsed, named template executed, unknown name returns error, empty pattern returns error

---

### 8. `page` package — `Site`

- [x] 8.1. Define `Site` interface: `Get`, `All`, `ByTag`, `ByCollection`, `FileRenderer`, `DirRenderer`
- [x] 8.2. Implement `NewSite(fsys fs.FS, opts ...SiteOption) (Site, error)`: walk FS, call internal constructors for each recognised file type, build URL-path and tag indexes
- [x] 8.3. URL path derivation: regular files keep extension (e.g. `blog/hello-world.md` → `/blog/hello-world.md`); index files (`index.html` > `index.htm` > `index.md`) register under parent directory path with trailing slash (e.g. `/blog/`)
- [x] 8.4. Discover and parse `_layouts/` at construction time; populate `Layout` (nil if absent)
- [x] 8.5. Implement `WithLayout(*Layout)` option to override `_layouts/` discovery
- [x] 8.6. Implement `WithIncludeDrafts()` option
- [x] 8.7. Implement `Slug` derivation: explicit `slug` in front matter / JSON-LD; otherwise last segment of URL path with content extension stripped
- [x] 8.8. Implement `SortByDate(pages []Page) []Page`: descending, zero-date pages last
- [x] 8.9. Implement `Paginate(pages []Page, pageSize, pageNum int) ([]Page, bool)`: 1-indexed
- [x] 8.10. Write tests: index built, URL paths correct, index file priority, draft filtering, ByTag, ByCollection, slug derivation (regular file, index file, explicit slug), SortByDate, Paginate

---

### 9. `page` package — hooks

- [x] 9.1. Implement `Site.FileRenderer() endpoint.FileRendererHook`: closure over site and layout; calls `site.Get(urlPath)`, closes `f` immediately on hit (page re-reads from its own FS reference), returns `nil, nil` on miss
- [x] 9.2. Implement `Site.DirRenderer() endpoint.FileRendererHook`: same pattern as FileRenderer; `nil, nil` on miss does not call ReadDir
- [x] 9.3. Write tests: page found returns renderer, page not found returns nil, renderer calls page.Renderer, render error propagated

---

### 10. Integration test

- [x] 10.1. End-to-end test: `NewSite` over testdata FS with `_layouts/`; `endpoint.FileSystem` constructed with `WithFileRenderer(site.FileRenderer())` and `WithDirRenderer(site.DirRenderer())`; assert `.md` request returns rendered HTML, `.html` request returns rendered HTML, static asset request returns file bytes, directory request returns index page, unknown path returns 404
