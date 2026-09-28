package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

const specYAML = `system: "base16"
name: "Tomorrow Night"
author: "Chris Kempson (http://chriskempson.com)"
variant: "dark"
palette:
  base00: "#1d1f21"
  base01: "#282a2e"
  base02: "#373b41"
  base03: "#969896"
  base04: "#b4b7b4"
  base05: "#c5c8c6"
  base06: "#e0e0e0"
  base07: "#ffffff"
  base08: "#cc6666"
  base09: "#de935f"
  base0A: "#f0c674"
  base0B: "#b5bd68"
  base0C: "#8abeb7"
  base0D: "#81a2be"
  base0E: "#b294bb"
  base0F: "#a3685a"
`

const legacyYAML = `scheme: "Tomorrow"
author: "Chris Kempson (http://chriskempson.com)"
base00: "FFFFFF"
base01: "e0e0e0"
base02: "d6d6d6"
base03: "8e908c"
base04: "969896"
base05: "4d4d4c"
base06: "282a2e"
base07: "1d1f21"
base08: "c82829"
base09: "f5871f"
base0A: "eab700"
base0B: "718c00"
base0C: "3e999f"
base0D: "4271ae"
base0E: "8959a8"
base0F: 000000
`

var tomorrowNight = [16]string{
	"#1d1f21", "#282a2e", "#373b41", "#969896", "#b4b7b4", "#c5c8c6", "#e0e0e0", "#ffffff",
	"#cc6666", "#de935f", "#f0c674", "#b5bd68", "#8abeb7", "#81a2be", "#b294bb", "#a3685a",
}

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want Scheme
	}{
		{
			name: "tinted spec",
			in:   specYAML,
			want: Scheme{Name: "Tomorrow Night", Author: "Chris Kempson (http://chriskempson.com)", Variant: "dark", Palette: tomorrowNight},
		},
		{
			name: "legacy format infers light variant",
			in:   legacyYAML,
			want: Scheme{Name: "Tomorrow", Author: "Chris Kempson (http://chriskempson.com)", Variant: "light", Palette: [16]string{
				"#ffffff", "#e0e0e0", "#d6d6d6", "#8e908c", "#969896", "#4d4d4c", "#282a2e", "#1d1f21",
				"#c82829", "#f5871f", "#eab700", "#718c00", "#3e999f", "#4271ae", "#8959a8", "#000000",
			}},
		},
		{
			name: "missing variant inferred dark",
			in:   strings.Replace(specYAML, `variant: "dark"`, "", 1),
			want: Scheme{Name: "Tomorrow Night", Author: "Chris Kempson (http://chriskempson.com)", Variant: "dark", Palette: tomorrowNight},
		},
		{
			name: "unknown variant replaced by inference",
			in:   strings.Replace(specYAML, `variant: "dark"`, `variant: "dusk"`, 1),
			want: Scheme{Name: "Tomorrow Night", Author: "Chris Kempson (http://chriskempson.com)", Variant: "dark", Palette: tomorrowNight},
		},
		{
			name: "base24 keeps first sixteen colors",
			in:   strings.Replace(specYAML, `system: "base16"`, `system: "base24"`, 1) + "  base10: \"#000000\"\n  base17: \"#111111\"\n",
			want: Scheme{Name: "Tomorrow Night", Author: "Chris Kempson (http://chriskempson.com)", Variant: "dark", Palette: tomorrowNight},
		},
		{
			name: "uppercase hex normalized",
			in:   strings.Replace(specYAML, `"#1d1f21"`, `"#1D1F21"`, 1),
			want: Scheme{Name: "Tomorrow Night", Author: "Chris Kempson (http://chriskempson.com)", Variant: "dark", Palette: tomorrowNight},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got != tt.want {
				t.Errorf("Parse =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "bad yaml", in: "name: [unclosed", wantErr: "parse scheme"},
		{name: "not a mapping", in: "- a\n- b\n", wantErr: "parse scheme"},
		{name: "empty", in: "", wantErr: "missing color base00"},
		{name: "missing color", in: strings.Replace(specYAML, `  base0C: "#8abeb7"`+"\n", "", 1), wantErr: "missing color base0C"},
		{name: "short hex", in: strings.Replace(specYAML, `"#1d1f21"`, `"#fff"`, 1), wantErr: `invalid color base00 "#fff"`},
		{name: "non hex digits", in: strings.Replace(specYAML, `"#1d1f21"`, `"#zzzzzz"`, 1), wantErr: "invalid color base00"},
		{name: "unsupported system", in: strings.Replace(specYAML, `system: "base16"`, `system: "base8"`, 1), wantErr: `unsupported system "base8"`},
		{name: "palette not a mapping", in: "name: x\npalette: [1, 2]\n", wantErr: "palette"},
		{name: "color not a scalar", in: strings.Replace(specYAML, `base00: "#1d1f21"`, `base00: [1]`, 1), wantErr: "palette"},
		{name: "legacy color not a scalar", in: strings.Replace(legacyYAML, `base00: "FFFFFF"`, `base00: {a: b}`, 1), wantErr: "missing color base00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tt.in))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEmbeddedSchemesParse(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(schemeFiles, "schemes/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no embedded schemes")
	}
	for _, f := range files {
		data, err := schemeFiles.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s, err := Parse(data)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if s.Name == "" || (s.Variant != "dark" && s.Variant != "light") {
			t.Errorf("%s: name %q variant %q", f, s.Name, s.Variant)
		}
	}
	if len(Builtin()) != len(files) {
		t.Errorf("Builtin() has %d schemes, %d files embedded", len(Builtin()), len(files))
	}
}

func TestBuiltin(t *testing.T) {
	t.Parallel()
	schemes := Builtin()
	if len(schemes) == 0 {
		t.Fatal("Builtin() is empty")
	}
	if !slices.IsSortedFunc(schemes, func(a, b Scheme) int { return strings.Compare(a.Slug, b.Slug) }) {
		t.Error("Builtin() not sorted by slug")
	}
	for _, slug := range []string{"tomorrow", "tomorrow-night"} {
		if !slices.ContainsFunc(schemes, func(s Scheme) bool { return s.Slug == slug }) {
			t.Errorf("Builtin() missing %q", slug)
		}
	}
	schemes[0].Name = "mutated"
	if Builtin()[0].Name == "mutated" {
		t.Error("Builtin() exposes shared slice")
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	s, ok := Lookup("tomorrow-night")
	if !ok {
		t.Fatal("Lookup(tomorrow-night) not found")
	}
	want := Scheme{Slug: "tomorrow-night", Name: "Tomorrow Night", Author: "Chris Kempson (http://chriskempson.com)", Variant: "dark", Palette: tomorrowNight}
	if s != want {
		t.Errorf("Lookup = %+v, want %+v", s, want)
	}
	if light, _ := Lookup("tomorrow"); light.Variant != "light" {
		t.Errorf("tomorrow variant = %q, want light", light.Variant)
	}
	if _, ok := Lookup("no-such-scheme"); ok {
		t.Error("Lookup(no-such-scheme) found")
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	specPath := write("my-theme.yaml", specYAML)
	legacyPath := write("old.yml", legacyYAML)
	noNamePath := write("unnamed.yaml", strings.Replace(specYAML, `name: "Tomorrow Night"`, "", 1))
	badPath := write("bad.yaml", "palette: {}")

	tests := []struct {
		name     string
		in       string
		wantSlug string
		wantName string
		wantErr  []string
	}{
		{name: "builtin by slug", in: "tomorrow-night", wantSlug: "tomorrow-night", wantName: "Tomorrow Night"},
		{name: "builtin is case-insensitive", in: "Tomorrow-Night", wantSlug: "tomorrow-night", wantName: "Tomorrow Night"},
		{name: "yaml path", in: specPath, wantSlug: "my-theme", wantName: "Tomorrow Night"},
		{name: "yml path", in: legacyPath, wantSlug: "old", wantName: "Tomorrow"},
		{name: "name falls back to slug", in: noNamePath, wantSlug: "unnamed", wantName: "unnamed"},
		{name: "relative extension-only path", in: "missing.yaml", wantErr: []string{"missing.yaml"}},
		{name: "path with separator", in: filepath.Join(dir, "nope"), wantErr: []string{"nope"}},
		{name: "invalid file", in: badPath, wantErr: []string{"bad.yaml", "missing color"}},
		{name: "unknown with suggestions", in: "tomorow-night", wantErr: []string{`unknown theme "tomorow-night"`, "did you mean", "tomorrow-night"}},
		{name: "unknown substring suggestions", in: "gruvbox", wantErr: []string{"did you mean", "gruvbox-"}},
		{name: "unknown with builtin as substring", in: "nord-dark", wantErr: []string{"did you mean", "nord"}},
		{name: "unknown without suggestions", in: "zzzzzzzzzzzzzzzzzzzz", wantErr: []string{`unknown theme "zzzzzzzzzzzzzzzzzzzz"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := Load(tt.in)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Load(%q) succeeded, want error", tt.in)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(%q): %v", tt.in, err)
			}
			if s.Slug != tt.wantSlug || s.Name != tt.wantName {
				t.Errorf("Load(%q) = slug %q name %q, want %q %q", tt.in, s.Slug, s.Name, tt.wantSlug, tt.wantName)
			}
		})
	}
}

func TestLoadSuggestionLimit(t *testing.T) {
	t.Parallel()
	_, err := Load("gruvbox")
	if err == nil {
		t.Fatal("expected error")
	}
	_, list, _ := strings.Cut(err.Error(), "did you mean ")
	hints := strings.Split(list, ", ")
	if len(hints) > 3 {
		t.Errorf("got %d suggestions, want at most 3: %s", len(hints), err)
	}
	for _, h := range hints {
		if !strings.HasPrefix(h, "gruvbox") {
			t.Errorf("unrelated suggestion %q in %s", h, err)
		}
	}
}

func TestCSSVars(t *testing.T) {
	t.Parallel()
	s := Scheme{Palette: tomorrowNight}
	want := "--base00: #1d1f21; --base01: #282a2e; --base02: #373b41; --base03: #969896; " +
		"--base04: #b4b7b4; --base05: #c5c8c6; --base06: #e0e0e0; --base07: #ffffff; " +
		"--base08: #cc6666; --base09: #de935f; --base0A: #f0c674; --base0B: #b5bd68; " +
		"--base0C: #8abeb7; --base0D: #81a2be; --base0E: #b294bb; --base0F: #a3685a;"
	if got := s.CSSVars(); got != want {
		t.Errorf("CSSVars =\n%s\nwant\n%s", got, want)
	}
}

func TestSyntaxCSS(t *testing.T) {
	t.Parallel()
	css := SyntaxCSS()
	if css != SyntaxCSS() {
		t.Error("SyntaxCSS not deterministic")
	}
	if m := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`).FindString(css); m != "" {
		t.Errorf("SyntaxCSS contains hard-coded color %s", m)
	}
	if m := regexp.MustCompile(`var\(--(?:base0[^0-9A-F]|base[^0])`).FindString(css); m != "" {
		t.Errorf("SyntaxCSS references unknown variable %s", m)
	}

	for tt, class := range chroma.StandardTypes {
		if class == "" || (tt < 0 && tt != chroma.Error && tt != chroma.Other) {
			continue
		}
		if !strings.Contains(css, ".chroma ."+class+" {") {
			t.Errorf("no rule for %s (.%s)", tt, class)
		}
	}

	rules := map[string]string{
		".chroma {":      "var(--base05)",
		".bg {":          "background-color: var(--base0",
		".chroma .c {":   "var(--base03)",
		".chroma .c1 {":  "italic",
		".chroma .k {":   "var(--base0E)",
		".chroma .s {":   "var(--base0B)",
		".chroma .s2 {":  "var(--base0B)",
		".chroma .m {":   "var(--base09)",
		".chroma .mi {":  "var(--base09)",
		".chroma .kc {":  "var(--base09)",
		".chroma .nf {":  "var(--base0D)",
		".chroma .nc {":  "var(--base0A)",
		".chroma .kt {":  "var(--base0A)",
		".chroma .nv {":  "var(--base08)",
		".chroma .nt {":  "var(--base08)",
		".chroma .nb {":  "var(--base0C)",
		".chroma .se {":  "var(--base0C)",
		".chroma .sr {":  "var(--base0C)",
		".chroma .o {":   "var(--base05)",
		".chroma .p {":   "var(--base05)",
		".chroma .gi {":  "var(--base0B)",
		".chroma .gd {":  "var(--base08)",
		".chroma .ln {":  "var(--base03)",
		".chroma .lnt {": "var(--base03)",
		".chroma .hl {":  "background-color: var(--base02)",
	}
	for selector, want := range rules {
		_, rest, ok := strings.Cut(css, selector)
		if !ok {
			t.Errorf("missing rule %q", selector)
			continue
		}
		body, _, _ := strings.Cut(rest, "}")
		if !strings.Contains(body, want) {
			t.Errorf("rule %q = %q, want %q", selector, body, want)
		}
	}
}
