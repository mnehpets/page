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

- `site`: `fsSite` gains `sync.RWMutex`; all query methods hold a read lock; `FileRenderer`/`DirRenderer` read page and layout under the same lock acquisition for consistency. No change to the `Site` interface contract.

## Impact

- `site.go`: add `sync.RWMutex`, `fsys`, `autoDiscoverLayout`, `fileMeta` to `fsSite`; locking in `FileRenderer`/`DirRenderer` via `lockedRenderer`; add `Refresh()`, `UpdateFile()`, `DeleteFile()`.
- `serve/handlers.go`: `pagesBuilder` stores `*fsSite`; adds `watch: true` option and fsnotify watcher startup.
- New dependency: `github.com/fsnotify/fsnotify`.
