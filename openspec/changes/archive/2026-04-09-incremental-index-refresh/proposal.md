## Why

`fsSite` builds its page index once at startup by walking the entire filesystem and parsing frontmatter from every file. If files change on disk — whether a user edits content locally or a deploy syncs files from GitHub — the index is stale until the process restarts.

## What Changes

- `fsSite` gains a `sync.RWMutex` for safe concurrent access; refresh methods update the maps in-place under the write lock (I/O done before acquiring the lock).
- `fsSite` gains incremental refresh methods: `Refresh()` (smart scan, re-parses only changed files), `UpdateFile()` (single file), and `DeleteFile()` (single file removal).
- `fsSite` stores `fsys` and an `autoDiscoverLayout bool` so it can re-parse files and optionally re-discover layout templates on refresh.
- `fileMeta` map (fsFilePath → sitePath + modTime) is added as the change-detection fingerprint.
- `pagesBuilder` gains a `watch: true` config option that starts an fsnotify watcher, debounces events (~400ms), and calls `UpdateFile`/`DeleteFile` per event.

## Capabilities

### New Capabilities

- `site-refresh`: Incremental live refresh of the page index via fsnotify.

### Modified Capabilities

- `site`: `fsSite` gains `sync.RWMutex`. The exported `Site` interface is renamed `SiteRenderer` and narrowed to rendering hooks only (`FileRenderer()`, `DirRenderer()`). Query methods (`Get`, `All`, `ByTag`, etc.) are removed from the exported interface and moved to an unexported `site` type (which holds a pointer to `*fsSite` and is set as `RenderContext.Site`). The read lock is acquired once per request inside `FileRenderer`/`DirRenderer` and held through `Render()` via `lockedRenderer`; `FileRenderer` and `DirRenderer` share a single `lockedRendererHook` implementation.

## Impact

- `site.go`: add `sync.RWMutex`, `fsys`, `autoDiscoverLayout`, `fileMeta` to `fsSite`; rename `Site` → `SiteRenderer` and narrow to renderer hooks; add unexported `site` type for template query API; shared `lockedRendererHook`; add `Refresh()`, `UpdateFile()`, `DeleteFile()`.
- `page.go`: `RenderContext.Site` typed as `*site`; `Page.Renderer` parameter typed as `*site`.
- `serve/handlers.go`: `pagesBuilder` type-asserts `page.Refreshable`; adds `watch: true` option; fsnotify watcher recursively watches all subdirectories via `addWatchDirs` and handles new directory creation events at runtime.
- New dependency: `github.com/fsnotify/fsnotify`.
