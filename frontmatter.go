package page

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	gmeta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ParseFrontMatter reads r, strips any ---‑delimited YAML front matter, and
// returns the parsed Meta and the remaining body bytes. If no front matter is
// present, Meta is zero‑valued and the full content is returned as body.
// Malformed YAML front matter returns a non‑nil error.
func ParseFrontMatter(r io.Reader) (Meta, []byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Meta{}, nil, err
	}

	// Only attempt YAML parsing when a proper ---...--- block is present.
	// Without a closing delimiter goldmark-meta errors on non-YAML body lines.
	if !hasFrontMatterBlock(data) {
		return Meta{}, data, nil
	}

	// Parse-only pass: goldmark-meta detects and parses the YAML block,
	// storing results (including any parse error) in the context.
	md := goldmark.New(goldmark.WithExtensions(gmeta.Meta))
	ctx := parser.NewContext()
	md.Parser().Parse(text.NewReader(data), parser.WithContext(ctx))

	metaMap, err := gmeta.TryGet(ctx)
	if err != nil {
		return Meta{}, nil, fmt.Errorf("page: front matter: %w", err)
	}

	return mapToMeta(metaMap), stripFrontMatterBytes(data), nil
}

// hasFrontMatterBlock reports whether data contains a complete ---...--- block.
func hasFrontMatterBlock(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return false
	}
	rest := data[4:]
	return bytes.Contains(rest, []byte("\n---\n")) || bytes.HasSuffix(rest, []byte("\n---"))
}

// stripFrontMatterBytes returns the body content after stripping the
// ---‑delimited front matter block. If no front matter is present, data is
// returned unchanged.
func stripFrontMatterBytes(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return data
	}
	rest := data[4:]
	if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
		return rest[idx+5:]
	}
	if bytes.HasSuffix(rest, []byte("\n---")) {
		return nil
	}
	return data
}

// mapToMeta converts the raw map returned by goldmark-meta into a Meta struct.
// goldmark-meta uses yaml.v2 internally, so nested mappings are
// map[interface{}]interface{}, not map[string]interface{}.
func mapToMeta(m map[string]interface{}) Meta {
	if m == nil {
		return Meta{}
	}
	var meta Meta
	meta.Title, _ = m["title"].(string)
	meta.Author, _ = m["author"].(string)
	meta.Collection, _ = m["collection"].(string)
	meta.Draft, _ = m["draft"].(bool)
	meta.Description, _ = m["description"].(string)
	meta.Slug, _ = m["slug"].(string)
	meta.LinkTitle, _ = m["linkTitle"].(string)
	meta.ContentType, _ = m["contentType"].(string)

	// layout: "name"  or  layouts: [a, b]
	if s, ok := m["layout"].(string); ok && s != "" {
		meta.Layouts = []string{s}
	}
	if seq, ok := m["layouts"].([]interface{}); ok {
		for _, v := range seq {
			if s, ok := v.(string); ok && s != "" {
				meta.Layouts = append(meta.Layouts, s)
			}
		}
	}

	// Tags: yaml.v2 unmarshals a YAML sequence as []interface{}.
	if tags, ok := m["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				meta.Tags = append(meta.Tags, s)
			}
		}
	}

	// Date / LastMod: yaml.v2 returns time.Time for YAML timestamps; fall back to string.
	parseYAMLDate := func(v interface{}) time.Time {
		switch d := v.(type) {
		case time.Time:
			return d
		case string:
			for _, layout := range []string{time.RFC3339, "2006-01-02"} {
				if t, err := parseTime(layout, d); err == nil {
					return t
				}
			}
		}
		return time.Time{}
	}
	meta.Date = parseYAMLDate(m["date"])
	meta.LastMod = parseYAMLDate(m["lastmod"])

	// Image: bare string or {url, alt} mapping.
	// yaml.v2 uses map[interface{}]interface{} for nested mappings.
	switch img := m["image"].(type) {
	case string:
		meta.Image = Image{URL: img}
	case map[interface{}]interface{}:
		url, _ := img["url"].(string)
		alt, _ := img["alt"].(string)
		meta.Image = Image{URL: url, Alt: alt}
	case map[string]interface{}:
		url, _ := img["url"].(string)
		alt, _ := img["alt"].(string)
		meta.Image = Image{URL: url, Alt: alt}
	}

	return meta
}

func parseTime(layout, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	return time.Parse(layout, s)
}
