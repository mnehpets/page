package page

import (
	"strings"
	"testing"
	"time"
)

func TestParseFrontMatter_WithFrontMatter(t *testing.T) {
	src := "---\ntitle: Hello World\nauthor: Alice\ndraft: true\ntags:\n  - go\n  - web\n---\nBody content here.\n"

	meta, body, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Title != "Hello World" {
		t.Errorf("Title = %q, want %q", meta.Title, "Hello World")
	}
	if meta.Author != "Alice" {
		t.Errorf("Author = %q, want %q", meta.Author, "Alice")
	}
	if !meta.Draft {
		t.Error("Draft should be true")
	}
	if len(meta.Tags) != 2 || meta.Tags[0] != "go" || meta.Tags[1] != "web" {
		t.Errorf("Tags = %v, want [go web]", meta.Tags)
	}
	if string(body) != "Body content here.\n" {
		t.Errorf("body = %q, want %q", string(body), "Body content here.\n")
	}
}

func TestParseFrontMatter_NoFrontMatter(t *testing.T) {
	src := "Just a plain markdown file.\nNo front matter here.\n"

	meta, body, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Title != "" || meta.Author != "" || meta.Draft {
		t.Errorf("Meta should be zero-valued, got %+v", meta)
	}
	if string(body) != src {
		t.Errorf("body = %q, want original content %q", string(body), src)
	}
}

func TestParseFrontMatter_MalformedYAML(t *testing.T) {
	// Valid delimiters but the YAML inside is structurally invalid.
	src := "---\ntitle: [unclosed bracket\n---\nBody.\n"

	_, _, err := ParseFrontMatter(strings.NewReader(src))
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

func TestParseFrontMatter_ImageBareString(t *testing.T) {
	src := "---\nimage: https://example.com/img.png\n---\n"
	meta, _, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Image.URL != "https://example.com/img.png" {
		t.Errorf("Image.URL = %q", meta.Image.URL)
	}
	if meta.Image.Alt != "" {
		t.Errorf("Image.Alt should be empty, got %q", meta.Image.Alt)
	}
}

func TestParseFrontMatter_ImageMapping(t *testing.T) {
	src := "---\nimage:\n  url: https://example.com/img.png\n  alt: A description\n---\n"
	meta, _, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Image.URL != "https://example.com/img.png" {
		t.Errorf("Image.URL = %q", meta.Image.URL)
	}
	if meta.Image.Alt != "A description" {
		t.Errorf("Image.Alt = %q, want %q", meta.Image.Alt, "A description")
	}
}

func TestParseFrontMatter_LinkTitle(t *testing.T) {
	src := "---\ntitle: A Very Long Page Title\nlinkTitle: Short Title\n---\n"
	meta, _, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Title != "A Very Long Page Title" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.LinkTitle != "Short Title" {
		t.Errorf("LinkTitle = %q, want %q", meta.LinkTitle, "Short Title")
	}
}

func TestParseFrontMatter_LastMod(t *testing.T) {
	src := "---\ndate: 2024-01-01\nlastmod: 2024-06-15\n---\n"
	meta, _, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantDate, _ := time.Parse("2006-01-02", "2024-01-01")
	wantLastMod, _ := time.Parse("2006-01-02", "2024-06-15")
	if !meta.Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", meta.Date, wantDate)
	}
	if !meta.LastMod.Equal(wantLastMod) {
		t.Errorf("LastMod = %v, want %v", meta.LastMod, wantLastMod)
	}
}

func TestParseFrontMatter_LastModAbsent(t *testing.T) {
	src := "---\ndate: 2024-01-01\n---\n"
	meta, _, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.LastMod.IsZero() {
		t.Errorf("LastMod should be zero when not set, got %v", meta.LastMod)
	}
}

func TestParseFrontMatter_NoClosingDelimiter(t *testing.T) {
	// Opening delimiter without closing — treated as no front matter.
	src := "---\ntitle: Orphan\nBody without closing delimiter.\n"
	meta, body, err := ParseFrontMatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Title != "" {
		t.Errorf("Title should be empty when no closing delimiter, got %q", meta.Title)
	}
	if string(body) != src {
		t.Errorf("body should be original content when no closing delimiter")
	}
}
