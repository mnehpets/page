## 1. Extend fsSite with mutex and refresh fields

- [x] 1.1 Add `mu sync.RWMutex`, `fsys fs.FS`, `autoDiscoverLayout bool`, and `fileMeta map[string]fileMeta` fields to `fsSite`; add `fileMeta` type (`sitePath string`, `modTime time.Time`)
- [x] 1.2 Update `NewSite` to populate `fileMeta` during `buildPageIndex` (stat each file via `d.Info().ModTime()`); store `fsys` on `fsSite`; set `autoDiscoverLayout = true` when no `WithLayout` option was provided
- [x] 1.3 Add `lockedRenderer` type that holds a `sync.Locker` (from `mu.RLocker()`) and an inner `endpoint.Renderer`; `Render(w, r)` calls `inner.Render` then releases the lock via `defer mu.Unlock()`
- [x] 1.4 Update `FileRenderer` and `DirRenderer`: acquire `mu.RLock()` at entry; read `pages[filePath]` and `layout` under that lock; on nil page or error release the lock immediately; otherwise wrap the renderer in `lockedRenderer` so the lock is held through template execution

## 2. Implement Refresh() — smart scan

- [x] 2.1 Walk the FS with `fs.WalkDir` (stat only via `d.Info()`); compare `ModTime` to `fileMeta`; collect changed, new, and deleted file paths — all without the lock
- [x] 2.2 Parse changed and new files via `newPageFromFS` — outside the lock; handle index file priority (`dirIndexes` logic) for directories with changed index files
- [x] 2.3 If any `_layouts/` file changed, re-run `discoverLayouts(s.fsys)` — outside the lock
- [x] 2.4 Acquire `mu.Lock()`; delete removed entries from `pages` and `fileMeta`; insert new/updated entries; update `layout` if re-discovered; rebuild `children` via `buildChildIndex`; release lock
- [x] 2.5 Return `(int, error)`: int = count of files re-parsed (new + changed); return error without mutating the index if any file parse fails

## 3. Implement UpdateFile() and DeleteFile()

- [x] 3.1 Add `UpdateFile(filePath string) error`: parse the file via `newPageFromFS` (outside lock); acquire `mu.Lock()`; update `pages`, `fileMeta`; if under `_layouts/`, re-run `discoverLayouts`; rebuild `children`; release lock
- [x] 3.2 Add `DeleteFile(filePath string)`: look up `sitePath` in `fileMeta` (under `mu.RLock()`); if the file was an index file, stat for a lower-priority fallback and parse it if found (outside write lock); acquire `mu.Lock()`; remove entry; insert fallback if found; rebuild `children`; release lock

## 4. fsnotify watcher in pagesBuilder

- [x] 4.1 Add `github.com/fsnotify/fsnotify` to `go.mod`
- [x] 4.2 Add `Watch bool` YAML field to `pagesBuilder`
- [x] 4.3 In `pagesBuilder.Build()`: if `Watch` is true, create a `fsnotify.Watcher` and add the pages directory
- [x] 4.4 In `pagesBuilder.Build()`: create a `metric.NewCounter("15m1m", "1h5m", "24h1h")` registered as `pageserve.route.<name>.refreshed_files` via `expvarMap`, mirroring the `requests` counter in `wrapWithStats`
- [x] 4.5 Start a goroutine that reads fsnotify events; maintain a `map[string]*time.Timer` for per-file debounce (~400ms); on timer fire, call `site.UpdateFile` for `Create`/`Write` events and `site.DeleteFile` for `Remove`/`Rename` events; increment the `refreshed_files` counter by the int returned from `Refresh()` or by 1 for `UpdateFile`/`DeleteFile`
- [x] 4.6 Shut down the watcher and cancel pending timers when the server context is cancelled

## 5. Tests

- [x] 5.1 Unit test `Refresh()`: build a site from an `fstest.MapFS`; mutate a file's content and `ModTime`; call `Refresh()`; assert updated metadata returned, other pages unchanged
- [x] 5.2 Unit test `Refresh()` — new file: add a file to the FS after construction; call `Refresh()`; assert it appears in `All()`
- [x] 5.3 Unit test `Refresh()` — deleted file: remove a file from the FS; call `Refresh()`; assert `Get` returns nil
- [x] 5.4 Unit test `UpdateFile()`: mutate one file; call `UpdateFile`; assert metadata updated
- [x] 5.5 Unit test `DeleteFile()`: call `DeleteFile` for a non-index file; assert absent; call for an index file with a lower-priority fallback; assert fallback promoted
- [x] 5.6 Concurrency test: run `Refresh()` and `All()` concurrently with `-race`; assert no data race and no panic
