package page

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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

	yamlBlock, body, ok := extractFrontMatterBlock(data)
	if !ok {
		return Meta{}, data, nil
	}

	var raw rawFrontMatter
	if err := yaml.Unmarshal(yamlBlock, &raw); err != nil {
		return Meta{}, nil, fmt.Errorf("page: front matter: %w", err)
	}

	var layouts []string
	if raw.Layout != "" {
		layouts = append(layouts, raw.Layout)
	}
	layouts = append(layouts, raw.Layouts...)

	meta := Meta{
		Title:       raw.Title,
		Author:      raw.Author,
		Date:        raw.Date.Time,
		LastMod:     raw.LastMod.Time,
		Tags:        raw.Tags,
		Collection:  raw.Collection,
		Layouts:     layouts,
		Draft:       raw.Draft,
		Description: raw.Description,
		Image:       Image{URL: raw.Image.URL, Alt: raw.Image.Alt},
		Slug:        raw.Slug,
		LinkTitle:   raw.LinkTitle,
		ContentType: raw.ContentType,
	}

	return meta, body, nil
}

// extractFrontMatterBlock splits data into the raw YAML bytes inside the
// ---…--- block and the body that follows. If no complete block is present,
// ok is false and body is the full data slice.
func extractFrontMatterBlock(data []byte) (yamlBlock, body []byte, ok bool) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, data, false
	}
	rest := data[4:]
	if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
		return rest[:idx], rest[idx+5:], true
	}
	if bytes.HasSuffix(rest, []byte("\n---")) {
		return rest[:len(rest)-4], nil, true
	}
	return nil, data, false
}

// rawFrontMatter is the direct yaml.v3 unmarshal target for front-matter blocks.
type rawFrontMatter struct {
	Title       string   `yaml:"title"`
	Author      string   `yaml:"author"`
	Date        yamlTime `yaml:"date"`
	LastMod     yamlTime `yaml:"lastmod"`
	Tags        []string `yaml:"tags"`
	Collection  string   `yaml:"collection"`
	Layout      string   `yaml:"layout"`
	Layouts     []string `yaml:"layouts"`
	Draft       bool     `yaml:"draft"`
	Description string   `yaml:"description"`
	Image       rawImage `yaml:"image"`
	Slug        string   `yaml:"slug"`
	LinkTitle   string   `yaml:"linkTitle"`
	ContentType string   `yaml:"contentType"`
}

// yamlTime unmarshals a YAML value as time.Time, accepting both native YAML
// timestamps and quoted date strings (RFC3339 or 2006-01-02).
type yamlTime struct {
	time.Time
}

func (t *yamlTime) UnmarshalYAML(value *yaml.Node) error {
	if err := value.Decode(&t.Time); err == nil {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value.Value)); err == nil {
			t.Time = parsed
			return nil
		}
	}
	return nil // leave as zero time if unparseable
}

// rawImage unmarshals a YAML value that may be either a bare URL string or
// an {url, alt} mapping.
type rawImage struct {
	URL string
	Alt string
}

func (i *rawImage) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		i.URL = value.Value
		return nil
	}
	type imageFields struct {
		URL string `yaml:"url"`
		Alt string `yaml:"alt"`
	}
	var f imageFields
	if err := value.Decode(&f); err != nil {
		return err
	}
	i.URL = f.URL
	i.Alt = f.Alt
	return nil
}
