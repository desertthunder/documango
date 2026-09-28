package theme

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

//go:embed schemes/*.yaml
var schemeFiles embed.FS

// curated lists the slugs of the embedded schemes. The schemes tool refreshes
// exactly these from tinted-theming/schemes; add a slug here to embed it.
var curated = []string{
	"catppuccin-latte", "catppuccin-mocha", "default-dark", "default-light", "dracula",
	"everforest-dark-medium", "everforest-light-medium", "github", "github-dark",
	"gruvbox-dark-medium", "gruvbox-light-medium", "kanagawa", "nord", "nord-light",
	"one-light", "onedark", "rose-pine", "rose-pine-dawn", "solarized-dark", "solarized-light",
	"tokyo-night-dark", "tokyo-night-light", "tomorrow", "tomorrow-night",
}

// Curated returns the slugs of the embedded schemes.
func Curated() []string {
	return slices.Clone(curated)
}

// Scheme is a base16 color scheme.
type Scheme struct {
	// Slug is the file name without extension, e.g. "tomorrow-night".
	Slug string
	// Name is the display name, e.g. "Tomorrow Night".
	Name   string
	Author string
	// Variant is "dark" or "light".
	Variant string
	// Palette holds lowercase "#rrggbb" colors; index 0 is base00, 15 is base0F.
	Palette [16]string
}

// Parse decodes a scheme in the tinted-theming format (a palette mapping with
// "#rrggbb" values) or the legacy base16 format (top-level scheme name and
// base00..base0F keys without '#'). Base24 schemes are accepted and reduced to
// their first sixteen colors. When the variant is missing or unrecognised it
// is inferred from the luminance of base00. Slug is left empty.
func Parse(data []byte) (Scheme, error) {
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Scheme{}, fmt.Errorf("parse scheme: %w", err)
	}
	str := func(key string) string {
		if n, ok := doc[key]; ok && n.Kind == yaml.ScalarNode {
			return n.Value
		}
		return ""
	}

	if sys := str("system"); sys != "" && sys != "base16" && sys != "base24" {
		return Scheme{}, fmt.Errorf("parse scheme: unsupported system %q", sys)
	}

	colors := map[string]string{}
	if node, ok := doc["palette"]; ok {
		if err := node.Decode(&colors); err != nil {
			return Scheme{}, fmt.Errorf("parse scheme palette: %w", err)
		}
	} else {
		for key := range doc {
			colors[key] = str(key)
		}
	}
	lower := make(map[string]string, len(colors))
	for k, v := range colors {
		lower[strings.ToLower(k)] = v
	}

	s := Scheme{Name: str("name"), Author: str("author"), Variant: strings.ToLower(str("variant"))}
	if s.Name == "" {
		s.Name = str("scheme")
	}
	for i := range s.Palette {
		key := fmt.Sprintf("base%02X", i)
		raw := lower[strings.ToLower(key)]
		if raw == "" {
			return Scheme{}, fmt.Errorf("parse scheme: missing color %s", key)
		}
		hex := strings.ToLower(strings.TrimPrefix(raw, "#"))
		if len(hex) != 6 {
			return Scheme{}, fmt.Errorf("parse scheme: invalid color %s %q", key, raw)
		}
		if _, err := strconv.ParseUint(hex, 16, 32); err != nil {
			return Scheme{}, fmt.Errorf("parse scheme: invalid color %s %q", key, raw)
		}
		s.Palette[i] = "#" + hex
	}

	if s.Variant != "dark" && s.Variant != "light" {
		// Palette entries are validated, so parsing cannot fail here.
		rgb, _ := strconv.ParseUint(s.Palette[0][1:], 16, 32)
		r, g, b := float64(rgb>>16&0xff), float64(rgb>>8&0xff), float64(rgb&0xff)
		if (0.2126*r+0.7152*g+0.0722*b)/255 < 0.5 {
			s.Variant = "dark"
		} else {
			s.Variant = "light"
		}
	}
	return s, nil
}

var builtin = sync.OnceValue(func() []Scheme {
	files, err := fs.Glob(schemeFiles, "schemes/*.yaml")
	if err != nil {
		panic(fmt.Sprintf("theme: list embedded schemes: %v", err))
	}
	schemes := make([]Scheme, 0, len(files))
	for _, f := range files {
		data, err := schemeFiles.ReadFile(f)
		if err != nil {
			panic(fmt.Sprintf("theme: read embedded %s: %v", f, err))
		}
		s, err := Parse(data)
		if err != nil {
			panic(fmt.Sprintf("theme: embedded %s: %v", f, err))
		}
		schemes = append(schemes, withSlug(s, strings.TrimSuffix(path.Base(f), ".yaml")))
	}
	slices.SortFunc(schemes, func(a, b Scheme) int { return strings.Compare(a.Slug, b.Slug) })
	return schemes
})

func withSlug(s Scheme, slug string) Scheme {
	s.Slug = slug
	if s.Name == "" {
		s.Name = slug
	}
	return s
}

// Builtin returns all embedded schemes sorted by Slug. The returned slice is
// a copy the caller may modify.
func Builtin() []Scheme {
	return slices.Clone(builtin())
}

// Lookup returns the embedded scheme with the given slug.
func Lookup(slug string) (Scheme, bool) {
	return find(builtin(), slug)
}

// find returns the scheme with the given slug from schemes sorted by slug.
func find(schemes []Scheme, slug string) (Scheme, bool) {
	i, ok := slices.BinarySearchFunc(schemes, slug, func(s Scheme, slug string) int {
		return strings.Compare(s.Slug, slug)
	})
	if !ok {
		return Scheme{}, false
	}
	return schemes[i], true
}

// Load resolves nameOrPath to a scheme. Values ending in .yaml or .yml, or
// containing a path separator, are read from disk and take their Slug from
// the file name. Anything else is looked up case-insensitively among the
// embedded schemes; an unknown name yields an error listing up to three
// similar slugs. Catalog.Load also resolves schemes that are not embedded.
func Load(nameOrPath string) (Scheme, error) {
	if isPath(nameOrPath) {
		return loadFile(nameOrPath)
	}
	if s, ok := Lookup(strings.ToLower(nameOrPath)); ok {
		return s, nil
	}
	return Scheme{}, errors.New(unknownTheme(nameOrPath, builtin()))
}

func isPath(nameOrPath string) bool {
	ext := strings.ToLower(filepath.Ext(nameOrPath))
	return ext == ".yaml" || ext == ".yml" || strings.ContainsAny(nameOrPath, `/\`)
}

func loadFile(name string) (Scheme, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return Scheme{}, fmt.Errorf("load theme: %w", err)
	}
	s, err := Parse(data)
	if err != nil {
		return Scheme{}, fmt.Errorf("load theme %s: %w", name, err)
	}
	return withSlug(s, strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))), nil
}

// unknownTheme describes an unknown theme name, suggesting up to three slugs
// from schemes that are close to it, nearest first.
func unknownTheme(name string, schemes []Scheme) string {
	type candidate struct {
		slug string
		dist int
	}
	msg := fmt.Sprintf("unknown theme %q", name)
	name = strings.ToLower(name)
	maxDist := max(2, len(name)/3)
	var found []candidate
	for _, s := range schemes {
		d := levenshtein(name, s.Slug)
		if d <= maxDist || strings.Contains(s.Slug, name) || (len(s.Slug) >= 3 && strings.Contains(name, s.Slug)) {
			found = append(found, candidate{s.Slug, d})
		}
	}
	if len(found) == 0 {
		return msg
	}
	slices.SortStableFunc(found, func(a, b candidate) int { return a.dist - b.dist })
	var hints []string
	for _, c := range found[:min(3, len(found))] {
		hints = append(hints, c.slug)
	}
	return msg + "; did you mean " + strings.Join(hints, ", ")
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// CSSVars returns the palette as sixteen CSS custom property declarations,
// "--base00: #rrggbb; ... --base0F: #rrggbb;", separated by single spaces.
func (s Scheme) CSSVars() string {
	var b strings.Builder
	for i, c := range s.Palette {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "--base%02X: %s;", i, c)
	}
	return b.String()
}
