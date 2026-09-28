package site

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func load(t *testing.T, fsys fs.FS, opts Options) *Site {
	t.Helper()
	s, err := Load(fsys, opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func urls(pages []*Page) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.URL
	}
	return out
}

// navShape renders the nav tree as "Title(URL)[children]" for compact comparison.
func navShape(items []*NavItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		s := it.Title + "(" + it.URL + ")"
		if len(it.Children) > 0 {
			s += "[" + navShape(it.Children) + "]"
		}
		parts[i] = s
	}
	return strings.Join(parts, " ")
}

func docsFS() fstest.MapFS {
	return fstest.MapFS{
		"index.md":                   file("# Welcome\n\nHome page.\n"),
		"getting-started.md":         file("---\norder: 1\n---\nNo heading here.\n"),
		"guide/README.md":            file("---\norder: 2\ndescription: The guide\n---\n# User Guide\n"),
		"guide/install.md":           file("# Install\n\n## Requirements\n\nSee [config](config.markdown#env) and ![logo](../img/logo.png).\n"),
		"guide/config.markdown":      file("---\ntitle: Configuration\norder: 1\n---\nConfig body.\n"),
		"guide/advanced/deep.md":     file("# Deep\n"),
		"api/zeta.md":                file("# Zeta\n"),
		"api/Alpha.MD":               file("# alpha\n"),
		"img/logo.png":               file("png"),
		"assets-only/file.txt":       file("txt"),
		"draft.md":                   file("---\ndraft: true\n---\n# Draft\n"),
		".hidden/secret.md":          file("# Secret\n"),
		".hidden.md":                 file("# Hidden\n"),
		"_partials/snippet.md":       file("# Partial\n"),
		"_private.png":               file("x"),
		"public/index.html":          file("<html>"),
		"public/old.md":              file("# Old\n"),
		"guide/advanced/diagram.svg": file("<svg/>"),
	}
}

func TestLoadStructure(t *testing.T) {
	t.Parallel()
	s := load(t, docsFS(), Options{Exclude: []string{"./public/"}})

	if s.Title != "Welcome" {
		t.Errorf("Title = %q, want Welcome", s.Title)
	}
	if s.Home == nil || s.Home.URL != "/" || s.Home.OutPath != "index.html" {
		t.Fatalf("Home = %+v", s.Home)
	}

	wantPages := []string{"/", "/api/Alpha/", "/api/zeta/", "/getting-started/", "/guide/", "/guide/advanced/deep/", "/guide/install/", "/guide/config/"}
	if got := urls(s.Pages); !reflect.DeepEqual(got, wantPages) {
		t.Errorf("Pages =\n%q\nwant\n%q", got, wantPages)
	}

	wantNav := "Api()[alpha(/api/Alpha/) Zeta(/api/zeta/)] Getting started(/getting-started/) " +
		"User Guide(/guide/)[Advanced()[Deep(/guide/advanced/deep/)] Install(/guide/install/) Configuration(/guide/config/)]"
	if got := navShape(s.Nav); got != wantNav {
		t.Errorf("Nav =\n%s\nwant\n%s", got, wantNav)
	}

	wantAssets := []string{"assets-only/file.txt", "guide/advanced/diagram.svg", "img/logo.png"}
	if !reflect.DeepEqual(s.Assets, wantAssets) {
		t.Errorf("Assets = %q, want %q", s.Assets, wantAssets)
	}
}

func TestPageFields(t *testing.T) {
	t.Parallel()
	s := load(t, docsFS(), Options{Exclude: []string{"public"}})

	tests := []struct {
		url, source, out, title, desc string
		hasH1                         bool
	}{
		{"/", "index.md", "index.html", "Welcome", "", true},
		{"/getting-started/", "getting-started.md", "getting-started/index.html", "Getting started", "", false},
		{"/guide/", "guide/README.md", "guide/index.html", "User Guide", "The guide", true},
		{"/guide/config/", "guide/config.markdown", "guide/config/index.html", "Configuration", "", false},
		{"/guide/install/", "guide/install.md", "guide/install/index.html", "Install", "", true},
		{"/api/Alpha/", "api/Alpha.MD", "api/Alpha/index.html", "alpha", "", true},
	}
	for _, tt := range tests {
		p := s.PageByURL(tt.url)
		if p == nil {
			t.Errorf("PageByURL(%q) = nil", tt.url)
			continue
		}
		if p.Source != tt.source || p.OutPath != tt.out || p.Title != tt.title || p.Description != tt.desc || p.HasH1 != tt.hasH1 {
			t.Errorf("page %s = {Source:%q OutPath:%q Title:%q Desc:%q HasH1:%v}", tt.url, p.Source, p.OutPath, p.Title, p.Description, p.HasH1)
		}
	}

	install := s.PageByURL("/guide/install/")
	if len(install.Headings) != 1 || install.Headings[0].ID != "requirements" {
		t.Errorf("Headings = %#v", install.Headings)
	}
	if !strings.Contains(install.Text, "Requirements") {
		t.Errorf("Text = %q", install.Text)
	}
	if !strings.Contains(string(install.Content), `href="/guide/config/#env"`) ||
		!strings.Contains(string(install.Content), `src="/img/logo.png"`) {
		t.Errorf("links not resolved: %s", install.Content)
	}

	for _, missing := range []string{"/draft/", "/public/old/", "/_partials/snippet/", "/.hidden/secret/", "/nope/"} {
		if p := s.PageByURL(missing); p != nil {
			t.Errorf("PageByURL(%q) = %s, want nil", missing, p.Source)
		}
	}
}

func TestPrevNext(t *testing.T) {
	t.Parallel()
	s := load(t, docsFS(), Options{Exclude: []string{"public"}})
	for i, p := range s.Pages {
		var wantPrev, wantNext *Page
		if i > 0 {
			wantPrev = s.Pages[i-1]
		}
		if i < len(s.Pages)-1 {
			wantNext = s.Pages[i+1]
		}
		if p.Prev != wantPrev || p.Next != wantNext {
			t.Errorf("page %s: prev/next wrong", p.URL)
		}
	}
}

func TestNavLinksPages(t *testing.T) {
	t.Parallel()
	s := load(t, docsFS(), Options{Exclude: []string{"public"}})
	var check func([]*NavItem)
	check = func(items []*NavItem) {
		for _, it := range items {
			if it.Page != nil && it.Page.URL != it.URL {
				t.Errorf("nav %q: URL %q != page URL %q", it.Title, it.URL, it.Page.URL)
			}
			if it.Page == nil && it.URL != "" {
				t.Errorf("nav %q: URL without page", it.Title)
			}
			check(it.Children)
		}
	}
	check(s.Nav)
}

func TestBasePath(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"index.md":     file("# Home\n\n[a](guide/a.md) ![i](img.png) [d](guide/)\n"),
		"guide/a.md":   file("# A\n\n[home](../index.md)\n"),
		"img.png":      file("x"),
		"guide/b.md":   file("# B\n"),
		"guide/c.md":   file("# C\n"),
		"guide/d/x.md": file("# X\n"),
	}
	for _, bp := range []string{"docs", "/docs", "docs/", "/docs/"} {
		s := load(t, fsys, Options{BasePath: bp, Title: "Custom"})
		if s.Title != "Custom" {
			t.Errorf("Title = %q", s.Title)
		}
		if s.Home.URL != "/docs/" || s.Home.OutPath != "index.html" {
			t.Errorf("BasePath %q: home URL %q out %q", bp, s.Home.URL, s.Home.OutPath)
		}
		a := s.PageByURL("/docs/guide/a/")
		if a == nil || a.OutPath != "guide/a/index.html" {
			t.Fatalf("BasePath %q: page a = %+v", bp, a)
		}
		home := string(s.Home.Content)
		for _, want := range []string{`href="/docs/guide/a/"`, `src="/docs/img.png"`, `href="/docs/guide"`} {
			if !strings.Contains(home, want) {
				t.Errorf("BasePath %q: home missing %s: %s", bp, want, home)
			}
		}
		if !strings.Contains(string(a.Content), `href="/docs/"`) {
			t.Errorf("BasePath %q: a content %s", bp, a.Content)
		}
	}
}

func TestLinkResolution(t *testing.T) {
	t.Parallel()

	tests := []struct{ dest, want string }{
		{"sub/s.md", "/guide/sub/s/"},
		{"./sub/s.md#frag", "/guide/sub/s/#frag"},
		{"../other.md?x=1#y", "/other/?x=1#y"},
		{"my%20file.md", "/guide/my%20file/"},
		{"index.md", "/guide/"},
		{"../", "/"},
		{".", "/guide/"},
		{"sub", "/guide/sub"},
		{"missing.md", "/guide/missing.md"},
		{"pic.png", "/guide/pic.png"},
		{"../img/a%20b.png", "/img/a%20b.png"},
		{"../../escape.md", "../../escape.md"},
		{"sub/../../../x.png", "sub/../../../x.png"},
		{"?q=1", "?q=1"},
		{"../.well-known/x.txt", "/.well-known/x.txt"},
		{"../missing-dir/..", "/"},
	}
	var body strings.Builder
	for _, tt := range tests {
		body.WriteString("[x](<" + tt.dest + ">)\n\n")
	}
	fsys := fstest.MapFS{
		"index.md":         file("# Home\n"),
		"guide/index.md":   file("# Guide\n"),
		"guide/page.md":    file(body.String()),
		"guide/my file.md": file("# Spaced\n"),
		"guide/sub/s.md":   file("# S\n"),
		"other.md":         file("# Other\n"),
	}
	content := string(load(t, fsys, Options{}).PageByURL("/guide/page/").Content)
	for _, tt := range tests {
		if !strings.Contains(content, `href="`+tt.want+`"`) {
			t.Errorf("link %q: want href %q in\n%s", tt.dest, tt.want, content)
		}
	}
}

func TestIndexWinsOverReadme(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"README.md":      file("# Readme\n"),
		"Index.md":       file("# Index\n"),
		"docs/readme.md": file("# Docs readme\n"),
	}
	s := load(t, fsys, Options{})
	if s.Home == nil || s.Home.Source != "Index.md" {
		t.Fatalf("Home = %+v", s.Home)
	}
	if got := urls(s.Pages); !reflect.DeepEqual(got, []string{"/", "/docs/"}) {
		t.Errorf("Pages = %q", got)
	}
	if len(s.Assets) != 0 {
		t.Errorf("Assets = %q, want none", s.Assets)
	}
}

func TestDraftIndexFallsBackToReadme(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"index.md":  file("---\ndraft: true\n---\n# Draft\n"),
		"README.md": file("# Readme\n"),
	}
	s := load(t, fsys, Options{})
	if s.Home == nil || s.Home.Source != "README.md" {
		t.Fatalf("Home = %+v", s.Home)
	}
}

func TestTitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fsys      fstest.MapFS
		wantSite  string
		wantNav   string
		wantPages []string
	}{
		{
			name:     "no home uses default title",
			fsys:     fstest.MapFS{"a_b-c.md": file("text")},
			wantSite: "Documentation",
			wantNav:  "A b c(/a_b-c/)",
		},
		{
			name:     "home without h1",
			fsys:     fstest.MapFS{"index.md": file("text")},
			wantSite: "Documentation",
			wantNav:  "",
		},
		{
			name: "dir index without title uses dir name",
			fsys: fstest.MapFS{
				"user-guide/index.md": file("text"),
				"user-guide/x.md":     file("# X"),
			},
			wantSite: "Documentation",
			wantNav:  "User guide(/user-guide/)[X(/user-guide/x/)]",
		},
		{
			name: "order ties broken case-insensitively; dirs interleave",
			fsys: fstest.MapFS{
				"b.md":           file("# beta"),
				"A.md":           file("# Alpha"),
				"c.md":           file("---\norder: -1\n---\n# Charlie"),
				"sub/index.md":   file("---\norder: 5\n---\n# Sub"),
				"sub/one.md":     file("# One"),
				"plain/index.md": file("# Middle"),
				"plain/other.md": file("# Other"),
				"nopage/x.png":   file("x"),
				"empty/_skip.md": file("# skip"),
			},
			wantSite: "Documentation",
			wantNav:  "Charlie(/c/) Alpha(/A/) beta(/b/) Middle(/plain/)[Other(/plain/other/)] Sub(/sub/)[One(/sub/one/)]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := load(t, tt.fsys, Options{})
			if s.Title != tt.wantSite {
				t.Errorf("site title = %q, want %q", s.Title, tt.wantSite)
			}
			if got := navShape(s.Nav); got != tt.wantNav {
				t.Errorf("nav = %s, want %s", got, tt.wantNav)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fsys fs.FS
		want []string
	}{
		{
			name: "duplicate url",
			fsys: fstest.MapFS{"foo.md": file("# a"), "foo/index.md": file("# b")},
			want: []string{"foo.md", "foo/index.md", "/foo/"},
		},
		{
			name: "duplicate index variants",
			fsys: fstest.MapFS{"index.md": file("# a"), "index.markdown": file("# b")},
			want: []string{"index.md", "index.markdown"},
		},
		{
			name: "bad front matter",
			fsys: fstest.MapFS{"guide/bad.md": file("---\ntitle: [x\n---\n")},
			want: []string{"guide/bad.md", "front matter"},
		},
		{
			name: "unreadable file",
			fsys: errFS{fstest.MapFS{"broken.md": file("x")}},
			want: []string{"broken.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(tt.fsys, Options{})
			if err == nil {
				t.Fatal("expected error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q missing %q", err, w)
				}
			}
		})
	}
}

func TestLoadMissingRoot(t *testing.T) {
	t.Parallel()
	if _, err := Load(fstest.MapFS{}, Options{}); err != nil {
		t.Fatalf("empty fs: %v", err)
	}
	_, err := Load(missingFS{}, Options{})
	if err == nil {
		t.Fatal("expected error for unreadable root")
	}
}

// errFS fails to open markdown files while still listing them.
type errFS struct{ fstest.MapFS }

func (e errFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, ".md") {
		return nil, errors.New("boom")
	}
	return e.MapFS.Open(name)
}

func (e errFS) ReadFile(name string) ([]byte, error) {
	if strings.HasSuffix(name, ".md") {
		return nil, errors.New("boom")
	}
	return e.MapFS.ReadFile(name)
}

type missingFS struct{}

func (missingFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func TestWarnings(t *testing.T) {
	t.Parallel()
	s := load(t, fstest.MapFS{
		"index.md":          file("# Home\n"),
		"guide/about.md":    file("# About\n\n## ![](diagram.svg)\n"),
		"guide/diagram.svg": file("<svg/>"),
		"a.md":              file("# A\n\n##\n"),
	}, Options{})
	want := []string{
		"a.md: a level-2 heading has no text",
		`guide/about.md: a level-2 heading has no text; give its image "diagram.svg" alt text`,
	}
	if !slices.Equal(s.Warnings, want) {
		t.Errorf("Warnings = %q, want %q", s.Warnings, want)
	}
}

func TestRootResolver(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"index.md":         file("# Home\n"),
		"guide/index.md":   file("# Guide\n"),
		"guide/install.md": file("# Install\n"),
	}
	resolve := load(t, fsys, Options{BasePath: "docs"}).Resolver()
	for dest, want := range map[string]string{
		"guide/install.md#run": "/docs/guide/install/#run",
		"guide/":               "/docs/guide/",
		"index.md":             "/docs/",
		"img/logo.png":         "/docs/img/logo.png",
		"../up.md":             "../up.md",
	} {
		if got := resolve(dest); got != want {
			t.Errorf("resolve(%q) = %q, want %q", dest, got, want)
		}
	}
	if (&Site{}).Resolver() != nil {
		t.Error("a site that was not loaded has a resolver")
	}
}

func TestLinkWarnings(t *testing.T) {
	t.Parallel()
	base := fstest.MapFS{
		"index.md":             file("# Home\n\n## Intro\n"),
		"guide/index.md":       file("# Guide\n\n## Install\n\n## Setup {#custom-setup}\n"),
		"guide/a.md":           file("# A\n"),
		"guide/my page.md":     file("# Spaced\n\n## Café\n"),
		"guide/draft.md":       file("---\ndraft: true\n---\n# Draft\n"),
		"guide/sub/README.md":  file("# Sub\n"),
		"guide/sub/deep.MD":    file("# Deep\n"),
		"guide/img/logo.png":   file("png"),
		"guide/assets/f.txt":   file("txt"),
		"_partials/p.md":       file("# P\n"),
		"private/secret.md":    file("# Secret\n"),
		".hidden.md":           file("# Hidden\n"),
		"both/index.md":        file("# Both\n"),
		"both/README.md":       file("# Readme\n"),
		"guide/raw.html":       file("<p>raw</p>"),
		"guide/sub/pic.JPG":    file("jpg"),
		"guide/sub/notes.txt":  file("n"),
		"guide/sub/extra.md":   file("# Extra\n"),
		"guide/sub/nested.png": file("png"),
	}
	tests := []struct {
		name, body string
		want       []string // warnings for guide/page.md, without the source prefix
	}{
		{"clean links", strings.Join([]string{
			"[a](a.md)", "[upper](sub/deep.MD)", "[dir](sub/)", "[dir no slash](sub)",
			"[readme](sub/README.md)", "[parent](../index.md)", "[root dir](../)", "[self dir](.)",
			"[spaced](my%20page.md)", "[spaced raw](<my page.md>)", "[query](a.md?x=1)",
			"![logo](img/logo.png)", "[file](assets/f.txt)", "[html](raw.html)",
			"[frag](index.md#install)", "[explicit id](./#custom-setup)", "[encoded frag](my%20page.md#caf%C3%A9)",
			"[home frag](../#intro)", "[query frag](index.md?x=1#install)",
			"[abs](/nowhere.md)", "[web](https://example.com/x.md)", "[mail](mailto:a@b.c)",
			"<a href=\"missing.md\">raw</a>", "[q](?only=query)",
			"## Here\n\n[same](#here) and a note[^1].\n\n[^1]: Footnote.\n\n[fn](#fn:1) [back](#fnref:1)",
		}, "\n\n"), nil},
		{"missing page", "[m](missing.md)", []string{`link to "missing.md" does not match a page`}},
		{"missing page upper ext", "[m](Missing.MARKDOWN)", []string{`link to "Missing.MARKDOWN" does not match a page`}},
		{"draft", "[d](draft.md)", []string{`link to "draft.md" is a draft`}},
		{"underscore", "[p](../_partials/p.md)", []string{`link to "../_partials/p.md" does not match a page`}},
		{"hidden", "[h](../.hidden.md)", []string{`link to "../.hidden.md" does not match a page`}},
		{"excluded", "[s](../private/secret.md)", []string{`link to "../private/secret.md" does not match a page`}},
		{"readme beside index", "[r](../both/README.md)", []string{`link to "../both/README.md" does not match a page`}},
		{"missing image", "![x](img/missing.png)", []string{`link to "img/missing.png" does not match a page or file`}},
		{"folder without page", "[f](assets/)", []string{`link to "assets/" does not match a page or file`}},
		{"missing nested", "[n](sub/none.png?v=2)", []string{`link to "sub/none.png?v=2" does not match a page or file`}},
		{"outside", "[o](../../x.md)", []string{`link to "../../x.md" points outside the docs folder`}},
		{"missing heading", "[h](index.md#nope)", []string{`link to "index.md#nope": the target page has no heading "#nope"`}},
		{"missing heading via dir", "[h](../guide/#nope)", []string{`link to "../guide/#nope": the target page has no heading "#nope"`}},
		{"same page missing heading", "# P\n\n[h](#nope)", []string{`link to "#nope": the target page has no heading "#nope"`}},
		{"missing footnote", "[fn](#fn:9)", []string{`link to "#fn:9": the target page has no heading "#fn:9"`}},
		{"once per page and in order", "[m](missing.md) [o](../../x.md) [m again](missing.md) ![img](missing.md)", []string{
			`link to "missing.md" does not match a page`,
			`link to "../../x.md" points outside the docs folder`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsys := fstest.MapFS{"guide/page.md": file(tt.body)}
			for k, v := range base {
				fsys[k] = v
			}
			s := load(t, fsys, Options{Exclude: []string{"private"}})
			var want []string
			for _, w := range tt.want {
				want = append(want, "guide/page.md: "+w)
			}
			if !slices.Equal(s.Warnings, want) {
				t.Errorf("Warnings = %q, want %q", s.Warnings, want)
			}
		})
	}
}

func TestLinkWarningsFromNestedAndIndexPages(t *testing.T) {
	t.Parallel()
	s := load(t, fstest.MapFS{
		"index.md":           file("# Home\n\n[g](guide/x.md)\n"),
		"guide/README.md":    file("# Guide\n\n[s](sub/a.md#b) [up](../index.md#c)\n"),
		"guide/sub/a.md":     file("# A\n\n## B\n\n[x](../../nope.png)\n"),
		"guide/sub/index.md": file("---\ndraft: true\n---\n"),
	}, Options{})
	want := []string{
		`guide/README.md: link to "../index.md#c": the target page has no heading "#c"`,
		`guide/sub/a.md: link to "../../nope.png" does not match a page or file`,
		`index.md: link to "guide/x.md" does not match a page`,
	}
	if !slices.Equal(s.Warnings, want) {
		t.Errorf("Warnings = %q, want %q", s.Warnings, want)
	}
}

func TestRootResolverRecordsNoWarnings(t *testing.T) {
	t.Parallel()
	s := load(t, fstest.MapFS{"index.md": file("# Home\n")}, Options{})
	s.Resolver()("missing.md")
	s.Resolver()("#nope")
	if len(s.Warnings) != 0 {
		t.Errorf("Warnings = %q", s.Warnings)
	}
}

func TestSkipsNodeModules(t *testing.T) {
	t.Parallel()
	s := load(t, fstest.MapFS{
		"index.md":                      file("# Home\n"),
		"node_modules/pkg/README.md":    file("# Pkg\n"),
		"node_modules/pkg/logo.png":     file("png"),
		"theme/node_modules/x/index.js": file("js"),
		"theme/page.md":                 file("# Theme\n"),
	}, Options{})
	if got := urls(s.Pages); !slices.Equal(got, []string{"/", "/theme/page/"}) {
		t.Errorf("pages = %q", got)
	}
	if len(s.Assets) != 0 {
		t.Errorf("Assets = %q", s.Assets)
	}
}

func TestSymlinks(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	dir := t.TempDir()
	outside := t.TempDir()
	for name, body := range map[string]string{
		"index.md":        "# Home\n\n[linked](linked.md) ![img](pic.png)\n",
		"real/page.md":    "# Real\n",
		"real.png":        "png",
		outside + "/o.md": "# Outside\n",
	} {
		p := name
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, name)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		"linked.md": filepath.Join(outside, "o.md"),
		"pic.png":   "real.png",
		"dirlink":   "real",
		"loop":      ".",
	} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	s := load(t, os.DirFS(dir), Options{})
	if p := s.PageByURL("/linked/"); p == nil || p.Title != "Outside" {
		t.Errorf("linked page = %+v", p)
	}
	if !slices.Equal(s.Assets, []string{"pic.png", "real.png"}) {
		t.Errorf("Assets = %q", s.Assets)
	}
	want := []string{
		"dirlink/ is a symbolic link to a folder; documango skips it",
		"loop/ is a symbolic link to a folder; documango skips it",
	}
	if !slices.Equal(s.Warnings, want) {
		t.Errorf("Warnings = %q, want %q", s.Warnings, want)
	}
}
