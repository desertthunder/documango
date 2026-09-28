// Package config reads the optional documango.toml or documango.yaml file
// that sets a site's title, metadata, themes and header links.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"
)

// Names are the config file names looked for at the root of a docs folder.
var Names = []string{"documango.toml", "documango.yaml", "documango.yml"}

// Config is a site's configuration. Zero fields are unset.
type Config struct {
	// Path is the file the config was read from; "" when there is none.
	Path string `toml:"-" yaml:"-"`

	Title       string `toml:"title" yaml:"title"`
	Description string `toml:"description" yaml:"description"`
	// URL is the absolute http(s) address of the published site, ending in "/".
	URL string `toml:"url" yaml:"url"`
	// BasePath defaults to the path of URL.
	BasePath string `toml:"base_path" yaml:"base_path"`
	Author   string `toml:"author" yaml:"author"`
	Language string `toml:"language" yaml:"language"`
	// Favicon and Logo are clean slash paths of files in the docs folder.
	Favicon string `toml:"favicon" yaml:"favicon"`
	Logo    string `toml:"logo" yaml:"logo"`
	Theme   Theme  `toml:"theme" yaml:"theme"`
	Fonts   Fonts  `toml:"fonts" yaml:"fonts"`
	Links   []Link `toml:"links" yaml:"links"`
	// Footer is Markdown shown at the foot of every page. nil keeps the
	// default footer; "" removes the footer.
	Footer *string `toml:"footer" yaml:"footer"`
}

// Fonts names the web fonts of the site, by family name such as "Inter" or
// by Fontsource ID such as "inter". Empty fields keep the system fonts.
type Fonts struct {
	Body string `toml:"body" yaml:"body"`
	// Heading defaults to Body.
	Heading string `toml:"heading" yaml:"heading"`
	Mono    string `toml:"mono" yaml:"mono"`
}

// Theme selects the color schemes and search engine.
type Theme struct {
	// Dark and Light hold theme names or base16 YAML files; theme files are
	// resolved against the config file's folder.
	Dark   Themes `toml:"dark" yaml:"dark"`
	Light  Themes `toml:"light" yaml:"light"`
	Search string `toml:"search" yaml:"search"`
}

// Themes is a list of themes, written as one name or a list of names.
type Themes []string

var errThemes = errors.New("theme must be a name or a list of names")

func (t *Themes) UnmarshalTOML(v any) error {
	switch v := v.(type) {
	case string:
		*t = Themes{v}
	case []any:
		*t = make(Themes, len(v))
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return errThemes
			}
			(*t)[i] = s
		}
	default:
		return errThemes
	}
	return nil
}

func (t *Themes) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*t = Themes{n.Value}
		return nil
	case yaml.SequenceNode:
		return n.Decode((*[]string)(t))
	}
	return errThemes
}

// Link is a header link.
type Link struct {
	Title string `toml:"title" yaml:"title"`
	URL   string `toml:"url" yaml:"url"`
}

// Load reads the config file at file, or when file is "", the one config
// file among Names in dir, if any. dir is the docs folder that favicon and
// logo paths are checked against.
func Load(dir, file string) (*Config, error) {
	if file == "" {
		var found []string
		for _, name := range Names {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				found = append(found, p)
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("find config file: %w", err)
			}
		}
		switch len(found) {
		case 0:
			return &Config{}, nil
		case 1:
			file = found[0]
		default:
			return nil, fmt.Errorf("found config files %s: keep only one", strings.Join(found, " and "))
		}
	}

	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("config file %s does not exist", file)
	}
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	c := &Config{Path: file}
	switch strings.ToLower(filepath.Ext(file)) {
	case ".toml":
		md, err := toml.Decode(string(data), c)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if keys := md.Undecoded(); len(keys) > 0 {
			return nil, fmt.Errorf("%s: unknown key %q", file, keys[0].String())
		}
	case ".yaml", ".yml":
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(c); err != nil && err != io.EOF {
			return nil, fmt.Errorf("%s: %w", file, yamlError(err))
		}
	default:
		return nil, fmt.Errorf("config file %s: use a .toml, .yaml or .yml file", file)
	}
	if err := c.check(dir); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return c, nil
}

var unknownField = regexp.MustCompile(`^(line \d+): field (\S+) not found in type \S+$`)

// yamlError rewrites the decoder's unknown field errors in config terms.
func yamlError(err error) error {
	var te *yaml.TypeError
	if !errors.As(err, &te) {
		return err
	}
	msgs := make([]string, len(te.Errors))
	for i, m := range te.Errors {
		msgs[i] = unknownField.ReplaceAllString(m, `$1: unknown key "$2"`)
	}
	return errors.New(strings.Join(msgs, "; "))
}

// check validates c and normalizes its paths and URL.
func (c *Config) check(dir string) error {
	if c.URL != "" {
		u, err := url.Parse(c.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("url %q must be an absolute http or https URL, such as https://example.com/docs/", c.URL)
		}
		if !strings.HasSuffix(u.Path, "/") {
			u.Path += "/"
			u.RawPath = ""
		}
		c.URL = u.String()
		if c.BasePath == "" {
			c.BasePath = u.Path
		}
	}
	if c.BasePath != "" {
		u, err := url.Parse(c.BasePath)
		if err != nil || u.Scheme != "" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("base_path %q must be a URL path, such as /docs/", c.BasePath)
		}
	}

	for _, f := range []struct {
		key  string
		path *string
	}{{"favicon", &c.Favicon}, {"logo", &c.Logo}} {
		if *f.path == "" {
			continue
		}
		name := path.Clean(filepath.ToSlash(*f.path))
		if !fs.ValidPath(name) {
			return fmt.Errorf("%s %q must be a file inside the docs folder", f.key, *f.path)
		}
		for part := range strings.SplitSeq(name, "/") {
			if strings.HasPrefix(part, "_") || strings.HasPrefix(part, ".") {
				return fmt.Errorf("%s %q is skipped when the site is built: no part of its path may start with _ or .", f.key, *f.path)
			}
		}
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s %q does not exist in the docs folder", f.key, *f.path)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", f.key, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s %q is a folder, not a file", f.key, *f.path)
		}
		*f.path = name
	}

	for _, t := range []struct {
		key    string
		themes Themes
	}{{"theme.dark", c.Theme.Dark}, {"theme.light", c.Theme.Light}} {
		seen := map[string]bool{}
		for i, name := range t.themes {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("%s has an empty entry", t.key)
			}
			if seen[strings.ToLower(name)] {
				return fmt.Errorf("%s lists %q more than once", t.key, name)
			}
			seen[strings.ToLower(name)] = true
			// Same test as the theme package: a YAML extension or a separator
			// marks a file rather than a theme name.
			ext := strings.ToLower(filepath.Ext(name))
			isFile := ext == ".yaml" || ext == ".yml" || strings.ContainsAny(name, `/\`)
			if isFile && !filepath.IsAbs(name) && c.Path != "" {
				t.themes[i] = filepath.Join(filepath.Dir(c.Path), name)
			}
		}
	}
	if s := c.Theme.Search; s != "" && s != "pagefind" && s != "builtin" {
		return fmt.Errorf("theme.search %q is not pagefind or builtin", s)
	}

	for _, f := range []struct {
		key  string
		name *string
	}{{"fonts.body", &c.Fonts.Body}, {"fonts.heading", &c.Fonts.Heading}, {"fonts.mono", &c.Fonts.Mono}} {
		*f.name = strings.TrimSpace(*f.name)
		if strings.ContainsAny(*f.name, "\";{}<\\\n\r") {
			return fmt.Errorf("%s %q must be a font family name, such as Inter", f.key, *f.name)
		}
	}

	for i, l := range c.Links {
		if strings.TrimSpace(l.Title) == "" {
			return fmt.Errorf("links[%d]: title is empty", i+1)
		}
		if strings.TrimSpace(l.URL) == "" {
			return fmt.Errorf("links[%d]: url is empty", i+1)
		}
	}
	return nil
}
