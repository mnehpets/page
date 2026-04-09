## Context

`fsSite` (site.go) builds its `pages` and `children` maps once in `NewSite()`. The walk and per-file frontmatter parse is the expensive part — O(n) file reads. Body content is already re-read from disk on every request; only metadata and the page index are frozen.

`FileRenderer()` and `DirRenderer()` return closures that capture `s *fsSite` at build time. Those closures are wired into the mux permanently. Any refresh mechanism must ensure these closures see updated data on each request without the mux being rewired. Because the closures already capture the `*fsSite` pointer, updating the struct's fields in-place is sufficient — no wrapper or indirection layer is needed.

## Goals / Non-Goals

**Goals:**
- Re-index only files that have changed (by `ModTime`) when the watcher fires.
- Re-index a single file immediately when an fsnotify event arrives.
- Guarantee that concurrent reads never observe a partially-updated index.
- Keep the `Site` interface and `NewSite` signature unchanged.

**Non-Goals:**
- SIGHUP handling or HTTP webhook endpoints — fsnotify covers both local edits and git-pull syncs by watching the filesystem directly.
- Sub-millisecond refresh latency — a ~400ms fsnotify debounce is acceptable.
- Watching remote (non-`os.DirFS`) filesystems.
- Distributing refresh signals across multiple processes.

## Decisions

### 1. `sync.RWMutex` held for the duration of each render

Add `mu sync.RWMutex` to `fsSite`. The read lock is acquired once per request at the top of the `FileRenderer`/`DirRenderer` hook and released only after `Render(w, r)` returns — via a `lockedRenderer` wrapper. Individual query methods (`Get`, `All`, `ByTag`, etc.) carry no locks: they are only ever called during template execution, which is reachable exclusively through `FileRenderer` and `DirRenderer`, so the read lock is always already held.

```go
// lockedRenderer releases the read lock after the inner Render() returns.
type lockedRenderer struct {
    mu    sync.Locker  // s.mu.RLocker()
    inner endpoint.Renderer
}
func (lr *lockedRenderer) Render(w http.ResponseWriter, r *http.Request) error {
    defer lr.mu.Unlock()
    return lr.inner.Render(w, r)
}
```

`FileRenderer` acquires `mu.RLock()` at entry, reads `pages[filePath]` and `layout` (consistent, same lock window), and returns a `lockedRenderer` that owns the lock. If the page is not found or rendering setup fails, the lock is released immediately.

Multiple concurrent requests each hold the read lock; they do not block each other. A refresh (`mu.Lock()`) must wait until all in-flight renders complete — a natural drain with no extra coordination.

**Alternative considered:** clone both maps and use `atomic.Pointer[siteSnapshot]` for a lock-free read path. Rejected because cloning is more expensive than a brief write lock on typical (< 10k page) sites, and the added struct complexity (snapshot type, atomic pointer) is not justified when the write lock is held only for in-memory operations.

**Alternative considered:** per-method `RLock`/`RUnlock` on `Get`, `All`, etc. Rejected because it acquires and releases the lock multiple times per render (once per template `{{ .Site.X }}` call) with no benefit, since all those calls already occur within the single `FileRenderer`-level lock window.

### 2. File I/O outside the write lock

Parse changed files before acquiring the write lock. The write lock is held only for the map mutations:

```
1. Walk FS (stat only, no lock)  →  find changed / new / deleted files
2. Parse changed + new files (no lock)  →  build map[filePath]Page
3. mu.Lock()
4.   Apply deletions to pages + fileMeta
5.   Insert new/updated entries
6.   Rebuild children (buildChildIndex, in-memory)
7. mu.Unlock()
```

This keeps the write lock window minimal regardless of how many files changed.

### 3. `fileMeta` map as the change fingerprint

Add `fileMeta map[string]fileMeta` (fsFilePath → `{sitePath, modTime}`) to `fsSite`. On `Refresh()`, walk the FS with stat-only calls, compare `ModTime` to `fileMeta`, and re-parse only differing files. Deleted files are keys in `fileMeta` absent from the walk.

Cost: O(dir-walk stat calls) + O(k · parse) where k = changed files. For a 1000-page site with one changed file, only one file is opened.

### 4. fsnotify as the sole refresh trigger

A `watch: true` option on `pagesBuilder` starts an fsnotify watcher on the pages directory. fsnotify fires for any on-disk change — user edits, `git pull`, rsync, or any other sync tool. This covers all intended use cases without separate SIGHUP or HTTP webhook infrastructure.

Events are debounced per-file with a ~400ms `time.AfterFunc`. When the timer fires:
- `Create` / `Write` → `UpdateFile(filePath)`: parse one file, update index under lock.
- `Remove` / `Rename` → `DeleteFile(filePath)`: remove from index under lock; if it was an index file, stat for a lower-priority fallback.

`UpdateFile` and `DeleteFile` skip the directory walk entirely, making them O(1 parse) + O(n children rebuild).

### 5. `fsSite` stores `fsys` and `autoDiscoverLayout` for refresh use

`NewSite` currently doesn't store the `fs.FS` it receives. The refresh methods need it to re-parse files. Add `fsys fs.FS` as an immutable field set at construction.

The other `siteConfig` fields (`includeDrafts`, `config`) are already stored directly on `fsSite` as `drafts` and `config`. The only additional thing to track is whether the layout was auto-discovered from `_layouts/` (and should be re-discovered on refresh) or was supplied by the caller via `WithLayout` (and should be left alone). A single `autoDiscoverLayout bool` field captures this — no need to store the full `siteConfig`.

### 6. `FileRenderer`/`DirRenderer` read page and layout under one lock acquisition

To ensure the page and its layout template are consistent, both are read inside a single `RLock` window:

```go
s.mu.RLock()
pg := s.pages[filePath]
layout := s.layout
s.mu.RUnlock()
```

A refresh that updates both `pages` and `layout` under the write lock guarantees either both are old or both are new from any reader's perspective.

## Risks / Trade-offs

**Write lock blocks all readers for children rebuild** → Accepted. `buildChildIndex` iterates `pages` in-memory with no I/O; on a 10k-page site this is in the low milliseconds at worst.

**fsnotify debounce means ~400ms stale window** → Acceptable for local development. In production, a `git pull` writes files atomically per-file, so the watcher picks up each changed file; the debounce just batches rapid saves from an editor.

**Index file priority edge case on `DeleteFile`** → Handled: when removing an index file, stat the directory for lower-priority index files and promote if found. Two stat calls in the worst case.

**Concurrent `UpdateFile` calls for the same path** → Serialised by `mu.Lock()`. Last writer wins, which is correct — both represent the current on-disk state.

## Migration Plan

`NewSite`, `Site` interface, and all query methods are API-compatible. Existing configs without `watch: true` behave identically to the current version. No data format changes, no rollback concerns beyond a binary swap.

### 6. `Refresh()` returns a file count; `pagesBuilder` owns the metric counter

`Refresh()` returns `(int, error)` where int is the number of files re-parsed (new + changed). The `page` package has no dependency on `github.com/zserge/metric` or `expvar` — it is not responsible for instrumentation.

`pagesBuilder` creates a per-route `metric.NewCounter("15m1m", "1h5m", "24h1h")` registered under `pageserve.route.<name>.refreshed_files`, mirroring the `requests` counter in `wrapWithStats`. The fsnotify watcher increments it by the count returned from `Refresh()`, and by 1 for each `UpdateFile()` or `DeleteFile()` call.
