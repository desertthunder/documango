package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const fullTOML = `title = "Acme Docs"
description = "Docs for Acme."
url = "https://acme.dev/docs"
author = "Acme Inc."
language = "fr"
favicon = "favicon.svg"
logo = "./img/logo.png"

[theme]
dark = ["tomorrow-night", "github-dark"]
light = "tomorrow"
search = "builtin"

[fonts]
body = " Inter "
mono = "JetBrains Mono"

[[links]]
title = "GitHub"
url = "https://github.com/acme/acme"

[[links]]
title = "Blog"
url = "/blog/"
`

const fullYAML = `title: Acme Docs
description: Docs for Acme.
url: https://acme.dev/docs
author: Acme Inc.
language: fr
favicon: favicon.svg
logo: ./img/logo.png
theme:
  dark: [tomorrow-night, github-dark]
  light: tomorrow
  search: builtin
fonts:
  body: " Inter "
  mono: JetBrains Mono
links:
  - title: GitHub
    url: https://github.com/acme/acme
  - title: Blog
    url: /blog/
`

func assetsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "favicon.svg"), "<svg/>")
	writeFile(t, filepath.Join(dir, "img", "logo.png"), "png")
	return dir
}

func TestLoadFormats(t *testing.T) {
	for name, body := range map[string]string{"documango.toml": fullTOML, "documango.yaml": fullYAML, "documango.yml": fullYAML} {
		t.Run(name, func(t *testing.T) {
			dir := assetsDir(t)
			writeFile(t, filepath.Join(dir, name), body)
			c, err := Load(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			want := &Config{
				Path:        filepath.Join(dir, name),
				Title:       "Acme Docs",
				Description: "Docs for Acme.",
				URL:         "https://acme.dev/docs/",
				BasePath:    "/docs/",
				Author:      "Acme Inc.",
				Language:    "fr",
				Favicon:     "favicon.svg",
				Logo:        "img/logo.png",
				Theme:       Theme{Dark: Themes{"tomorrow-night", "github-dark"}, Light: Themes{"tomorrow"}, Search: "builtin"},
				Fonts:       Fonts{Body: "Inter", Mono: "JetBrains Mono"},
				Links:       []Link{{"GitHub", "https://github.com/acme/acme"}, {"Blog", "/blog/"}},
			}
			if !reflect.DeepEqual(c, want) {
				t.Errorf("got  %+v\nwant %+v", c, want)
			}
		})
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir, "")
	if err != nil || !reflect.DeepEqual(c, &Config{}) {
		t.Errorf("no config: %+v, %v", c, err)
	}
	_, err = Load(dir, filepath.Join(dir, "nope.toml"))
	if err == nil || !strings.Contains(err.Error(), "nope.toml") || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing explicit config: %v", err)
	}
}

func TestLoadExplicit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "documango.toml"), `title = "Implicit"`)
	other := filepath.Join(t.TempDir(), "site.yaml")
	writeFile(t, other, "title: Explicit\ntheme:\n  dark: themes/mine.yaml\n  light: [tomorrow, /abs/light.yml]\n")
	c, err := Load(dir, other)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Explicit" || c.Path != other {
		t.Errorf("explicit config ignored: %+v", c)
	}
	// Theme files are relative to the config file.
	want := Theme{Dark: Themes{filepath.Join(filepath.Dir(other), "themes/mine.yaml")}, Light: Themes{"tomorrow", "/abs/light.yml"}}
	if !reflect.DeepEqual(c.Theme, want) {
		t.Errorf("theme = %+v, want %+v", c.Theme, want)
	}
}

func TestLoadEmpty(t *testing.T) {
	for _, name := range []string{"documango.toml", "documango.yaml"} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, name), "")
		if c, err := Load(dir, ""); err != nil || c.Path == "" || c.Title != "" {
			t.Errorf("%s: %+v, %v", name, c, err)
		}
	}
}

func TestBasePath(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`url = "https://acme.dev"`, "/"},
		{`url = "https://acme.dev/docs/"`, "/docs/"},
		{"url = \"https://acme.dev/docs/\"\nbase_path = \"/\"", "/"},
		{`base_path = "guide"`, "guide"},
		{``, ""},
	} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "documango.toml"), tc.body)
		c, err := Load(dir, "")
		if err != nil || c.BasePath != tc.want {
			t.Errorf("%q: base path %q, %v; want %q", tc.body, c.BasePath, err, tc.want)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"documango.toml", `colour = "red"`, `documango.toml: unknown key "colour"`},
		{"documango.toml", "[theme]\nsearch = \"builtin\"\nmode = 1", `unknown key "theme.mode"`},
		{"documango.toml", "[[links]]\ntitle = \"A\"\nurl = \"/a/\"\nicon = \"x\"", `unknown key "links.icon"`},
		{"documango.yaml", "colour: red\n", `documango.yaml: line 1: unknown key "colour"`},
		{"documango.yaml", "links:\n  - title: A\n    href: /a/\n", `line 3: unknown key "href"`},
		{"documango.toml", `title = `, "documango.toml:"},
		{"documango.yaml", "title: [\n", "documango.yaml:"},
		{"documango.toml", `title = 3`, "documango.toml:"},
		{"documango.toml", "[theme]\ndark = 3", "theme must be a name or a list of names"},
		{"documango.toml", "[theme]\ndark = [3]", "theme must be a name or a list of names"},
		{"documango.yaml", "theme:\n  dark: {a: b}\n", "theme must be a name or a list of names"},
		{"documango.yaml", "theme:\n  dark: [[a]]\n", "documango.yaml:"},
		{"documango.toml", "[theme]\nlight = [\"a\", \"\"]", `theme.light has an empty entry`},
		{"documango.toml", "[theme]\nlight = [\"a\", \"A\"]", `theme.light lists "A" more than once`},
		{"documango.toml", "[theme]\nsearch = \"lunr\"", `theme.search "lunr" is not pagefind or builtin`},
		{"documango.toml", `url = "acme.dev/docs"`, `url "acme.dev/docs" must be an absolute http or https URL`},
		{"documango.toml", `url = "ftp://acme.dev/"`, "must be an absolute http or https URL"},
		{"documango.toml", `url = "https://acme.dev/?q=1"`, "must be an absolute http or https URL"},
		{"documango.toml", `url = "https://acme.dev/%zz"`, "must be an absolute http or https URL"},
		{"documango.toml", `base_path = "https://acme.dev/docs/"`, `base_path "https://acme.dev/docs/" must be a URL path`},
		{"documango.toml", `base_path = "/docs/#top"`, "must be a URL path"},
		{"documango.toml", `favicon = "missing.svg"`, `favicon "missing.svg" does not exist in the docs folder`},
		{"documango.toml", `logo = "../logo.svg"`, `logo "../logo.svg" must be a file inside the docs folder`},
		{"documango.toml", `logo = "/abs/logo.svg"`, "must be a file inside the docs folder"},
		{"documango.toml", `logo = "img"`, `logo "img" is a folder`},
		{"documango.toml", `logo = "_assets/logo.svg"`, `logo "_assets/logo.svg" is skipped`},
		{"documango.toml", `favicon = ".hidden.svg"`, "is skipped"},
		{"documango.toml", "[[links]]\ntitle = \"GitHub\"", "links[1]: url is empty"},
		{"documango.toml", "[fonts]\nsize = 3", `unknown key "fonts.size"`},
		{"documango.yaml", "fonts:\n  display: swap\n", `line 2: unknown key "display"`},
		{"documango.toml", "[fonts]\nbody = \"Inter; color: red\"", `fonts.body "Inter; color: red" must be a font family name`},
		{"documango.toml", "[fonts]\nheading = 'A\"B'", "fonts.heading"},
		{"documango.yaml", "fonts:\n  mono: \"a}b\"\n", "fonts.mono"},
		{"documango.yaml", "fonts:\n  mono: \"a{b\"\n", "fonts.mono"},
		{"documango.yaml", "fonts:\n  mono: \"</style>\"\n", "fonts.mono"},
		{"documango.yaml", "fonts:\n  mono: \"a\\\\b\"\n", "fonts.mono"},
		{"documango.yaml", "fonts:\n  body: \"a\\nb\"\n", "fonts.body"},
		{"documango.toml", "[[links]]\nurl = \"/a/\"\n[[links]]\nurl = \"/b/\"\ntitle = \"B\"", "links[1]: title is empty"},
	} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "img", "a.png"), "png")
		writeFile(t, filepath.Join(dir, "_assets", "logo.svg"), "<svg/>")
		writeFile(t, filepath.Join(dir, ".hidden.svg"), "<svg/>")
		writeFile(t, filepath.Join(dir, tc.name), tc.body)
		_, err := Load(dir, "")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %q: error %v, want %q", tc.name, tc.body, err, tc.want)
		}
	}
}

func TestLoadAmbiguous(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "documango.toml"), "")
	writeFile(t, filepath.Join(dir, "documango.yml"), "")
	_, err := Load(dir, "")
	if err == nil || !strings.Contains(err.Error(), "documango.toml") || !strings.Contains(err.Error(), "documango.yml") {
		t.Errorf("two configs: %v", err)
	}
}

func TestLoadUnknownExtension(t *testing.T) {
	p := filepath.Join(t.TempDir(), "site.json")
	writeFile(t, p, "{}")
	if _, err := Load(t.TempDir(), p); err == nil || !strings.Contains(err.Error(), ".toml, .yaml or .yml") {
		t.Errorf("json config: %v", err)
	}
	if _, err := Load(t.TempDir(), t.TempDir()); err == nil {
		t.Error("folder as config: no error")
	}
}

func TestFooter(t *testing.T) {
	const multi = "Made by [Acme](https://acme.dev).\n\nSee [the guide](guide/index.md).\n"
	for _, tc := range []struct {
		name, file, body string
		want             *string
	}{
		{"toml unset", "documango.toml", `title = "Acme"`, nil},
		{"toml empty", "documango.toml", `footer = ""`, ptr("")},
		{"toml multi-line", "documango.toml", "footer = \"\"\"\n" + multi + "\"\"\"", ptr(multi)},
		{"yaml unset", "documango.yaml", "title: Acme", nil},
		{"yaml empty", "documango.yaml", `footer: ""`, ptr("")},
		{"yaml multi-line", "documango.yaml", "footer: |\n  Made by [Acme](https://acme.dev).\n\n  See [the guide](guide/index.md).\n", ptr(multi)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, tc.file), tc.body)
			c, err := Load(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.Footer, tc.want) {
				t.Errorf("footer = %v, want %v", deref(c.Footer), deref(tc.want))
			}
		})
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<unset>"
	}
	return strconv.Quote(*s)
}
