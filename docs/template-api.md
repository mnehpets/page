# Template API

Layout templates are standard Go `html/template` files. When a page is rendered,
the template named by the page's `layout` (or `default` if none is declared)
receives a `RenderContext` value as its dot (`.`).

## RenderContext fields

| Field | Type | Description |
|-------|------|-------------|
| `.Content` | `template.HTML` | The rendered page body — safe HTML, not escaped again. For markdown pages this is the converted HTML; for HTML pages it is the inner HTML of `<body>`. In a multi-step layout chain each step's output becomes `.Content` for the next. |
| `.Head` | `template.HTML` | Extra `<head>` elements from an HTML page (everything except `<title>`, recognised `<meta>` tags, and JSON-LD). Empty for markdown pages. |
| `.JSONLD` | `template.JS` | A schema.org JSON-LD blob ready to drop into `<script type="application/ld+json">{{ .JSONLD }}</script>`. Empty string when no metadata was available. |
| `.Meta` | `Meta` | Parsed page metadata. See the [Meta fields](#meta-fields) table below. |
| `.Config` | `SiteConfig` | Operator-supplied site configuration. See [SiteConfig fields](#siteconfig-fields). |
| `.Site` | `Site` | The site index. Provides query methods for iterating over all pages. See [Site methods](#site-methods). |
| `.Request` | `*http.Request` | The incoming HTTP request. Useful for reading the URL, query parameters, or headers. |

## Meta fields

Accessed as `.Meta.FieldName` in a template.

| Field | Type | Description |
|-------|------|-------------|
| `.Meta.Title` | `string` | Page title. |
| `.Meta.Author` | `string` | Author name. |
| `.Meta.Date` | `time.Time` | Publication date. Falls back to the file modification time if not set. |
| `.Meta.Description` | `string` | Short description or summary. |
| `.Meta.Tags` | `[]string` | List of tags. |
| `.Meta.Collection` | `string` | Collection name, for grouping related pages. |
| `.Meta.Image.URL` | `string` | URL of the representative image. |
| `.Meta.Image.Alt` | `string` | Alt text for the representative image. |
| `.Meta.Slug` | `string` | URL-friendly identifier. Derived from the file name if not set explicitly. |
| `.Meta.Layouts` | `[]string` | The layout pipeline for this page. Rarely needed inside a template. |
| `.Meta.Draft` | `bool` | Whether this page is a draft. |
| `.Meta.ContentType` | `string` | MIME type of the HTTP response (e.g. `application/xml`). Defaults to `text/html`. |

## SiteConfig fields

Accessed as `.Config.FieldName`.

| Field | Type | Description |
|-------|------|-------------|
| `.Config.BaseURL` | `string` | Canonical base URL, e.g. `https://example.com` (no trailing slash). Use this to build absolute URLs: `{{ printf "%s%s" .Config.BaseURL .Meta.Slug }}`. |
| `.Config.Name` | `string` | Human-readable site name. |
| `.Config.Lang` | `string` | BCP 47 language tag, e.g. `en`. Defaults to `en` when empty. |

## Site methods

The `.Site` value is available to all templates and supports the following calls.

| Call | Returns | Description |
|------|---------|-------------|
| `.Site.All` | `[]Page` | All non-draft pages in the site. |
| `.Site.ByTag "name"` | `[]Page` | Pages that carry the given tag. |
| `.Site.ByCollection "name"` | `[]Page` | Pages in the named collection. |
| `.Site.Get "/path/"` | `Page, bool` | Look up a single page by its URL path. |
| `.Site.AncestorsOf "/path/"` | `[]Page` | Pages on the path from the root down to (but not including) the given URL path, ordered root-first. Intermediate paths absent from the site index are skipped. Useful for breadcrumb navigation. |
| `.Site.ChildrenOf "/path/"` | `[]Page` | Pages that are exactly one path segment deeper than the given URL path. Use `ChildrenOf (parentPath .Request.URL.Path)` to get siblings of the current page. |
| `.Site.Config` | `SiteConfig` | Same value as `.Config`. |

Pages returned by Site methods expose two callable fields:

| Call | Returns | Description |
|------|---------|-------------|
| `.URLPath` | `string` | The page's URL path, e.g. `/notes/hello/`. |
| `.Meta` | `Meta` | The page's metadata (same fields as above). |

## Template functions

These functions are available in all layout templates.

| Function | Signature | Description |
|----------|-----------|-------------|
| `sortByDate` | `sortByDate pages` | Returns a new `[]Page` sorted by `Meta.Date` descending (newest first). Pages with no date sort last. |
| `sortByPath` | `sortByPath pages` | Returns a new `[]Page` sorted lexicographically by `URLPath` ascending. |
| `hasTag` | `hasTag page "tag"` | Reports whether a `Page` carries the given tag. |
| `json` | `json value` | Marshals any value to a JSON literal (`template.JS`). Useful for passing structured data to JavaScript. |
| `safeHTML` | `safeHTML "string"` | Marks a string as safe HTML, bypassing contextual escaping. Use only with trusted, statically-known strings — never with user input. |
| `parentPath` | `parentPath "/a/b/"` | Returns the parent URL path. `parentPath("/a/b/")` → `"/a/"`, `parentPath("/a/")` → `"/"`, `parentPath("/")` → `""`. Pure string operation; does not consult the site index. |

## Layout chains

A page can declare a multi-step rendering pipeline via the `layouts` frontmatter
key. Each template except the last renders into a buffer; its output becomes
`.Content` for the next step. This is useful for wrapping content in intermediate
containers before the outer page shell is applied.

```yaml
# Render body → "article" → "page"
layouts: [article, page]
```

If no layout is declared, the template named `default` is used.

## Examples

### Blog index listing recent posts

```html
{{ define "default" }}
<!DOCTYPE html>
<html lang="{{ .Config.Lang }}">
<head>
  <meta charset="utf-8">
  <title>{{ .Config.Name }}</title>
  {{ .Head }}
</head>
<body>
  <h1>Recent posts</h1>
  <ul>
  {{- range sortByDate (.Site.ByCollection "posts") }}
    <li>
      <a href="{{ .URLPath }}">{{ .Meta.Title }}</a>
      — {{ .Meta.Date.Format "2 January 2006" }}
    </li>
  {{- end }}
  </ul>
</body>
</html>
{{ end }}
```

### XML sitemap

```html
{{ define "sitemap" }}
{{ safeHTML "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" }}
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
{{- range .Site.ByTag "mime:text/html" }}
{{- if not (hasTag . "noindex") }}
  <url>
    <loc>{{ printf "%s%s" $.Config.BaseURL .URLPath }}</loc>
  </url>
{{- end }}
{{- end }}
</urlset>
{{ end }}
```

The sitemap page would declare `content-type: application/xml` so the response
is served with the correct MIME type, and `layout: sitemap` to select the template above.

### Navigation sidebar

A TOC-style sidebar showing the breadcrumb trail, siblings, and children of
the current page. Siblings are obtained with `ChildrenOf` on the parent path.

```html
{{ define "sidebar" }}
{{- $cur := .Request.URL.Path }}
<nav class="sidebar">
  {{- $ancestors := .Site.AncestorsOf $cur }}
  {{- if $ancestors }}
  <ol class="breadcrumb">
    {{- range $ancestors }}
    <li><a href="{{ .URLPath }}">{{ or .Meta.Title .Meta.Slug }}</a></li>
    {{- end }}
  </ol>
  {{- end }}

  {{- $siblings := sortByPath (.Site.ChildrenOf (parentPath $cur)) }}
  {{- if $siblings }}
  <ul class="siblings">
    {{- range $siblings }}{{- if not (hasTag . "noindex") }}
    <li{{if eq .URLPath $cur}} class="current"{{end}}>
      <a href="{{ .URLPath }}">{{ or .Meta.Title .Meta.Slug }}</a>
    </li>
    {{- end }}{{- end }}
  </ul>
  {{- end }}

  {{- $children := sortByPath (.Site.ChildrenOf $cur) }}
  {{- if $children }}
  <ul class="children">
    {{- range $children }}{{- if not (hasTag . "noindex") }}
    <li><a href="{{ .URLPath }}">{{ or .Meta.Title .Meta.Slug }}</a></li>
    {{- end }}{{- end }}
  </ul>
  {{- end }}
</nav>
{{ end }}
```

### Injecting JSON-LD and passing data to JavaScript

```html
{{ define "article" }}
<article>
  {{- if .JSONLD }}
  <script type="application/ld+json">{{ .JSONLD }}</script>
  {{- end }}
  {{ .Content }}
</article>
{{ end }}

{{ define "page" }}
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
  {{ .Content }}
</body>
</html>
{{ end }}
```
