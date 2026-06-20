# pageserve config.yaml schema

`config.yaml` drives the entire server. It is safe to commit — secrets are never
stored inline; they are referenced by environment variable name and resolved at
startup from a separate `secrets.env` file (or any other source the caller provides).

---

## Top-level sections

| Section | Required | Description |
|---|---|---|
| `server` | yes | Network listener configuration |
| `oauth` | yes (if auth routes are configured) | OAuth provider list |
| `session` | yes (if access policies or auth routes are configured) | Session cookie configuration |
| `access` | no | Named email allow-list policies |
| `csp` | no | Content-Security-Policy directive extensions |
| `cors` | no | Cross-origin resource sharing settings |
| `cross_origin` | no | Cross-origin response header overrides |
| `site` | no | Site-level rendering configuration |
| `routes` | yes | Flat list of route entries |

---

## `server`

```yaml
server:
  listeners:
    - address: ":8080"            # plain HTTP
    - address: ":8443"            # HTTPS
      tls:
        cert_file: /path/to/cert.pem
        key_file:  /path/to/key.pem
```

At least one listener is required. Multiple listeners can be declared to serve
HTTP and HTTPS simultaneously, or on several ports at once. Each listener runs
in its own goroutine sharing the same handler.

| Field | Type | Required | Description |
|---|---|---|---|
| `listeners` | list | yes | One or more listener entries |
| `listeners[].address` | string | yes | TCP listen address (e.g. `":8080"`, `"0.0.0.0:443"`) |
| `listeners[].tls` | mapping | no | Omit for plain HTTP; set to enable HTTPS |
| `listeners[].tls.cert_file` | string | yes (if tls) | Path to PEM-encoded TLS certificate |
| `listeners[].tls.key_file` | string | yes (if tls) | Path to PEM-encoded private key |

---

## `oauth`

```yaml
oauth:
  providers:
    - provider: google                                       # required
      client_id: 1234567890-abc.apps.googleusercontent.com  # required
      client_secret:
        env: OAUTH_CLIENT_SECRET                            # required — env var name
```

| Field | Type | Required | Description |
|---|---|---|---|
| `providers` | list | yes | List of OAuth provider entries |
| `providers[].provider` | enum | yes | Provider type. Valid value: `google` |
| `providers[].client_id` | string | yes | OAuth client ID (not a secret; safe to commit) |
| `providers[].client_secret.env` | string | yes | Name of env var holding the client secret |

Only `google` is currently supported. The `providers` list is structured for future
extensibility.

---

## `session`

```yaml
session:
  cookie_name: myapp_session   # optional — omit to use the middleware default
  keys:
    - id: key-2025             # required — opaque identifier for key rotation
      env: SESSION_KEY_2025    # required — env var name holding the signing key
    - id: key-2024             # optional additional keys for rotation
      env: SESSION_KEY_2024
```

| Field | Type | Required | Description |
|---|---|---|---|
| `cookie_name` | string | no | Name of the session cookie. Omit to use the middleware default |
| `keys` | list | yes (if session section is present) | At least one signing key entry |
| `keys[].id` | string | yes | Opaque key identifier used for rotation |
| `keys[].env` | string | yes | Name of env var holding the key bytes (base64-encoded) |

**Key encoding:** the env var value must be the raw key bytes encoded as standard
base64. Generate a key with:

```shell
openssl rand 32 | base64
```

**Key rotation:** add a new key at the front of the list and retain the old key.
Sessions signed with old keys remain valid until they expire; remove old keys
only after all sessions using them have expired.

---

## `access`

```yaml
access:
  admin:
    allow: ["alice@example.com"]
  members:
    allow: ["*@mycompany.com", "partner@external.com"]
```

| Field | Type | Required | Description |
|---|---|---|---|
| `<name>` | mapping | — | Policy name; referenced by `routes[].access` |
| `<name>.allow` | list of strings | yes | Email glob patterns. `*` matches any character except `@` |

**Pattern rules:** `@` is treated as a separator — wildcards do not cross it.
`*@example.com` matches all emails at `example.com`. Patterns are case-sensitive.

Routes that reference an undefined policy name fail validation at startup.

---

## `csp`

Extends the default Content-Security-Policy with additional directives. Omit this
section to use the default policy unchanged.

The default CSP is:
```
default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; upgrade-insecure-requests
```

Each entry in `extra_directives` is a complete CSP directive that is appended to
the default. Existing default directives are not modified.

```yaml
csp:
  extra_directives:
    - "script-src 'self' 'unsafe-inline' 'unsafe-eval'"
    - "script-src-elem 'self' https://unpkg.com 'unsafe-inline'"
    - "style-src 'self' 'unsafe-inline'"
```

| Field | Type | Required | Description |
|---|---|---|---|
| `extra_directives` | list of strings | yes | Complete CSP directives appended to the defaults |

**Note on `unsafe-inline` and `unsafe-eval`:** these weaken script security. Use them
only when loading third-party libraries (such as Tailwind CDN or Vue from unpkg.com)
that require them, and scope them as narrowly as possible via `script-src-elem`.

---

## `cors`

Global CORS settings applied to all routes. Omit this section entirely to disable
CORS headers.

```yaml
cors:
  allowed_origins: ["https://app.example.com"]
  allowed_methods: ["GET", "POST"]
  allowed_headers: ["Content-Type", "Authorization"]
  exposed_headers: []
  allow_credentials: true
  max_age: 3600
```

| Field | Type | Default | Description |
|---|---|---|---|
| `allowed_origins` | list of strings | — | Origins permitted to make cross-origin requests |
| `allowed_methods` | list of strings | — | HTTP methods allowed in CORS requests |
| `allowed_headers` | list of strings | — | Request headers allowed in CORS requests |
| `exposed_headers` | list of strings | — | Response headers the browser may expose |
| `allow_credentials` | bool | `false` | Whether cookies/auth headers are allowed |
| `max_age` | int | `0` | Preflight cache duration in seconds |

---

## `cross_origin`

Overrides the three cross-origin response headers set on every response.
Omit this section to use the defaults shown below.

```yaml
cross_origin:
  coop: same-origin-allow-popups   # Cross-Origin-Opener-Policy
  coep: unsafe-none                # Cross-Origin-Embedder-Policy
  corp: same-origin                # Cross-Origin-Resource-Policy
```

| Field | Type | Default | Description |
|---|---|---|---|
| `coop` | string | `"same-origin-allow-popups"` | Cross-Origin-Opener-Policy. Default permits OAuth popup flows |
| `coep` | string | `"unsafe-none"` | Cross-Origin-Embedder-Policy. Default permits third-party embeds (YouTube, maps, etc.) |
| `corp` | string | `"same-origin"` | Cross-Origin-Resource-Policy |

Only fields that are set override the default; omitted fields keep their defaults.

---

## `site`

```yaml
site:
  base_url: https://example.com   # no trailing slash
  name: My Site                   # optional — available to page templates as .Config.Name
  lang: en                        # optional — BCP 47 language tag (default: "en")
```

| Field | Type | Required | Description |
|---|---|---|---|
| `base_url` | string | no | Canonical base URL. Used by OAuth callback construction and page templates |
| `name` | string | no | Human-readable site name, available in templates |
| `lang` | string | no | BCP 47 language tag (default `"en"`) |

---

## `routes`

A flat list of route entries. Each entry has at minimum a `path` and `handler`.

```yaml
routes:
  - path: /                  # http.ServeMux pattern
    handler: pages           # handler type
    access: members          # optional — policy name from access:
```

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | `http.ServeMux` pattern (e.g. `/`, `GET /api/`, `/assets/`) |
| `handler` | string | yes | Handler type name (see below) |
| `access` | string | no | Named policy from `access:`. Omit for public routes |

Routes with no `access` field are public and require no session.

### Handler types

#### `pages`

Serves content via the `page` FS library from a local directory.

```yaml
- path: /
  handler: pages
  dir: ./content          # optional — source directory (default ".")
  include_drafts: false   # optional — include draft pages (default false)
  dir_list: false         # optional — serve directory listings (default false)
  dotfiles: false         # optional — serve/list dotfiles (default false)
  symlinks: false         # optional — follow symlinks (default false)
  watch: false            # optional — watch for file changes and refresh index (default false)
```

| Field | Type | Default | Description |
|---|---|---|---|
| `dir` | string | `"."` | Source directory for page content |
| `include_drafts` | bool | `false` | Whether draft pages are served |
| `dir_list` | bool | `false` | Serve HTML directory listings |
| `dotfiles` | bool | `false` | Serve and list dotfiles |
| `symlinks` | bool | `false` | Follow symlinks |
| `watch` | bool | `false` | Watch `dir` for file changes and refresh the page index without restarting |

`dotfiles` and `symlinks` affect both serving and listing.

**`watch` behaviour:** when enabled, a filesystem watcher (fsnotify) monitors `dir` and updates the
in-memory page index automatically. File creates and writes re-parse the affected file; removes and
renames drop it (with directory-index fallback). Events are debounced at 400 ms to coalesce editor
save sequences. The count of refreshed files is exposed in expvar at
`pageserve.route.<name>.refreshed_files`, visible via the `defaultmux` handler at `/debug/vars`.

#### `files`

Serves a directory tree as raw files via `http.FileServer` semantics.

```yaml
- path: /assets/
  handler: files
  dir: ./static           # required — source directory
  dir_list: false         # optional — serve directory listings (default false)
  index_html: true        # optional — serve index.html for directories (default true)
  dotfiles: false         # optional — serve/list dotfiles (default false)
  symlinks: false         # optional — follow symlinks (default false)
  webdav: false           # optional — also serve read-only WebDAV (default false)
```

| Field | Type | Default | Description |
|---|---|---|---|
| `dir` | string | — | **Required.** Source directory |
| `dir_list` | bool | `false` | Serve HTML directory listings |
| `index_html` | bool | `true` | Serve `index.html` for directory requests |
| `dotfiles` | bool | `false` | Serve and list dotfiles |
| `symlinks` | bool | `false` | Follow symlinks |
| `webdav` | bool | `false` | Also answer WebDAV `PROPFIND`/`OPTIONS` (read-only) |

`dotfiles` and `symlinks` affect both serving and listing.

**`webdav` behaviour:** when enabled, the route additionally answers WebDAV
`PROPFIND` and `OPTIONS` alongside `GET`/`HEAD`, so read-only WebDAV clients can
mount and browse the tree. `OPTIONS` advertises class-1 compliance (`DAV: 1`).
`PROPFIND` responses are `207 Multi-Status` XML reporting only the resource type
(file vs collection), size (`getcontentlength`), content type (`getcontenttype`),
and last-modified time (`getlastmodified`). The `Depth` header is honoured for
`0` (the resource itself) and `1` (the resource plus its immediate children); an
absent `Depth` is treated as `1`, and `Depth: infinity` is rejected with `403`
rather than walking the whole tree. The `dotfiles`/`symlinks` filter applies to
`PROPFIND` child listings too, so a resource hidden from `GET` is also hidden
from `PROPFIND`. Write methods (`PUT`, `DELETE`, `MKCOL`, `PROPPATCH`, `LOCK`)
are not implemented.

#### `redirect`

Issues an HTTP redirect.

```yaml
- path: /old/
  handler: redirect
  to: /new/              # required — destination base URL
  code: 301              # optional — HTTP status code (default 302)
  preserve_path: true    # optional — append sub-path to to (default true)
```

| Field | Type | Default | Description |
|---|---|---|---|
| `to` | string | — | **Required.** Redirect destination |
| `code` | int | `302` | HTTP status code |
| `preserve_path` | bool | `true` | Append the matched sub-path to `to` (tree routes only) |

**Path behaviour:** when `path` ends with `/` (a tree route), `preserve_path` controls
whether the sub-path is carried over to the destination:

- `preserve_path: true` (default) — `/old/foo/bar` → `/new/foo/bar`
- `preserve_path: false` — all requests under `/old/` redirect to the same fixed `to`

For exact-match routes (no trailing slash, or ending with `{$}`), `preserve_path`
has no effect — the route matches a single path and there is no sub-path to carry
over. This includes patterns like `/{$}` which anchors an exact match on `/`.

#### `proxy`

Reverse-proxies requests to a destination URL. The route path prefix is stripped
before forwarding (e.g. `/gh/repos/foo` → `/repos/foo` for `to: https://api.github.com`).
Client headers including `Authorization` are forwarded unchanged.

```yaml
- path: /gh/
  handler: proxy
  to: https://api.github.com   # required — absolute destination URL
  access: members              # optional — protect with a policy
```

| Field | Type | Required | Description |
|---|---|---|---|
| `to` | string | yes | Absolute destination URL |

#### `auth`

Registers the OAuth login, callback, and logout sub-paths under the declared base
path. No handler-specific config fields are required.

```yaml
- path: /auth/
  handler: auth
```

Sub-paths managed automatically:
- `<path>/login/{provider}` — initiates OAuth flow
- `<path>/callback/{provider}` — OAuth callback
- `POST <path>/logout` — clears the session and redirects to `/`

**Logout requires POST.** The logout endpoint only accepts `POST` requests to
prevent CSRF logout attacks (e.g. via `<img>` tags or navigations). Clients must
submit a form or use `fetch` with `method: "POST"`.

The callback URL is derived from `site.base_url` and the registered path.
No explicit callback URL is declared in the config.

#### `defaultmux`

Exposes Go's default `http.ServeMux`, which includes `/debug/vars` (expvar) and
`/debug/pprof` (runtime profiling) when the relevant packages are imported.

```yaml
- path: /debug/
  handler: defaultmux
  access: admin   # recommended — restrict to trusted users
```

No handler-specific fields.

---

## Full example

```yaml
server:
  listeners:
    - address: ":8080"

oauth:
  providers:
    - provider: google
      client_id: 1234567890-abc.apps.googleusercontent.com
      client_secret:
        env: OAUTH_CLIENT_SECRET

session:
  # cookie_name: myapp_session   # optional — omit to use the middleware default
  keys:
    - id: key-2025
      env: SESSION_KEY_2025

access:
  admin:
    allow: ["alice@example.com"]
  members:
    allow: ["*@mycompany.com"]

site:
  base_url: https://example.com
  name: My Site
  lang: en

# csp is optional; omit to use the default policy.
# csp:
#   extra_directives:
#     - "script-src 'self' 'unsafe-inline' 'unsafe-eval'"
#     - "script-src-elem 'self' https://unpkg.com 'unsafe-inline'"
#     - "style-src 'self' 'unsafe-inline'"

# cross_origin is optional; omit to use the defaults.
# cross_origin:
#   coop: same-origin-allow-popups
#   coep: require-corp             # stricter — disallows third-party embeds
#   corp: same-origin

routes:
  - path: /auth/
    handler: auth

  - path: /debug/
    handler: defaultmux
    access: admin

  - path: /assets/
    handler: files
    dir: ./static

  - path: /admin/
    handler: pages
    dir: ./content/admin
    access: admin

  - path: /
    handler: pages
    dir: ./content
    access: members
```

```shell
# secrets.env  (gitignored)
OAUTH_CLIENT_SECRET=supersecretvalue
SESSION_KEY_2025=<base64-encoded 32 bytes, e.g. output of: openssl rand 32 | base64>
```
