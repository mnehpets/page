# Template API

Layout templates are standard Go `html/template` files stored under `_layouts/`.

## Layout directory structure

```
_layouts/
  base/
    entry.html     ← common base template; defines "entry" and calls {{block "main" .}}
    nav.html       ← any other shared templates called from entry
    post-entry.html  ← shared template for post-like layouts; defines "post-entry" and calls {{block "main" .}}
  default.html     ← "default" entry-point template; defines "main" and uses the base "entry" template
  post.html        ← "post" entry-point template; defines "main" and defines "entryname" as "post-entry"
  sitemap.xml      ← standalone entry-point template; defines "entry" to override the base entry
```

Content declares a single `layout:` name (e.g. `layout: post`). The renderer
picks the matching entry-point template, combines it with the base templates,
and executes `entry`. The entry-point's `{{define "main"}}` block overrides
the `{{block "main" .}}` placeholder in the base.

**`entry` is the execution entry point** — the renderer always executes the
template named `"entry"`, in every mode:

- **With base templates**: `"entry"` is defined in `_layouts/base/entry.html`
  and calls `{{block "main" .}}`; the layout's entry-point file overrides that
  block with `{{define "main"}}`.
- **Without base templates**: `"entry"` is defined directly in the entry-point
  file, which owns its full output with no shared outer shell.

An entry-point template can select a different execution entry point by defining
an `"entryname"` template whose text body is the desired template name:

```html
{{define "entryname"}}wide-entry{{end}}
```

The renderer executes this template with no data, trims whitespace from the
result, and uses it as the entry-point name. The named template must be defined
in one of the base files. This allows multiple layouts to share the same base
directory while each routing to a different outer shell.

An entry-point template can also define `{{define "entry"}}` inline to replace
the base shell entirely for that layout. This is useful for layouts that produce
non-HTML output (XML sitemaps, Atom feeds) where the standard `<html>` wrapper
is unwanted — the entry-point's definition wins because it is parsed after the
base files.

When no `_layouts/base/` directory is present, entry-point files must define
`{{define "entry"}}` directly and own their full output — a simpler mode
suitable for sites that don't need a shared outer structure.

When a page doesn't have a `layout:` declaration, the `default` entry-point is used.

---

When a page is rendered, the `RenderContext` value is the dot (`.`) for the
executing template.

## RenderContext fields

| Field | Type | Description |
|-------|------|-------------|
| `.Content` | `template.HTML` | The rendered page body — safe HTML, not escaped again. For markdown pages this is the converted HTML; for HTML pages it is the inner HTML of `<body>`. |
| `.Head` | `template.HTML` | Extra `<head>` elements from an HTML page (everything except `<title>`, recognised `<meta>` tags, and JSON-LD). Empty for markdown pages. |
| `.JSONLD` | `template.JS` | A schema.org JSON-LD blob ready to drop into `<script type="application/ld+json">{{ .JSONLD }}</script>`. Empty string when no metadata was available. |
| `.Meta` | `Meta` | Parsed page metadata. See the [Meta fields](#meta-fields) table below. |
| `.Config` | `SiteConfig` | Operator-supplied site configuration. See [SiteConfig fields](#siteconfig-fields). |
| `.SitePath` | `string` | Site-relative path of the current page, e.g. `blog/hello.md` or `blog` for a directory index. The root index page has path `"."`. Use this with `Site` methods such as `.Site.AncestorsOf .SitePath`. |
| `.Site` | `Site` | The site index. Provides query methods for iterating over all pages. See [Site methods](#site-methods). |
| `.Request` | `*http.Request` | The incoming HTTP request. Useful for reading the URL, query parameters, or headers. |

## RenderContext methods

These are the primary link-generation operations for templates.

| Call | Returns | Description |
|------|---------|-------------|
| `.Href target` | `string` | Returns a URL relative to the current page. `target` may be a `Page`, a `RenderContext`, or a site-relative path string. Typical use inside `range`: `{{ $.Href . }}`. |
| `.AbsURL target` | `string` | Returns the canonical absolute URL for `target`. `target` may be a `Page`, a `RenderContext`, or a site-relative path string. Typical use in feeds/sitemaps: `{{ $.AbsURL . }}`. |

## Meta fields

Accessed as `.Meta.FieldName` in a template.

| Field | Type | Description |
|-------|------|-------------|
| `.Meta.Title` | `string` | Page title. |
| `.Meta.Author` | `string` | Author name. |
| `.Meta.Date` | `time.Time` | Publication date. Falls back to the file modification time if not set. |
| `.Meta.LastMod` | `time.Time` | Last-modified time. Set from `lastmod` frontmatter (or `dateModified` JSON-LD for HTML pages); always falls back to the file modification time. |
| `.Meta.Description` | `string` | Short description or summary. |
| `.Meta.Tags` | `[]string` | List of tags. |
| `.Meta.Collection` | `string` | Collection name, for grouping related pages. |
| `.Meta.Image.URL` | `string` | URL of the representative image. |
| `.Meta.Image.Alt` | `string` | Alt text for the representative image. |
| `.Meta.Slug` | `string` | URL-friendly identifier. Derived from the file name if not set explicitly. |
| `.Meta.LinkTitle` | `string` | Short title for navigation links. Falls back to `.Meta.Title` when empty. |
| `.Meta.Layout` | `string` | The layout name for this page. Rarely needed inside a template. |
| `.Meta.Draft` | `bool` | Whether this page is a draft. |
| `.Meta.ContentType` | `string` | MIME type of the HTTP response (e.g. `application/xml`). Defaults to `text/html`. |

## SiteConfig fields

Accessed as `.Config.FieldName`.

| Field | Type | Description |
|-------|------|-------------|
| `.Config.BaseURL` | `string` | Canonical base URL of the site, including routing prefix, e.g. `https://example.com/docs` (no trailing slash). `RenderContext.AbsURL` uses this to build canonical absolute URLs. |
| `.Config.Name` | `string` | Human-readable site name. |
| `.Config.Lang` | `string` | BCP 47 language tag, e.g. `en`. Defaults to `en` when empty. |

## Site methods

The `.Site` value is available to all templates and supports the following calls.

All path arguments and return values use the **site-relative** form: no leading
slash, directories without a trailing slash, and `"."` for the site root. The
current page's site-relative path is available as `.SitePath` on `RenderContext`.
Path values are primarily useful for lookups and site navigation; for link generation,
prefer `.Href` and `.AbsURL` on `RenderContext`.

| Call | Returns | Description |
|------|---------|-------------|
| `.Site.All` | `[]Page` | All non-draft pages in the site. |
| `.Site.ByTag "name"` | `[]Page` | Pages that carry the given tag. |
| `.Site.ByCollection "name"` | `[]Page` | Pages in the named collection. |
| `.Site.Get "blog/hello.md"` | `Page` | Look up a single page by its site-relative path. Returns `nil` if no page is registered at that path. |
| `.Site.AncestorsOf "blog/post.md"` | `[]Page` | Pages on the path from the root down to (but not including) the given path, ordered root-first. Intermediate paths absent from the site index are skipped. Useful for breadcrumb navigation. |
| `.Site.ChildrenOf "blog"` | `[]Page` | Pages that are exactly one path segment deeper than the given path. |
| `.Site.SiblingsOf "blog/post.md"` | `[]Page` | Pages that share the same parent as the given path (i.e. children of its parent). For the root path `"."`, returns the root page itself. Useful for peer navigation. |
| `.Site.Config` | `SiteConfig` | Same value as `.Config`. |

Pages returned by Site methods expose two callable fields:

| Call | Returns | Description |
|------|---------|-------------|
| `.SitePath` | `string` | The page's site-relative path, e.g. `blog/hello.md` or `blog` for a directory index. The root index page has path `"."`. Also available on the top-level `RenderContext` as `.SitePath` for the currently-rendering page. |
| `.Meta` | `Meta` | The page's metadata (same fields as above). |

## Template functions

These functions are available in all layout templates.

| Function | Signature | Description |
|----------|-----------|-------------|
| `sortByDate` | `sortByDate pages` | Returns a new `[]Page` sorted by `Meta.Date` descending (newest first). Pages with no date sort last. |
| `sortByLastMod` | `sortByLastMod pages` | Returns a new `[]Page` sorted by `Meta.LastMod` descending (most recently modified first). |
| `sortByPath` | `sortByPath pages` | Returns a new `[]Page` sorted lexicographically by `SitePath` ascending. |
| `hasTag` | `hasTag meta "tag"` | Reports whether a `Meta` carries the given tag. Use `.Meta` when ranging over pages, or `.Meta` at the top level. |
| `json` | `json value` | Marshals any value to a JSON literal (`template.JS`). Useful for passing structured data to JavaScript. |
| `safeHTML` | `safeHTML "string"` | Marks a string as safe HTML, bypassing contextual escaping. Use only with trusted, statically-known strings — never with user input. |
| `parentPath` | `parentPath "blog/post.md"` | Returns the parent path. `parentPath("blog/post.md")` → `"blog"`, `parentPath("blog")` → `"."`, `parentPath(".")` → `""`. Pure string operation; does not consult the site index. |

## Examples

### Blog index listing recent posts

`_layouts/default.html` — entry-point used when no `layout:` is declared:

```html
{{ define "main" }}
<h1>Recent posts</h1>
<ul>
{{- range sortByDate (.Site.ByCollection "posts") }}
  <li>
    <a href="{{ $.Href . }}">{{ .Meta.Title }}</a>
    — {{ .Meta.Date.Format "2 January 2006" }}
  </li>
{{- end }}
</ul>
{{ end }}
```

### XML sitemap

`_layouts/sitemap.html` — a standalone layout that owns its full output.
Because it defines `entry` itself, it replaces the base template's HTML shell
for this layout only:

```html
{{ define "entry" }}{{ safeHTML "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" }}
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
{{- range .Site.ByTag "mime:text/html" }}
{{- if not (hasTag .Meta "noindex") }}
  <url>
    <loc>{{ $.AbsURL . }}</loc>
  </url>
{{- end }}
{{- end }}
</urlset>
{{ end }}
```

The sitemap page declares `contentType: application/xml` and `layout: sitemap`.
Defining `entry` in the entry-point file overrides the base template's
`entry` for this layout, so no HTML wrapper is emitted.

### Navigation sidebar

A TOC-style sidebar showing the breadcrumb trail, siblings, and children of
the current page. Siblings are obtained with `ChildrenOf` on the parent path.

```html
{{ define "sidebar" }}
{{- $cur := .SitePath }}
<nav class="sidebar">
  {{- $ancestors := .Site.AncestorsOf $cur }}
  {{- if $ancestors }}
  <ol class="breadcrumb">
    {{- range $ancestors }}
    <li><a href="{{ $.Href . }}">{{ or .Meta.LinkTitle .Meta.Title .Meta.Slug }}</a></li>
    {{- end }}
  </ol>
  {{- end }}

  {{- $siblings := sortByPath (.Site.ChildrenOf (parentPath $cur)) }}
  {{- if $siblings }}
  <ul class="siblings">
    {{- range $siblings }}{{- if not (hasTag .Meta "noindex") }}
    <li{{if eq .SitePath $cur}} class="current"{{end}}>
      <a href="{{ $.Href . }}">{{ or .Meta.LinkTitle .Meta.Title .Meta.Slug }}</a>
    </li>
    {{- end }}{{- end }}
  </ul>
  {{- end }}

  {{- $children := sortByPath (.Site.ChildrenOf $cur) }}
  {{- if $children }}
  <ul class="children">
    {{- range $children }}{{- if not (hasTag .Meta "noindex") }}
    <li><a href="{{ $.Href . }}">{{ or .Meta.LinkTitle .Meta.Title .Meta.Slug }}</a></li>
    {{- end }}{{- end }}
  </ul>
  {{- end }}
</nav>
{{ end }}
```

### Multiple base templates

When a site needs more than one outer HTML shell (e.g. a standard page and a
full-width landing page), add a second base template to `_layouts/base/` and
point each layout that needs it at the right one via `entryname`.

`_layouts/base/entry.html` — standard shell with a sidebar slot:

```html
{{ define "entry" }}
<!DOCTYPE html>
<html>
<body>
  <aside>{{ block "sidebar" . }}{{ end }}</aside>
  <main>{{ block "main" . }}{{ end }}</main>
</body>
</html>
{{ end }}
```

`_layouts/base/wide-entry.html` — full-width shell, no sidebar:

```html
{{ define "wide-entry" }}
<!DOCTYPE html>
<html>
<body>
  <main class="wide">{{ block "main" . }}{{ end }}</main>
</body>
</html>
{{ end }}
```

`_layouts/landing.html` — selects the wide shell:

```html
{{define "entryname"}}wide-entry{{end}}

{{define "main"}}
<section class="hero">{{ .Content }}</section>
{{end}}
```

All other entry-point templates that do not define `entryname` continue to use
`entry`. The base files are compiled into every layout's template set, so
`wide-entry` is available to any layout that asks for it.

### Injecting JSON-LD and passing data to JavaScript

`_layouts/base/entry.html` — the shared outer shell:

```html
{{ define "entry" }}
<!DOCTYPE html>
<html lang="{{ .Config.Lang }}">
<head>
  <meta charset="utf-8">
  <title>{{ or .Meta.Title .Config.Name }}</title>
  {{ .Head }}
  <script>
    const PAGE = {{ json .Meta }};
  </script>
</head>
<body>
  {{ block "main" . }}{{ end }}
</body>
</html>
{{ end }}
```

`_layouts/article.html` — entry-point for article pages:

```html
{{ define "main" }}
<article>
  {{- if .JSONLD }}
  <script type="application/ld+json">{{ .JSONLD }}</script>
  {{- end }}
  {{ .Content }}
</article>
{{ end }}
```

Content declares `layout: article`. The renderer combines `entry.html` with
`article.html`, executes `entry`, and the `{{block "main" .}}` is filled by
`article.html`'s `{{define "main"}}`.
