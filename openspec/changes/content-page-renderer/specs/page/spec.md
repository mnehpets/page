## ADDED Requirements

### Requirement: Page interface
The system SHALL define a `Page` interface with three methods: `URLPath() string`, `Meta() Meta`, and `Render(w io.Writer, r *http.Request, site Site, layout *Layout) error`. The primary responsibility of a `Page` is rendering its content; `URLPath` and `Meta` are secondary, supporting the `Site` index and template queries.

#### Scenario: Page renders to writer
- **WHEN** `page.Render(w, r, site, layout)` is called
- **THEN** the page writes its fully-rendered output (content wrapped in layout) to `w` and returns nil on success

#### Scenario: Page exposes URL path
- **WHEN** `page.URLPath()` is called
- **THEN** it returns the URL path under which the page is registered (e.g. `/blog/hello-world.md`)

#### Scenario: Page exposes metadata
- **WHEN** `page.Meta()` is called
- **THEN** it returns the parsed `Meta` struct for the page

---

### Requirement: Meta struct
The system SHALL define a `Meta` struct with the following fields: `Title string`, `Author string`, `Date time.Time`, `Tags []string`, `Collection string`, `Layout string`, `Draft bool`, `Description string`, `Image Image`, `Slug string`. All fields are optional; unpopulated fields carry zero values.

`Image` is a struct with `URL string` and `Alt string`. In front matter it may be expressed as a nested mapping (`image: {url: ..., alt: ...}`) or as a bare string (URL only, `Alt` defaults to `""`). In JSON-LD it maps from the `image` property (object or string); in HTML `<meta>` it maps from `og:image` (URL) and `og:image:alt` (Alt).

`Slug` is populated from an explicit `slug` key in front matter / JSON-LD if present, otherwise derived from the last segment of the page's URL path with the content extension stripped (e.g. `/blog/hello-world.md` → `hello-world`). `Date` represents last-modified date, falling back to `fs.FileInfo.ModTime()` when not set explicitly.

Metadata is resolved using a three-level fallback chain, applied per field (first non-zero value wins):
1. **Front matter / JSON-LD** — explicit metadata in the file's primary metadata block
2. **HTML `<meta>` tags** — for `.html` files only; standard `<meta name="...">` and `<meta property="...">` tags in `<head>`
3. **File system metadata** — `fs.FileInfo` from the FS (e.g. `ModTime` → `Date`); applies to both `.md` and `.html` files

#### Scenario: All fields populated from front matter
- **WHEN** a Markdown file contains YAML front matter with `title`, `date`, `tags`, `collection`, `layout`, `draft`, and `description` keys
- **THEN** `page.Meta()` returns a `Meta` with all fields populated from front matter

#### Scenario: Front matter takes precedence over HTML meta
- **WHEN** an HTML file has a `name` field in JSON-LD and also a `<meta property="og:title">` tag
- **THEN** `page.Meta().Title` is populated from JSON-LD

#### Scenario: HTML meta used when JSON-LD field absent
- **WHEN** an HTML file has no `name` in JSON-LD but has `<meta property="og:title" content="My Page">`
- **THEN** `page.Meta().Title` is `"My Page"`

#### Scenario: FS metadata used when primary sources absent
- **WHEN** a file has no `date` in its front matter or JSON-LD (and, for HTML, no relevant `<meta>` tag)
- **THEN** `page.Meta().Date` is populated from `fs.FileInfo.ModTime()`

#### Scenario: Slug derived from URL path when not set explicitly
- **WHEN** a file has no `slug` in its front matter or JSON-LD
- **THEN** `page.Meta().Slug` is the last segment of the page's URL path with the content extension stripped (e.g. `hello-world` for `/blog/hello-world.md`)

#### Scenario: Slug derived from index file uses directory name
- **WHEN** a file has no `slug` in its front matter or JSON-LD and its URL path is a directory path (e.g. `/blog/staff` from `blog/staff/index.md`)
- **THEN** `page.Meta().Slug` is the last segment of the URL path (e.g. `staff`)

#### Scenario: Explicit slug overrides derived slug
- **WHEN** a file has `slug: custom-slug` in its front matter
- **THEN** `page.Meta().Slug` is `"custom-slug"`

#### Scenario: Missing fields with no fallback default to zero values
- **WHEN** a file has no `collection` in any metadata source and no FS equivalent
- **THEN** `page.Meta().Collection` is `""`

---

### Requirement: PageFromFile auto-detection
The system SHALL provide a `PageFromFile(urlPath string, f fs.File) (Page, error)` function that detects the page type from the file's name extension. For unrecognised extensions it MUST return `nil, nil`; the hook treats a nil page as a signal to fall through to the default file handler.

#### Scenario: Markdown file detected
- **WHEN** `PageFromFile` is called with a file whose name ends in `.md`
- **THEN** it returns a `markdownPage`

#### Scenario: HTML file detected
- **WHEN** `PageFromFile` is called with a file whose name ends in `.html` or `.htm`
- **THEN** it returns an `htmlPage`

#### Scenario: Unrecognised extension returns nil
- **WHEN** `PageFromFile` is called with a file whose name has any other extension (e.g. `.css`, `.png`)
- **THEN** it returns `nil, nil`

#### Scenario: Unreadable content file returns error
- **WHEN** `PageFromFile` is called with a `.md` or `.html` file that cannot be read
- **THEN** it returns a non-nil error

---

### Requirement: markdownPage parses YAML front matter
The system SHALL parse YAML front matter from `.md` files. Front matter MUST be delimited by `---` on its own line at the start of the file and terminated by a second `---` line. The body is everything after the closing delimiter.

#### Scenario: Front matter parsed
- **WHEN** a `.md` file begins with `---\n...\n---\n`
- **THEN** the YAML between the delimiters is parsed into `Meta`

#### Scenario: No front matter
- **WHEN** a `.md` file does not begin with `---`
- **THEN** `Meta` is zero-valued and the entire file content is treated as the Markdown body

#### Scenario: Malformed YAML returns error
- **WHEN** a `.md` file has `---`-delimited content that is not valid YAML
- **THEN** `PageFromFile` returns a non-nil error

---

### Requirement: markdownPage renders Markdown via goldmark
The system SHALL render the Markdown body to HTML using `github.com/yuin/goldmark`. The renderer MUST NOT apply server-side syntax highlighting; fenced code blocks MUST emit `<code class="language-<lang>">` elements for client-side highlighting.

#### Scenario: Markdown body rendered to HTML
- **WHEN** `page.Render` is called on a `markdownPage`
- **THEN** the page renders the Markdown body to HTML, populates `RenderContext.Content` (and `RenderContext.Head` if the renderer produces head content), and executes the layout template with that `RenderContext`, writing the result to `w`

#### Scenario: Fenced code block emits language class
- **WHEN** a Markdown file contains a fenced code block with an info string (e.g. ` ```go `)
- **THEN** the rendered HTML contains `<code class="language-go">`

---

### Requirement: htmlPage parses JSON-LD metadata
The system SHALL parse metadata from `.html` files via a `<script type="application/ld+json">` block in `<head>`. Recognised JSON-LD properties SHALL be mapped to `Meta` fields. A `layout` property in the JSON-LD object SHALL populate `Meta.Layout`. For fields absent from JSON-LD, the system SHALL fall back to standard HTML `<meta>` tags (e.g. `<meta name="description">`, `<meta property="og:title">`, `<meta name="keywords">`) and the `<title>` element (`Meta.Title`).

#### Scenario: JSON-LD metadata parsed
- **WHEN** an `.html` file contains a `<script type="application/ld+json">` block with a `name` property
- **THEN** `page.Meta().Title` is populated from `name`

#### Scenario: JSON-LD layout field parsed
- **WHEN** an `.html` file contains a JSON-LD block with a `layout` property
- **THEN** `page.Meta().Layout` is populated with that value

#### Scenario: head > title used as Title fallback
- **WHEN** an `.html` file has no `name` in JSON-LD but has a `<title>` element in `<head>`
- **THEN** `page.Meta().Title` is populated from the `<title>` element

#### Scenario: HTML meta tag used as fallback
- **WHEN** an `.html` file has no JSON-LD block (or JSON-LD lacks a field) but has a corresponding `<meta>` tag
- **THEN** `page.Meta()` is populated from the `<meta>` tag for that field

#### Scenario: No JSON-LD block and no meta tags
- **WHEN** an `.html` file contains neither a JSON-LD block nor relevant `<meta>` tags
- **THEN** `Meta` fields with no FS fallback are zero-valued and the file is still served as `htmlPage`

#### Scenario: Invalid JSON returns error
- **WHEN** an `.html` file contains a `<script type="application/ld+json">` block with invalid JSON
- **THEN** `PageFromFile` returns a non-nil error

---

### Requirement: htmlPage renders HTML content
The system SHALL render `.html` pages by passing the HTML file's body content as `template.HTML` to the layout template. The raw HTML MUST NOT be re-escaped.

#### Scenario: HTML content rendered through layout
- **WHEN** `page.Render` is called on an `htmlPage`
- **THEN** the page passes its body as `template.HTML` to the layout and writes the result to `w`

