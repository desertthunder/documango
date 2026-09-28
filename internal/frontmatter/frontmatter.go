// Package frontmatter splits YAML or TOML front matter from Markdown documents.
package frontmatter

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"
)

// Meta is the front matter recognized by documango. Unknown keys are kept in Extra.
type Meta struct {
	Title       string         `yaml:"title" toml:"title"`
	Description string         `yaml:"description" toml:"description"`
	Order       int            `yaml:"order" toml:"order"` // sort key within a nav section, ascending; ties broken by title
	Draft       bool           `yaml:"draft" toml:"draft"` // drafts are excluded from the site
	Extra       map[string]any `yaml:",inline" toml:"-"`
}

var bom = []byte("\xef\xbb\xbf")

// Parse splits src into its front matter and body.
//
// Front matter is recognized only when the document starts (after an optional
// UTF-8 BOM) with a line that is exactly "---" (YAML) or "+++" (TOML). YAML
// ends at the next line that is "---" or "..."; TOML ends at the next "+++".
// Without front matter Parse returns a zero Meta and the document with any
// BOM removed.
func Parse(src []byte) (Meta, []byte, error) {
	src = bytes.TrimPrefix(src, bom)

	first, rest, ok := cutLine(src)
	if !ok {
		return Meta{}, src, nil
	}
	var isTOML bool
	switch string(bytes.TrimRight(first, " \t")) {
	case "---":
	case "+++":
		isTOML = true
	default:
		return Meta{}, src, nil
	}

	var blockEnd int
	body := rest
	for {
		line, next, ok := cutLine(body)
		if !ok {
			return Meta{}, nil, fmt.Errorf("unclosed front matter: missing closing %q", first[:3])
		}
		if isClosing(line, isTOML) {
			blockEnd = len(rest) - len(body)
			body = next
			break
		}
		body = next
	}

	block := rest[:blockEnd]
	if isTOML {
		meta, err := parseTOML(block)
		return meta, body, err
	}
	var meta Meta
	if err := yaml.Unmarshal(block, &meta); err != nil {
		return Meta{}, nil, fmt.Errorf("invalid YAML front matter: %w", err)
	}
	return meta, body, nil
}

// parseTOML decodes block into Meta, collecting top-level keys that Meta
// does not declare into Extra.
func parseTOML(block []byte) (Meta, error) {
	var raw map[string]any
	if err := toml.Unmarshal(block, &raw); err != nil {
		return Meta{}, fmt.Errorf("invalid TOML front matter: %w", err)
	}
	var meta Meta
	md, err := toml.Decode(string(block), &meta)
	if err != nil {
		return Meta{}, fmt.Errorf("invalid TOML front matter: %w", err)
	}
	for _, key := range md.Undecoded() {
		if len(key) != 1 {
			continue
		}
		if meta.Extra == nil {
			meta.Extra = make(map[string]any)
		}
		meta.Extra[key[0]] = raw[key[0]]
	}
	return meta, nil
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

func isClosing(line []byte, isTOML bool) bool {
	line = bytes.TrimRight(line, " \t")
	if isTOML {
		return string(line) == "+++"
	}
	return string(line) == "---" || string(line) == "..."
}
