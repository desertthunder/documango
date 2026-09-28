// Package frontmatter splits YAML front matter from Markdown documents.
package frontmatter

import (
	"bytes"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Meta is the YAML front matter recognized by documango. Unknown keys are kept in Extra.
type Meta struct {
	Title       string         `yaml:"title"`
	Description string         `yaml:"description"`
	Order       int            `yaml:"order"` // sort key within a nav section, ascending; ties broken by title
	Draft       bool           `yaml:"draft"` // drafts are excluded from the site
	Extra       map[string]any `yaml:",inline"`
}

var bom = []byte("\xef\xbb\xbf")

// Parse splits src into its front matter and body.
//
// Front matter is recognized only when the document starts (after an optional
// UTF-8 BOM) with a line that is exactly "---", and ends at the next line that
// is "---" or "...". Without front matter Parse returns a zero Meta and the
// document with any BOM removed.
func Parse(src []byte) (Meta, []byte, error) {
	src = bytes.TrimPrefix(src, bom)

	first, rest, ok := cutLine(src)
	if !ok || !isDelimiter(first, false) {
		return Meta{}, src, nil
	}

	var yamlEnd int
	body := rest
	for {
		line, next, ok := cutLine(body)
		if !ok {
			return Meta{}, nil, errors.New("unclosed front matter: missing closing \"---\"")
		}
		if isDelimiter(line, true) {
			yamlEnd = len(rest) - len(body)
			body = next
			break
		}
		body = next
	}

	var meta Meta
	if err := yaml.Unmarshal(rest[:yamlEnd], &meta); err != nil {
		return Meta{}, nil, fmt.Errorf("invalid front matter: %w", err)
	}
	return meta, body, nil
}

// cutLine returns the first line of b without its line ending and the
// remainder after it. ok is false when b is empty.
func cutLine(b []byte) (line, rest []byte, ok bool) {
	if len(b) == 0 {
		return nil, nil, false
	}
	line, rest, _ = bytes.Cut(b, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r")), rest, true
}

func isDelimiter(line []byte, closing bool) bool {
	line = bytes.TrimRight(line, " \t")
	return string(line) == "---" || (closing && string(line) == "...")
}
