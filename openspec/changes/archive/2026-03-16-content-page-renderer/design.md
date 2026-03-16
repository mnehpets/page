## Context

`github.com/mnehpets/page` is a greenfield Go module that provides a content rendering layer for `oneserve`'s `endpoint.FileSystem`. It supports Markdown with YAML front matter and HTML with JSON-LD metadata, renders pages through `html/template`, and integrates with `oneserve` via two new hook functions in `endpoint.FileSystem`.

## Goals / Non-Goals

**Goals:**
- Define a `Page` interface and two concrete implementations (Markdown, HTML)
- Parse YAML front matter from `.md` files; parse JSON-LD metadata from `.html` files
- Provide a `Site` interface with an FS-backed implementation, indexing pages by URL path and tag and supporting querying by URL path, tag, sort order, and pagination
- Provide a `Layout` type backed by `html/template`, discovered from `_layouts/` at `NewSite` time, applied inside `page.Render`
- Expose two hook functions (`FileRenderer`, `DirFallback`) that integrate cleanly with `endpoint.FileSystem` via functional options at construction time — omitting them preserves existing behaviour

**Non-Goals:**
- Full CMS, database, or dynamic content management
- Full-text search or indexing beyond tag and date sorting
- Tight coupling to any HTTP framework beyond the two hook functions
- Any change to existing `oneserve` API surface beyond adding two functional options to `endpoint.FileSystem`

## Decisions

### Page interface over concrete struct

**Decision:** Define `Page` as an interface (`URLPath`, `Meta`, `Renderer`) rather than a concrete struct. The primary responsibility of a `Page` is producing an `endpoint.Renderer` for its content; metadata and URL path are secondary, supporting the `Site` index and template queries.

**Rationale:** External content sources (CMS APIs, generated pages) need to integrate without re-implementing file parsing. The interface keeps the library open for extension without modifying core types.

**Alternative considered:** A single concrete struct with a type discriminant field. Rejected because it leaks parsing internals and makes it harder to add new content types.

---

### goldmark for Markdown

**Decision:** Use `github.com/yuin/goldmark` for Markdown rendering. No server-side syntax highlighting extension — goldmark emits standard `<code class="language-*">` blocks; syntax highlighting is the caller's responsibility via client-side JS (e.g. highlight.js, Prism.js).

**Rationale:** goldmark is CommonMark-compliant, extensible, and actively maintained. Deferring syntax highlighting to the client removes the `goldmark-highlighting/v2` (Chroma) dependency, eliminates Chroma version coupling, and gives callers full control over theme and library choice via their own templates.

**Alternative considered:** `russross/blackfriday`. Rejected — not CommonMark-compliant and in maintenance mode. Server-side Chroma highlighting also considered; rejected — adds a dependency and couples theme choices to the library rather than the caller's CSS.

---

### YAML front matter delimited by `---`

**Decision:** Markdown files use YAML front matter between `---` delimiters. HTML files use either optional `---` YAML front matter or a JSON-LD `<script type="application/ld+json">` block in `<head>`.

**Rationale:** `---` YAML is the Hugo/Jekyll convention; editors highlight it correctly. JSON-LD is already the standard structured-data format for HTML — it avoids a non-standard metadata convention for HTML files.

**Alternative considered:** Separate sidecar `.yaml` files. Rejected — splits metadata from content, complicating file moves and tooling.

---

### Site as an interface

**Decision:** `Site` is an interface exposing query methods (`Get(urlPath)`, `All()`, `ByTag()`, `ByCollection()`, etc.). The provided implementation is FS-backed (`NewSite(fsys fs.FS)`). The object exposed to `html/template` is the `Site` interface, not a concrete type.

There is no `Collection` type. "Collection" is a string attribute on a page's `Meta` used to filter queries — e.g. `site.ByCollection("blog")`.

**Rationale:** Different backing stores have different consistency models. The FS-backed implementation uses a two-phase approach: metadata (front matter) is parsed at construction time to build the index; body content is read from the FS at render time and is always fresh. A database-backed implementation could be fully consistent on every query. Making `Site` an interface means the consistency model is an implementation detail — callers, hook functions, and `RenderContext` are all agnostic to it. Alternative implementations (in-memory for tests, DB-backed, API-backed) slot in without touching any call sites.

**Dynamic content:** A custom `fs.FS` is the primary extension point for dynamic or externally-sourced content — it resolves paths at `Open` time and returns synthetic content. A fully dynamic backing store can alternatively be provided as a separate `Site` implementation.

**Future:** A database-backed or API-backed `Site` implementation is a natural next step. Because all call sites accept the interface, no API changes are needed to introduce one.

---

### Hook-based oneserve integration

**Decision:** Add two functional options (`WithFileRenderer(FileRendererHook)`, `WithDirRenderer(FileRendererHook)`) to `endpoint.FileSystem` in `oneserve`, passed at construction time. Omitting the options preserves existing behaviour.

**Rationale:** Keeps `oneserve` unaware of `page` — the dependency flows one way. Hook functions return `(Renderer, error)` matching the existing `endpoint.Renderer` interface; returning `nil, nil` signals fall-through. `FileRendererHook` has signature `(urlPath string, f fs.File) (Renderer, error)` — `endpoint.FileSystem` passes both the request URL path and the open file; the hook calls `site.Get(urlPath)` and, if the page is found, closes `f` immediately (the page holds its own FS reference and re-reads content at render time) then returns a `RendererFunc` that calls `page.Renderer(r, site, layout)`. URL paths for regular content files include the file extension (e.g. `/blog/hello-world.md`); `index.md` / `index.html` files are the exception, registered under the parent directory path (e.g. `/blog/`). Functional options keep the `endpoint.FileSystem` struct unexported-friendly and avoid nil-field checks scattered through the implementation.

**Alternative considered:** A middleware wrapper that intercepts `ServeHTTP`. Rejected — requires duplicating static file handling and breaks range requests.

---

### Layout system

**Decision:** A `Layout` type wraps a parsed `*html/template` set containing one or more named layout templates. The layout name is specified as a front matter field (for `.md` files) or a JSON-LD field (for `.html` files). `NewSite` discovers and parses layouts from a `_layouts/` subdirectory within the content `fs.FS` at construction time. A `WithLayout(*Layout)` functional option overrides this convention when callers want full control over layout file locations.

**Render flow:** The Site hook calls `page.Renderer(r, site, layout)`. Inside `page.Renderer`, the page renders its content to a buffer, then returns an `endpoint.HTMLTemplateRenderer` for the layout template named by `meta.Layout` (fallback: `"default"`) with a `RenderContext` struct: `{Content template.HTML, Head template.HTML, Meta Meta, Site Site, Request *http.Request}`. `Head` carries page-specific head elements: for `.html` files this is the `<head>` inner HTML excluding fields already captured by `Meta`; for `.md` files it is any head content produced by the Markdown renderer (empty when no extensions contribute head content). `RenderContext` is the template-facing type; it is constructed inside `page.Renderer`, not by the hook. This keeps the hook thin and each `Page` implementation self-contained. Using `endpoint.HTMLTemplateRenderer` means Content-Type, response buffering, and status code are all handled consistently by the endpoint framework.

**Rationale:** Applying the layout inside `page.Render` keeps the hook simple (it only resolves the page and calls Render) and avoids exposing template data construction as an API concern. `_layouts/` discovery at `NewSite` time is consistent with the two-phase approach (layout templates are parsed once; content is read at render time). The `WithLayout` override makes the convention escapable without changing the core API.

**No layout inheritance:** `html/template`'s `define`/`block` mechanism is sufficient for callers who need template composition. No additional inheritance machinery is provided.

**Alternative considered:** Passing a pre-built `RenderContext` from the hook into `page.Render`. Rejected — moves template data construction into the hook, coupling it to every Page implementation's internal needs.

## Risks / Trade-offs

- **goldmark version coupling** → Pin to a specific minor version; run tests on upgrades.
- **Metadata staleness in FS-backed Site** → Front matter is parsed at construction time; if a file's metadata changes, the index is stale until `NewSite` is called again. Body content is always fresh (re-read at render time). Mitigation: callers use a file watcher to trigger rebuilds; production deployments restart on content change.
- **Site rebuild cost on large sites** → Full walk + index rebuild on every file-watch event. Mitigation: acceptable for typical static sites (<10k pages); callers with larger corpora can debounce or use partial invalidation outside this library.
- **Companion change required** → The `oneserve` hook fields must ship before the `Site` hooks are usable. Mitigation: hook fields default to nil (no breakage), so they can be merged independently.
- **Dynamic content not in FS Site index** → Pages resolved dynamically by a custom `fs.FS` are not walked at construction time and are not queryable via `site.All()`, `site.ByTag()`, etc. Mitigation: accepted limitation for v1; a fully dynamic `Site` implementation is the longer-term answer.

## Migration Plan

This is a greenfield module — no migration from existing callers is required.

The companion change to `oneserve` adds two functional options (`WithFileRenderer`, `WithDirRenderer`) to `endpoint.FileSystem`. Because existing callers do not pass these options, the change is a drop-in non-breaking addition. No oneserve callers need updating.

Rollback: revert the two functional option additions to `endpoint.FileSystem` — all existing callers continue working unmodified.

## Open Questions

None.
