## MODIFIED Requirements

### Requirement: Two-phase content loading
The system SHALL use a two-phase approach in the FS-backed implementation: metadata (front matter / JSON-LD) is parsed at index construction time to build the index; the page body is re-read from the FS at render time and is always current. The index SHALL be refreshable after construction via `Refresh()`, `UpdateFile()`, and `DeleteFile()` methods, which re-parse metadata for changed files only. A `FileRenderer` or `DirRenderer` hook invocation SHALL read the page and layout under a single read lock acquisition so they are consistent with each other.

#### Scenario: Metadata loaded at construction
- **WHEN** `NewSite` completes
- **THEN** `page.Meta()` returns the parsed metadata without re-reading the file

#### Scenario: Body content read at render time
- **WHEN** a file's body changes between index construction and `page.Render`
- **THEN** `page.Render` reads the updated body from the FS

#### Scenario: Metadata refreshable after construction
- **WHEN** a file's frontmatter changes on disk and `Refresh()` or `UpdateFile()` is called
- **THEN** subsequent calls to `page.Meta()` for that page return the updated metadata

#### Scenario: Renderer uses consistent page and layout
- **WHEN** `FileRenderer` or `DirRenderer` is invoked for a request
- **THEN** the page lookup and layout used for rendering are read under the same lock acquisition
