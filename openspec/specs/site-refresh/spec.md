## ADDED Requirements

### Requirement: Smart scan refresh
The system SHALL provide a `Refresh() (int, error)` method on `fsSite` that walks the filesystem using stat-only calls, re-parses only files whose `ModTime` has changed since the last index build, adds newly created files, removes deleted files, and updates the index under a write lock. The returned int SHALL be the number of files re-parsed (new + changed). The `Site` interface MUST NOT expose `Refresh()`.

#### Scenario: Changed file is re-parsed
- **WHEN** a file's content and `ModTime` have changed since `NewSite` was called and `Refresh()` is invoked
- **THEN** the updated metadata is reflected in subsequent calls to `site.Get()`, `site.All()`, etc., and the returned count is at least 1

#### Scenario: Return count reflects files re-parsed
- **WHEN** `Refresh()` is called and k files have changed `ModTime`
- **THEN** the returned int equals k

#### Scenario: Unchanged files are not re-parsed
- **WHEN** `Refresh()` is called and a file's `ModTime` is identical to the stored fingerprint
- **THEN** the file is not opened or read during the refresh

#### Scenario: New file is added to the index
- **WHEN** a new `.md`, `.html`, or `.htm` file is created on disk and `Refresh()` is invoked
- **THEN** `site.Get(sitePath)` returns the new page and it appears in `site.All()`

#### Scenario: Deleted file is removed from the index
- **WHEN** a file is deleted from disk and `Refresh()` is invoked
- **THEN** `site.Get(sitePath)` returns `nil` and the page no longer appears in `site.All()`

#### Scenario: Index file priority re-evaluated on change
- **WHEN** a directory's index files change (e.g., `index.html` is added alongside `index.md`) and `Refresh()` is invoked
- **THEN** the higher-priority file (`index.html`) becomes the directory index

#### Scenario: Layout templates refreshed if changed
- **WHEN** a file under `_layouts/` has a changed `ModTime` and `Refresh()` is invoked
- **THEN** the updated layout templates are used for subsequent render requests

#### Scenario: Refresh error preserves old index
- **WHEN** `Refresh()` encounters a parse error on a changed file
- **THEN** `Refresh()` returns a non-nil error and the existing index continues to serve requests unchanged

---

### Requirement: Per-file update
The system SHALL provide an `UpdateFile(filePath string) error` method on `fsSite` that re-parses a single file and updates its entry in the index under a write lock. This method is intended for use by the fsnotify event handler.

#### Scenario: Single file metadata update
- **WHEN** `UpdateFile("blog/hello.md")` is called after that file's frontmatter has changed
- **THEN** `site.Get("blog/hello.md").Meta()` returns the updated metadata

#### Scenario: New file added via UpdateFile
- **WHEN** `UpdateFile` is called for a file path not previously in the index
- **THEN** the page is added to the index and appears in `site.All()`

#### Scenario: Layout update via UpdateFile
- **WHEN** `UpdateFile` is called for a file under `_layouts/`
- **THEN** layout templates are re-discovered and the updated templates are used for subsequent renders

---

### Requirement: Per-file deletion
The system SHALL provide a `DeleteFile(filePath string)` method on `fsSite` that removes a single file's entry from the index under a write lock. This method is intended for use by the fsnotify event handler.

#### Scenario: File removed from index
- **WHEN** `DeleteFile("blog/hello.md")` is called
- **THEN** `site.Get("blog/hello.md")` returns `nil` and the page is absent from `site.All()`

#### Scenario: Index file deletion falls back to lower priority
- **WHEN** `DeleteFile("blog/index.html")` is called and `blog/index.md` exists on disk
- **THEN** `blog/index.md` becomes the directory index for `"blog"`

#### Scenario: Deleting the only index file removes directory entry
- **WHEN** `DeleteFile("blog/index.md")` is called and no other index file exists for `blog/`
- **THEN** `site.Get("blog")` returns `nil`

---

### Requirement: Concurrent access safety
The system SHALL guarantee that concurrent render requests never observe a partially-updated index. A read lock SHALL be acquired at the start of each `FileRenderer`/`DirRenderer` invocation and held for the entire duration of `Render()` — covering all template-level Site method calls. File I/O for refresh operations SHALL occur outside the write lock; the write lock SHALL be held only for in-place map mutations and children index rebuild.

#### Scenario: Render uses a consistent index snapshot
- **WHEN** a `FileRenderer` or `DirRenderer` hook is invoked
- **THEN** a read lock is held from that point through the completion of `Render(w, r)`, so all template query method calls on `RenderContext.Site` read a consistent index

#### Scenario: Refresh waits for in-flight renders
- **WHEN** `Refresh()` is called while renders are in progress
- **THEN** the write lock is not acquired until all active renders have completed

#### Scenario: Write lock held only for in-memory operations
- **WHEN** `Refresh()` is called
- **THEN** file parsing happens before the write lock is acquired; the lock is held only for map mutations and children rebuild

---

### Requirement: fsnotify watcher with recursive subdirectory watching
The system SHALL support a `watch: true` configuration on the `pages` handler that starts an fsnotify watcher. The watcher SHALL watch the content root and all its subdirectories recursively (fsnotify on Linux is non-recursive; all subdirectories are added explicitly at startup via `addWatchDirs`). New directories created at runtime SHALL be added to the watcher immediately. Only `Write`, `Create`, `Remove`, `Rename` events SHALL trigger index updates; other events (e.g. `Chmod`) SHALL be ignored. The watcher SHALL debounce events per-file with a ~400ms window and call `UpdateFile` or `DeleteFile` as appropriate. The `pages` handler SHALL maintain a per-route rolling counter of refreshed files (via `expvar` + `metric.NewCounter`, mirroring the `requests` counter in `wrapWithStats`) incremented by 1 for each `UpdateFile()` or `DeleteFile()` call.

#### Scenario: File edit in subdirectory triggers index update
- **WHEN** `watch: true` is set and a page file in a subdirectory is saved on disk
- **THEN** within ~500ms, the template query for that page reflects the updated frontmatter without a server restart

#### Scenario: File deletion triggers index removal
- **WHEN** `watch: true` is set and a page file is deleted
- **THEN** within ~500ms, the page is absent from the site index

#### Scenario: New subdirectory is watched
- **WHEN** `watch: true` is set and a new subdirectory is created under the content root
- **THEN** files subsequently created in that subdirectory generate watch events

#### Scenario: Rapid edits debounced to single update
- **WHEN** a file receives multiple write events within the debounce window
- **THEN** `UpdateFile` is called exactly once after the window expires

#### Scenario: Refreshed file count is tracked
- **WHEN** `UpdateFile()` or `DeleteFile()` is called
- **THEN** the `refreshed_files` expvar counter for the route is incremented by 1 and visible at `/debug/vars`

#### Scenario: Watcher shuts down with server
- **WHEN** the server context is cancelled
- **THEN** the fsnotify watcher goroutine exits cleanly and all pending debounce timers are cancelled