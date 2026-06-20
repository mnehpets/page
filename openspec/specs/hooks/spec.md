## ADDED Requirements

### Requirement: FileRenderer hook
The system SHALL expose a `FileRenderer() endpoint.FileRendererHook` method on `Site`. `endpoint.FileRendererHook` has signature `(sitePath string, f fs.File) (endpoint.Renderer, error)`. The returned func is a closure over the `Site` and its `Layout`. `endpoint.FileSystem` calls the hook after path normalisation, passing the request URL path and the open `fs.File`; the hook calls `site.Get(sitePath)` and, if the page is found, closes `f` immediately (the page re-reads content from its own FS reference at render time) and returns a `RendererFunc` that calls `page.Renderer(r, site, layout)`.

**File ownership:** if a page is found, ownership of the `fs.File` transfers to the hook, which closes it immediately. If no page is found, ownership remains with `endpoint.FileSystem` and the hook MUST NOT call `Read` on the file (`Stat` is safe).

#### Scenario: Page found and renderer returned
- **WHEN** the `FileRendererHook` is called with sitePath `/blog/hello-world.md` and the corresponding open file
- **THEN** it finds the page at `/blog/hello-world.md` and returns a non-nil `endpoint.Renderer`

#### Scenario: Page not found returns nil
- **WHEN** the `FileRendererHook` is called with a URL path that is not in the site index (e.g. `/style.css`)
- **THEN** it returns `nil, nil` without closing or reading the file

#### Scenario: Renderer calls page.Renderer
- **WHEN** the returned `endpoint.Renderer` is executed by `endpoint.FileSystem`
- **THEN** it calls `page.Renderer(r, site, layout)` and delegates to the returned `endpoint.HTMLTemplateRenderer`

#### Scenario: Render error propagated
- **WHEN** `page.Render` returns a non-nil error
- **THEN** the `endpoint.Renderer` propagates the error

---

### Requirement: DirRenderer hook
The system SHALL expose a `DirRenderer() endpoint.FileRendererHook` method on `Site`. The hook signature is the same type as `FileRendererHook`: `(sitePath string, f fs.File) (endpoint.Renderer, error)`. The returned func is a closure over the `Site` and its `Layout`. When called by `endpoint.FileSystem` for a directory request, it receives the URL path string for the directory (e.g. `/blog/`) and the open directory `fs.File`, then calls `site.Get(sitePath)`. This works because `index.html`, `index.htm`, and `README.md` files are registered in the site index under their parent directory path rather than their file path (e.g. `blog/README.md` is indexed at `/blog/`). When multiple index files exist, priority is resolved at `NewSite` time (`index.html` > `index.htm` > `README.md`) so the hook always calls a single `site.Get` with no ambiguity. If a page is found, it returns a non-nil `endpoint.Renderer`; otherwise it returns `nil, nil`.

The hook is called after path normalisation but **before** any index-file lookup or directory listing, giving it priority over `IndexHTML`. The hook MUST NOT call `ReadDir` on the file if it returns `nil, nil` (`Stat` is safe). File ownership on a non-nil return transfers to the hook; on `nil, nil` ownership remains with `endpoint.FileSystem`.

#### Scenario: Directory URL resolves to README.md
- **WHEN** the `DirRendererHook` is called with sitePath `/blog/` and `blog/README.md` is registered at `/blog/`
- **THEN** it returns a non-nil `endpoint.Renderer` for that page

#### Scenario: Directory URL resolves to index.html
- **WHEN** the `DirRendererHook` is called with sitePath `/blog/` and `blog/index.html` is registered at `/blog/`
- **THEN** it returns a non-nil `endpoint.Renderer` for that page

#### Scenario: No index page returns nil
- **WHEN** the `DirRendererHook` is called with sitePath `/blog/` and no page is registered at `/blog/`
- **THEN** it returns `nil, nil`

---

### Requirement: Hook fall-through preserves existing behaviour
The system SHALL ensure that returning `nil, nil` from either hook causes `endpoint.FileSystem` to fall through to its default handling (e.g., `http.ServeContent` for static files, `IndexHTML` lookup, directory listing, or 404). This MUST be the behaviour when the hooks are omitted entirely via `WithFileRenderer` / `WithDirRenderer`.

#### Scenario: Nil return causes fall-through
- **WHEN** a hook returns `nil, nil`
- **THEN** `endpoint.FileSystem` handles the request using its default logic

#### Scenario: Omitted hooks preserve default behaviour
- **WHEN** `endpoint.FileSystem` is constructed without `WithFileRenderer` or `WithDirRenderer`
- **THEN** all requests are handled exactly as they were before the options were added

---

### Requirement: oneserve functional options
The system SHALL add `WithFileRenderer(FileRendererHook)` and `WithDirRenderer(FileRendererHook)` as functional options to `endpoint.FileSystem` in `github.com/mnehpets/oneserve`. Both options accept the same `FileRendererHook` type. These options MUST be the only change to the `oneserve` API.

#### Scenario: FileRenderer option accepted
- **WHEN** `endpoint.FileSystem` is constructed with `WithFileRenderer(site.FileRenderer())`
- **THEN** the file renderer hook is active for all file requests

#### Scenario: DirRenderer option accepted
- **WHEN** `endpoint.FileSystem` is constructed with `WithDirRenderer(site.DirRenderer())`
- **THEN** the directory renderer hook is active for all directory requests, taking priority over index-file lookup

#### Scenario: Existing callers unaffected
- **WHEN** existing code constructs `endpoint.FileSystem` without the new options
- **THEN** behaviour is identical to before the options were added
