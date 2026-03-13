## ADDED Requirements

### Requirement: FileRenderer hook
The system SHALL expose a `FileRenderer() FileRendererFunc` method on `Site`. `FileRendererFunc` has signature `(urlPath string, f fs.File) (endpoint.Renderer, error)`. The returned func is a closure over the `Site` and its `Layout`. `endpoint.FileSystem` passes the request URL path and the open `fs.File` directly; the hook calls `site.Get(urlPath)` and, if the page is found, returns a Renderer that calls `page.Render(w, r, site, layout)`.

**File ownership:** if a page is found, ownership of the `fs.File` transfers to the hook — `endpoint.FileSystem` MUST NOT close it. If no page is found, ownership remains with `endpoint.FileSystem` and the hook MUST NOT close the file.

#### Scenario: Page found and renderer returned
- **WHEN** `FileRendererFunc` is called with urlPath `/blog/hello-world.md` and the corresponding open file
- **THEN** it finds the page at `/blog/hello-world.md` and returns a non-nil `endpoint.Renderer`

#### Scenario: Page not found returns nil
- **WHEN** `FileRendererFunc` is called with a URL path that is not in the site index (e.g. `/style.css`)
- **THEN** it returns `nil, nil` without closing the file

#### Scenario: Renderer calls page.Render
- **WHEN** the returned `endpoint.Renderer` is executed by `endpoint.FileSystem`
- **THEN** it calls `page.Render(w, r, site, layout)` and writes the response

#### Scenario: Render error propagated
- **WHEN** `page.Render` returns a non-nil error
- **THEN** the `endpoint.Renderer` propagates the error

---

### Requirement: DirFallback hook
The system SHALL expose a `DirFallback() DirFallbackFunc` method on `Site`. `DirFallbackFunc` has signature `(urlPath string) (endpoint.Renderer, error)`. The returned func is a closure over the `Site` and its `Layout`. When called by `endpoint.FileSystem` for a directory request, it receives the URL path string for the directory (e.g. `/blog`) and calls `site.Get(urlPath)`. This works because `index.md`, `index.html`, and `index.htm` files are registered in the site index under their parent directory path rather than their file path (e.g. `blog/index.md` is indexed at `/blog`). If a page is found, it returns a non-nil `endpoint.Renderer`; otherwise it returns `nil, nil`. No file ownership concerns apply — no `fs.File` is passed.

#### Scenario: Directory URL resolves to index.md
- **WHEN** `DirFallbackFunc` is called with urlPath `/blog` and `blog/index.md` is registered at `/blog`
- **THEN** it returns a non-nil `endpoint.Renderer` for that page

#### Scenario: Directory URL resolves to index.html
- **WHEN** `DirFallbackFunc` is called with urlPath `/blog` and `blog/index.html` is registered at `/blog`
- **THEN** it returns a non-nil `endpoint.Renderer` for that page

#### Scenario: No index page returns nil
- **WHEN** `DirFallbackFunc` is called with urlPath `/blog` and no page is registered at `/blog`
- **THEN** it returns `nil, nil`

---

### Requirement: Hook fall-through preserves existing behaviour
The system SHALL ensure that returning `nil, nil` from either hook function causes `endpoint.FileSystem` to fall through to its default handling (e.g., `http.ServeContent` for static files, default directory listing or 404). This MUST be the behaviour when the hooks are omitted entirely via `WithFileRenderer` / `WithDirFallback`.

#### Scenario: Nil return causes fall-through
- **WHEN** a hook returns `nil, nil`
- **THEN** `endpoint.FileSystem` handles the request using its default logic

#### Scenario: Omitted hooks preserve default behaviour
- **WHEN** `endpoint.FileSystem` is constructed without `WithFileRenderer` or `WithDirFallback`
- **THEN** all requests are handled exactly as they were before the options were added

---

### Requirement: oneserve functional options
The system SHALL add `WithFileRenderer(FileRendererFunc)` and `WithDirFallback(DirFallbackFunc)` as functional options to `endpoint.FileSystem` in `github.com/mnehpets/oneserve`. These options MUST be the only change to the `oneserve` API.

#### Scenario: FileRenderer option accepted
- **WHEN** `endpoint.FileSystem` is constructed with `WithFileRenderer(site.FileRenderer())`
- **THEN** the file renderer hook is active for all file requests

#### Scenario: DirFallback option accepted
- **WHEN** `endpoint.FileSystem` is constructed with `WithDirFallback(site.DirFallback())`
- **THEN** the directory fallback hook is active for all directory requests

#### Scenario: Existing callers unaffected
- **WHEN** existing code constructs `endpoint.FileSystem` without the new options
- **THEN** behaviour is identical to before the options were added
