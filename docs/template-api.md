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
| `.Meta.Layouts` | `[]string` | The layout pipeline for this page. Rarely needed inside a template. |
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
      <a href="{{ $.Href . }}">{{ .Meta.Title }}</a>
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
{{- if not (hasTag .Meta "noindex") }}
  <url>
    <loc>{{ $.AbsURL . }}</loc>
  </url>
{{- end }}
{{- end }}
</urlset>
{{ end }}
```

The sitemap page would declare `contentType: application/xml` so the response
is served with the correct MIME type, and `layout: sitemap` to select the template above.

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
