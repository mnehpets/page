# Frontmatter and metadata

Page metadata drives the site index, the template `.Meta` value, and the
schema.org JSON-LD blob (`.JSONLD`) that is generated automatically for each
page. This document describes how to declare metadata in both markdown and HTML
source files.

## Markdown frontmatter

Markdown files declare metadata as YAML between `---` delimiters at the top of
the file.

```markdown
---
title: Hello, world
author: Alice
date: 2024-06-01
description: A short introduction.
tags: [go, web]
collection: posts
layout: post
draft: false
image:
  url: /images/hello.png
  alt: A friendly greeting
---

Page body starts here.
```

### Supported fields

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Page title. |
| `author` | string | Author name. |
| `date` | date | Publication date. Accepts `2006-01-02` or RFC 3339. Falls back to the file modification time. |
| `lastmod` | date | Last-modified date. Accepts `2006-01-02` or RFC 3339. Always falls back to the file modification time if not set. |
| `description` | string | Short description. |
| `tags` | list of strings | Tags for querying via `.Site.ByTag`. |
| `collection` | string | Collection name for `.Site.ByCollection`. |
| `layout` | string | Name of the layout template to use. Defaults to `default`. |
| `draft` | bool | If `true`, the page is excluded from `.Site.All`, `.Site.ByTag`, and `.Site.ByCollection`. |
| `slug` | string | URL-friendly identifier. Derived from the file name if omitted. |
| `linkTitle` | string | Short title for navigation links (breadcrumbs, sidebars). Falls back to `title` when empty. |
| `contentType` | string | MIME type for the HTTP response (e.g. `application/xml`). Defaults to `text/html`. |
| `image` | string or mapping | Representative image. Bare string sets the URL; mapping accepts `url` and `alt` keys. |

---

## HTML metadata

HTML pages declare metadata through two mechanisms, checked in priority order:

1. **JSON-LD** (`<script type="application/ld+json">`) — highest priority.
2. **`<meta>` tags and `<title>`** — used for any field not already set by JSON-LD.

The file modification time is used as a fallback for `date` when neither source provides one.

### JSON-LD fields

The JSON-LD block uses standard schema.org vocabulary at the top level. Site-specific
fields that have no schema.org equivalent go inside a `"site"` key, which is
stripped before the blob is passed to templates (so it never leaks to end users).

```html
<script type="application/ld+json">
{
  "@context": "https://schema.org/",
  "name": "Hello, world",
  "author": "Alice",
  "datePublished": "2024-06-01",
  "description": "A short introduction.",
  "keywords": ["go", "web"],
  "image": {
    "url": "/images/hello.png",
    "alt": "A friendly greeting"
  },
  "site": {
    "layout": "post",
    "collection": "posts",
    "draft": false,
    "slug": "hello-world",
    "linkTitle": "Hello",
    "contentType": "text/html"
  }
}
</script>
```

#### Top-level JSON-LD fields

| JSON-LD key | Maps to | Equivalent HTML meta |
|-------------|---------|----------------------|
| `name` | `Meta.Title` | `<meta name="title">`, `<meta property="og:title">`, or `<title>` |
| `author` | `Meta.Author` | `<meta name="author">` |
| `description` | `Meta.Description` | `<meta name="description">`, `<meta property="og:description">` |
| `keywords` | `Meta.Tags` | `<meta name="keywords">` (comma-separated string or JSON array) |
| `datePublished` | `Meta.Date` | — |
| `dateModified` | `Meta.LastMod`; also `Meta.Date` if `datePublished` is absent | — |
| `image` | `Meta.Image` | `<meta property="og:image">`, `<meta property="og:image:alt">` |

#### `"site"` namespace fields (stripped from the public JSON-LD blob)

| Key | Maps to | Description |
|-----|---------|-------------|
| `layout` | `Meta.Layout` | Layout template name. |
| `collection` | `Meta.Collection` | Collection name. |
| `slug` | `Meta.Slug` | URL-friendly identifier. |
| `draft` | `Meta.Draft` | Exclude from site indexes when `true`. |
| `linkTitle` | `Meta.LinkTitle` | Short title for navigation links. Falls back to `Meta.Title` when empty. |
| `contentType` | `Meta.ContentType` | MIME type for the HTTP response (e.g. `application/xml`). Defaults to `text/html`. |

### Recognised `<meta>` tags

These tags are read when the equivalent JSON-LD field is absent. They are also
removed from `.Head` so the layout template is not responsible for deduplicating
them.

| Tag | Attribute | Maps to |
|-----|-----------|---------|
| `<meta name="title" content="...">` | `name` | `Meta.Title` |
| `<meta property="og:title" content="...">` | `property` | `Meta.Title` |
| `<meta name="author" content="...">` | `name` | `Meta.Author` |
| `<meta name="description" content="...">` | `name` | `Meta.Description` |
| `<meta property="og:description" content="...">` | `property` | `Meta.Description` |
| `<meta name="keywords" content="go, web">` | `name` | `Meta.Tags` (comma-split) |
| `<meta property="og:image" content="...">` | `property` | `Meta.Image.URL` |
| `<meta property="og:image:alt" content="...">` | `property` | `Meta.Image.Alt` |
| `<title>Hello, world</title>` | — | `Meta.Title` (lowest priority) |

---

## Examples

### Markdown article

```markdown
---
title: Building a static site in Go
author: Bob
date: 2024-09-15
description: A walkthrough of the page package.
tags: [go, tutorial]
collection: posts
layout: post
image: /images/static-site.png
---

## Introduction

The `page` package turns a directory of Markdown and HTML files into a
navigable site...
```

### HTML page with JSON-LD and meta fallbacks

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">

  <!-- JSON-LD: highest priority, provides most fields -->
  <script type="application/ld+json">
  {
    "@context": "https://schema.org/",
    "name": "About this site",
    "author": "Carol",
    "datePublished": "2024-01-10",
    "description": "Background and colophon.",
    "site": {
      "layout": "page",
      "collection": "meta"
    }
  }
  </script>

  <!-- og:image has no JSON-LD equivalent, so set it here -->
  <meta property="og:image" content="/images/about.png">
  <meta property="og:image:alt" content="Site logo">

  <!-- Any other head elements not captured above pass through to .Head -->
  <link rel="canonical" href="https://example.com/about/">
</head>
<body>
  <p>This site is built with the <code>page</code> package.</p>
</body>
</html>
```

The `<link rel="canonical">` element is not captured by the metadata parser, so
it passes through in `.Head` and the layout template can render it as-is.
