package page_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mnehpets/http/endpoint"
	"github.com/mnehpets/page"
)

var integrationFS = fstest.MapFS{
	"blog/hello.md": {Data: []byte("---\ntitle: Hello\n---\n# Hello from Markdown\n")},
	"about.html": {Data: []byte(`<!DOCTYPE html><html>
<head><title>About</title></head>
<body><p>About page content.</p></body>
</html>`)},
	"style.css":     {Data: []byte("body { margin: 0; }")},
	"blog/index.md": {Data: []byte("---\ntitle: Blog Index\n---\nBlog index content.\n")},
	"_layouts/default.html": {Data: []byte(
		`{{define "default"}}<!DOCTYPE html><html><body>{{.Content}}</body></html>{{end}}`,
	)},
}

func newIntegrationServer(t *testing.T) *httptest.Server {
	t.Helper()

	site, err := page.NewSite(integrationFS)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	fsHandler := &endpoint.FileSystem{
		FS: func(_ context.Context, _ *http.Request) (fs.FS, error) {
			return integrationFS, nil
		},
		FileRenderer: site.FileRenderer(),
		DirRenderer:  site.DirRenderer(),
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := endpoint.FileSystemParams{Path: strings.TrimPrefix(r.URL.Path, "/")}
		renderer, err := fsHandler.Endpoint(w, r, params)
		if err != nil {
			var ee *endpoint.EndpointError
			status := http.StatusInternalServerError
			if errors.As(err, &ee) {
				status = ee.Status
			}
			http.Error(w, err.Error(), status)
			return
		}
		if renderer == nil {
			http.NotFound(w, r)
			return
		}
		if err := renderer.Render(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func TestIntegration_MarkdownRequestReturnsRenderedHTML(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/blog/hello.md")
	if err != nil {
		t.Fatalf("GET /blog/hello.md: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if body := readBody(t, resp); !strings.Contains(body, "Hello from Markdown") {
		t.Errorf("response missing markdown content: %q", body)
	}
}

func TestIntegration_HTMLRequestReturnsRenderedHTML(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/about.html")
	if err != nil {
		t.Fatalf("GET /about.html: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "About page content") {
		t.Errorf("response missing html content: %q", body)
	}
}

func TestIntegration_StaticAssetReturnsFileBytes(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/style.css")
	if err != nil {
		t.Fatalf("GET /style.css: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "margin") {
		t.Errorf("response missing CSS content: %q", body)
	}
}

func TestIntegration_DirectoryRequestReturnsIndexPage(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/blog/")
	if err != nil {
		t.Fatalf("GET /blog/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "Blog index content") {
		t.Errorf("response missing blog index content: %q", body)
	}
}

func TestIntegration_UnknownPathReturns404(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/does-not-exist.md")
	if err != nil {
		t.Fatalf("GET /does-not-exist.md: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
