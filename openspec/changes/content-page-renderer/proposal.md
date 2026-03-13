## Why

Serving content-oriented pages requires a rendering layer that `oneserve`'s generic file server doesn't provide.
This library adds that layer: a content renderer that supports Markdown with YAML front matter and HTML source
files with JSON-LD metadata, applies site layouts via `html/template`, and integrates cleanly with the
existing `endpoint.FileSystem` hook points.

## What Changes

- New Go module `github.com/mnehpets/page` — a self-contained content rendering library.
- Pages are defined by a `Page` interface whose primary responsibility is rendering content to an `io.Writer`. `Meta` (front matter / JSON-LD) and `URLPath` are secondary, supporting indexing and template queries. The render signature is `Render(w, r, site, layout)` — layout application and template data construction happen inside each `Page` implementation.
- Two concrete page types: `markdownPage` and `htmlPage`. Non-content files (assets, etc.) are not represented as pages — the hook returns `nil, nil` for unrecognised file types, falling through to `endpoint.FileSystem`'s default handler.
- Front matter in Markdown files is YAML delimited by `---`; metadata in HTML files is JSON-LD in a `<script type="application/ld+json">` tag in the `<head>`.
- A `Site` interface is backed by an FS implementation (`NewSite(fsys fs.FS)`) and indexes all pages by URL path and tag. It is the object exposed to `html/template` for querying related pages, lists, etc. There is no separate `Collection` type — "collection" is a string attribute on `Meta` used to filter `Site` queries (e.g. `site.ByCollection("blog")`).
- A `Layout` type wraps a parsed `*html/template` set. `NewSite` discovers and parses layout templates from a `_layouts/` subdirectory within the content FS at construction time; `WithLayout(*Layout)` overrides this. The layout applied to each page is named by the page's `layout` front matter field (or JSON-LD equivalent). Templates receive `RenderData{Content template.HTML, Meta Meta, Site Site, Request *http.Request}`.
- Integration with `endpoint.FileSystem` is via two functional options (`WithFileRenderer`, `WithDirFallback`) added to `github.com/mnehpets/oneserve` — this is a companion change to that repo. **Omitting the options preserves all existing behaviour; no callers need updating.**

## Capabilities

### New Capabilities

- `page`: The `Page` interface, `Meta`, `PageFromFile` auto-detection, and the two concrete page types (`markdownPage`, `htmlPage`) including front matter and JSON-LD parsing.
- `site`: `Site` interface and FS-backed implementation — URL-path index, tag index, collection filtering, draft filtering, sorting, and `Paginate` helper.
- `layout`: `Layout` type — parses named layout templates from `fs.FS` or explicit files; applied inside `page.Render` via `RenderData`.
- `hooks`: `FileRenderer` and `DirFallback` — methods on `Site` returning hook functions for `endpoint.FileSystem` integration.

### Modified Capabilities

## Impact

- New dependency on `github.com/yuin/goldmark`, `gopkg.in/yaml.v3`.
- Soft dependency on `github.com/mnehpets/oneserve` (only the hook functions import endpoint types; the rest of the library has no such dependency).
- The functional option additions to `endpoint.FileSystem` in `oneserve` are additive and non-breaking — omitting them preserves all existing behaviour.
- No changes to any existing `page` or `oneserve` API; this is a greenfield module.
