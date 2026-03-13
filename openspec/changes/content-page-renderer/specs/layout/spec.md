## ADDED Requirements

### Requirement: Layout type
The system SHALL define a `Layout` type that wraps a parsed `*html/template` set. A `Layout` MAY contain multiple named templates. The layout name used for a given page is taken from `Meta.Layout`; if `Meta.Layout` is empty, a template named `"default"` MUST be used as the fallback.

#### Scenario: Named layout template executed
- **WHEN** `page.Render` is called and `meta.Layout` is `"post"`
- **THEN** the layout executes the template named `"post"` with `RenderContext`

#### Scenario: Default layout used when layout name is empty
- **WHEN** `page.Render` is called and `meta.Layout` is `""`
- **THEN** the layout executes the template named `"default"` with `RenderContext`

#### Scenario: Unknown layout name returns error
- **WHEN** `page.Render` is called and `meta.Layout` names a template not present in the `Layout`
- **THEN** `page.Render` returns a non-nil error

---

### Requirement: Layout constructor
The system SHALL provide `NewLayout(fsys fs.FS, patterns ...string) (*Layout, error)` that parses all matching files from `fsys` as `html/template` templates. Template names are determined by `{{define "name"}}` declarations within the files, as per `html/template` semantics.

#### Scenario: Templates parsed from FS
- **WHEN** `NewLayout` is called with an FS containing files that declare `{{define "default"}}` and `{{define "post"}}`
- **THEN** the returned `Layout` contains templates named `"default"` and `"post"`

#### Scenario: Parse error returns error
- **WHEN** `NewLayout` is called and a template file contains invalid `html/template` syntax
- **THEN** it returns a non-nil error

#### Scenario: Empty pattern list returns error
- **WHEN** `NewLayout` is called with patterns that match no files
- **THEN** it returns a non-nil error

---

### Requirement: RenderContext template data
The system SHALL define a `RenderContext` struct passed to the layout template during execution. `RenderContext` MUST contain: `Content template.HTML` (the rendered page body), `Head template.HTML` (page-specific head elements), `Meta Meta` (the page's metadata), `Site Site` (the site interface for template queries), and `Request *http.Request` (the current HTTP request).

For `.html` files, `Head` contains the inner HTML of the source `<head>` element excluding fields already captured by `Meta`. For `.md` files, `Head` contains any head content produced by the Markdown renderer (e.g. extension-injected CSS or scripts); it is empty when no extensions contribute head content.

#### Scenario: RenderContext fields accessible in template
- **WHEN** a layout template references `{{.Content}}`, `{{.Head}}`, `{{.Meta.Title}}`, `{{.Site}}`, or `{{.Request}}`
- **THEN** the correct values are rendered

#### Scenario: Content is safe HTML
- **WHEN** `RenderContext.Content` is used in a layout template with `{{.Content}}`
- **THEN** the content is rendered unescaped (as `template.HTML`)

#### Scenario: Head is safe HTML
- **WHEN** `RenderContext.Head` is used in a layout template with `{{.Head}}`
- **THEN** the head content is rendered unescaped (as `template.HTML`)

#### Scenario: Markdown page with no head-generating extensions
- **WHEN** a `.md` file is rendered using basic goldmark with no extensions that produce head content
- **THEN** `RenderContext.Head` is empty

#### Scenario: HTML page head content extracted
- **WHEN** an `.html` file contains a `<head>` with a `<link rel="stylesheet">` not captured by `Meta`
- **THEN** `RenderContext.Head` contains that `<link>` element

---

### Requirement: _layouts/ convention in NewSite
The system SHALL automatically discover and parse layout templates from a `_layouts/` subdirectory within the content `fs.FS` at `NewSite` construction time. If no `_layouts/` directory exists, `NewSite` MUST succeed and the `Layout` MUST be nil (callers are responsible for providing a layout or handling the nil case).

#### Scenario: _layouts/ directory parsed at construction
- **WHEN** `NewSite` is called with an FS containing a `_layouts/` directory
- **THEN** the site's layout is populated from those template files

#### Scenario: No _layouts/ directory
- **WHEN** `NewSite` is called with an FS containing no `_layouts/` directory
- **THEN** `NewSite` succeeds and the site has a nil layout

#### Scenario: WithLayout overrides _layouts/ convention
- **WHEN** `NewSite` is called with a `WithLayout(l)` option and the FS also contains `_layouts/`
- **THEN** the provided `Layout` `l` is used and `_layouts/` is ignored
