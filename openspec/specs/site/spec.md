## ADDED Requirements

### Requirement: Site interface
The system SHALL define a `Site` interface with the following query methods: `Get(sitePath string) (Page, bool)`, `All() []Page`, `ByTag(tag string) []Page`, `ByCollection(name string) []Page`. The object exposed to `html/template` via `RenderContext.Site` MUST be the `Site` interface, not a concrete type.

#### Scenario: Get returns page by URL path
- **WHEN** `site.Get("/blog/hello-world.md")` is called and a page is registered at that path
- **THEN** it returns the page and `true`

#### Scenario: Get returns false for unknown path
- **WHEN** `site.Get("/not/found")` is called and no page is registered at that path
- **THEN** it returns `nil, false`

#### Scenario: All returns all non-draft pages
- **WHEN** `site.All()` is called
- **THEN** it returns all pages whose `Meta.Draft` is `false`

#### Scenario: ByTag filters by tag
- **WHEN** `site.ByTag("go")` is called
- **THEN** it returns all non-draft pages whose `Meta.Tags` contains `"go"`

#### Scenario: ByCollection filters by collection name
- **WHEN** `site.ByCollection("blog")` is called
- **THEN** it returns all non-draft pages whose `Meta.Collection` equals `"blog"`

---

### Requirement: NewSite construction
The system SHALL provide `NewSite(fsys fs.FS, opts ...SiteOption) (Site, error)` that walks the `fs.FS`, parses front matter and JSON-LD metadata from all `.md`, `.html`, and `.htm` files, and builds an index keyed by URL path. Unrecognised file types (e.g. `.css`, `.png`) MUST NOT be added to the index.

#### Scenario: Site built from FS
- **WHEN** `NewSite` is called with an FS containing `.md` and `.html` files
- **THEN** each file is parsed, a page is created, and the page is reachable via `site.Get(sitePath)`

#### Scenario: URL path derived from file path
- **WHEN** a file exists at `blog/hello-world.md` in the FS
- **THEN** its URL path is `/blog/hello-world.md`

#### Scenario: index file maps to directory path
- **WHEN** a file exists at `blog/index.html`, `blog/index.htm`, or `blog/README.md` in the FS
- **THEN** its URL path is `/blog/`

#### Scenario: index file conflict resolution
- **WHEN** a directory contains more than one index file (e.g. both `blog/index.html` and `blog/README.md`)
- **THEN** `index.html` takes priority over `index.htm`, which takes priority over `README.md`; only the highest-priority file is registered at the directory path

#### Scenario: Static files excluded from index
- **WHEN** the FS contains a `.png` or `.css` file
- **THEN** `site.All()` does not include it

#### Scenario: FS walk error returns error
- **WHEN** `NewSite` is called and the FS returns an error during walk
- **THEN** `NewSite` returns a non-nil error

---

### Requirement: Two-phase content loading
The system SHALL use a two-phase approach in the FS-backed implementation: metadata (front matter / JSON-LD) is parsed at index construction time to build the index; the page body is re-read from the FS at render time and is always current. The index SHALL be refreshable after construction via `Refresh()`, `UpdateFile()`, and `DeleteFile()` methods, which re-parse metadata for changed files only. A `FileRenderer` or `DirRenderer` hook invocation SHALL read the page and layout under a single read lock acquisition so they are consistent with each other.

#### Scenario: Metadata loaded at construction
- **WHEN** `NewSite` completes
- **THEN** `page.Meta()` returns the parsed metadata without re-reading the file

#### Scenario: Body content read at render time
- **WHEN** a file's body changes between `NewSite` and `page.Render`
- **THEN** `page.Render` reads the updated body from the FS

#### Scenario: Metadata refreshable after construction
- **WHEN** a file's frontmatter changes on disk and `Refresh()` or `UpdateFile()` is called
- **THEN** subsequent calls to `page.Meta()` for that page return the updated metadata

#### Scenario: Renderer uses consistent page and layout
- **WHEN** `FileRenderer` or `DirRenderer` is invoked for a request
- **THEN** the page lookup and layout used for rendering are read under the same lock acquisition

---

### Requirement: Narrowed exported interface; query methods on template type
`NewSite` SHALL return `SiteRenderer` (renamed from `Site`), which exposes only `FileRenderer()` and `DirRenderer()`. Query methods (`Get`, `All`, `ByTag`, `ByCollection`, `AncestorsOf`, `ChildrenOf`, `SiblingsOf`, `Config`) SHALL be methods on the unexported `site` type, which holds a pointer to `*fsSite`. `RenderContext.Site` SHALL be typed as `*site`. Template usage (`.Site.Get`, `.Site.All`, etc.) is unaffected.

#### Scenario: NewSite returns SiteRenderer
- **WHEN** `NewSite` is called
- **THEN** the return value satisfies `SiteRenderer` (providing `FileRenderer` and `DirRenderer`) and can be type-asserted to `Refreshable`

#### Scenario: Template query methods accessible via RenderContext.Site
- **WHEN** a template calls `.Site.Get`, `.Site.All`, `.Site.ByTag`, etc.
- **THEN** the call is dispatched to the `*site` value stored in `RenderContext.Site`

---

### Requirement: Draft filtering
The system SHALL exclude draft pages from `All()`, `ByTag()`, and `ByCollection()` results. Draft pages MUST still be retrievable via `site.Get(sitePath)`.

#### Scenario: Draft page excluded from All
- **WHEN** a page has `Meta.Draft == true`
- **THEN** `site.All()` does not include it

#### Scenario: Draft page retrievable by URL
- **WHEN** a page has `Meta.Draft == true`
- **THEN** `site.Get(sitePath)` returns it with `true`

---

### Requirement: Sorting
The system SHALL provide `SortByDate(pages []Page) []Page` that returns pages ordered by `Meta.Date` descending (newest first). Pages with zero `Date` values MUST sort after dated pages.

#### Scenario: Pages sorted by date descending
- **WHEN** `SortByDate` is called with pages having different dates
- **THEN** the returned slice is ordered newest-first

#### Scenario: Zero-date pages sort last
- **WHEN** `SortByDate` is called with a mix of dated and undated pages
- **THEN** undated pages appear after all dated pages

---

### Requirement: Pagination
The system SHALL provide a `Paginate(pages []Page, pageSize int, pageNum int) ([]Page, bool)` function that returns the requested page of results and a boolean indicating whether more pages follow. `pageNum` is 1-indexed.

#### Scenario: First page of results
- **WHEN** `Paginate(pages, 10, 1)` is called with 25 pages
- **THEN** it returns the first 10 pages and `true` (more pages follow)

#### Scenario: Last page of results
- **WHEN** `Paginate(pages, 10, 3)` is called with 25 pages
- **THEN** it returns the 5 remaining pages and `false`

#### Scenario: Page number beyond range returns empty
- **WHEN** `Paginate(pages, 10, 5)` is called with 25 pages
- **THEN** it returns an empty slice and `false`
