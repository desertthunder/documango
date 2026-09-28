package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/desertthunder/documango/internal/site"
	"github.com/desertthunder/documango/internal/theme"
)

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

// scheme returns a scheme whose palette entries are prefix followed by the
// slot number, so tests can tell which scheme a CSS variable came from.
func scheme(prefix string) theme.Scheme {
	var s theme.Scheme
	for i := range s.Palette {
		s.Palette[i] = fmt.Sprintf("#%s%02x", prefix, i)
	}
	return s
}

func docsFS() fstest.MapFS {
	return fstest.MapFS{
		"index.md":           file("# Welcome\n\nIntro text.\n"),
		"getting-started.md": file("---\norder: 1\ndescription: Start here\n---\nNo heading.\n\n## Install\n\nRun it.\n\n### From source\n\nBuild it.\n"),
		"guide/index.md":     file("---\norder: 2\n---\n# Guide\n\n## Only section\n"),
		"guide/install.md":   file("# Install\n\n## Requirements\n\n## Steps\n\nSee [the guide](index.md).\n"),
		"reference/api.md":   file("---\ntitle: API <script>alert(1)</script>\n---\nBody & more.\n"),
		"img/logo.png":       &fstest.MapFile{Data: []byte{0x89, 'P', 'N', 'G', 0}},
	}
}

func defaultOpts() Options {
	return Options{Dark: scheme("dd"), Light: scheme("aa"), Version: "1.2.3"}
}

func build(t *testing.T, fsys fstest.MapFS, opts Options) map[string][]byte {
	t.Helper()
	s, err := site.Load(fsys, site.Options{BasePath: opts.BasePath})
	if err != nil {
		t.Fatalf("site.Load: %v", err)
	}
	files, err := Render(s, fsys, opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return files
}

func get(t *testing.T, files map[string][]byte, name string) string {
	t.Helper()
	data, ok := files[name]
	if !ok {
		t.Fatalf("missing output %q", name)
	}
	return string(data)
}

func mustContain(t *testing.T, doc string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(doc, s) {
			t.Errorf("output does not contain %q", s)
		}
	}
}

func mustNotContain(t *testing.T, doc string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(doc, s) {
			t.Errorf("output unexpectedly contains %q", s)
		}
	}
}

// section returns the part of doc from the first occurrence of start up to
// and including the following end.
func section(t *testing.T, doc, start, end string) string {
	t.Helper()
	i := strings.Index(doc, start)
	if i < 0 {
		t.Fatalf("no %q in output", start)
	}
	j := strings.Index(doc[i:], end)
	if j < 0 {
		t.Fatalf("no %q after %q", end, start)
	}
	return doc[i : i+j+len(end)]
}

func TestRenderOutputSet(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())
	want := []string{
		"index.html",
		"getting-started/index.html",
		"guide/index.html",
		"guide/install/index.html",
		"reference/api/index.html",
		"404.html",
		"img/logo.png",
		"_documango/style.css",
		"_documango/app.js",
		"_documango/search.json",
	}
	for _, name := range want {
		if _, ok := files[name]; !ok {
			t.Errorf("missing %q", name)
		}
	}
	if len(files) != len(want) {
		t.Errorf("got %d files, want %d", len(files), len(want))
	}
	if got := files["img/logo.png"]; string(got) != "\x89PNG\x00" {
		t.Errorf("asset not copied verbatim: %q", got)
	}
}

func TestLayoutLandmarks(t *testing.T) {
	doc := get(t, build(t, docsFS(), defaultOpts()), "guide/install/index.html")
	mustContain(t, doc,
		"<!doctype html>",
		`<html lang="en" class="no-js" data-base="/">`,
		`<meta charset="utf-8">`,
		`<meta name="viewport" content="width=device-width, initial-scale=1">`,
		`<meta name="generator" content="documango 1.2.3">`,
		`<link rel="stylesheet" href="/_documango/style.css">`,
		`<script src="/_documango/app.js" defer></script>`,
		`<a class="skip-link" href="#content">Skip to content</a>`,
		`<header class="site-header">`,
		`<a class="site-header__title" href="/">Welcome</a>`,
		`role="search"`,
		`<input class="search__input" id="search-input" type="search"`,
		`aria-controls="sidebar"`,
		`aria-expanded="false"`,
		`<nav class="sidebar-nav" aria-label="Documentation">`,
		`<main class="site-main" id="content" tabindex="-1">`,
		`<article class="prose">`,
		`<footer class="site-footer">`,
		"Built with documango",
	)
	mustNotContain(t, doc, "data-livereload", `<meta name="description"`)
}

func TestDocumentTitles(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())
	tests := map[string]string{
		"index.html":                 "<title>Welcome</title>",
		"guide/install/index.html":   "<title>Install · Welcome</title>",
		"getting-started/index.html": "<title>Getting started · Welcome</title>",
		"404.html":                   "<title>Page not found · Welcome</title>",
	}
	for name, want := range tests {
		mustContain(t, get(t, files, name), want)
	}
	mustContain(t, get(t, files, "getting-started/index.html"), `<meta name="description" content="Start here">`)
}

func TestEscaping(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())
	doc := get(t, files, "reference/api/index.html")
	mustContain(t, doc,
		"<title>API &lt;script&gt;alert(1)&lt;/script&gt; · Welcome</title>",
		`<h1>API &lt;script&gt;alert(1)&lt;/script&gt;</h1>`,
		"Body &amp; more.",
	)
	mustNotContain(t, doc, "<script>alert(1)</script>")
	// The escaped title also appears in other pages' nav links.
	mustNotContain(t, get(t, files, "index.html"), "<script>alert(1)</script>")
}

func TestHeadingOnlyWhenMissing(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())
	gs := get(t, files, "getting-started/index.html")
	mustContain(t, gs, "<h1>Getting started</h1>")

	install := get(t, files, "guide/install/index.html")
	if n := strings.Count(install, "<h1"); n != 1 {
		t.Errorf("install page has %d h1 elements, want 1", n)
	}
}

func TestSidebarNav(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())
	doc := get(t, files, "guide/install/index.html")
	nav := section(t, doc, `<nav class="sidebar-nav"`, "</nav>")

	if n := strings.Count(nav, `aria-current="page"`); n != 1 {
		t.Fatalf("aria-current count = %d, want 1", n)
	}
	mustContain(t, nav,
		`<a class="sidebar-nav__link" href="/guide/install/" aria-current="page">Install</a>`,
		`<a class="sidebar-nav__link" href="/guide/">Guide</a>`,
		`<span class="sidebar-nav__label">Reference</span>`,
		`<a class="sidebar-nav__link" href="/reference/api/">`,
		`<a class="sidebar-nav__link" href="/getting-started/">Getting started</a>`,
	)
	// Guide is the only ancestor of the current page.
	if n := strings.Count(nav, "sidebar-nav__item--active"); n != 1 {
		t.Errorf("active ancestors = %d, want 1", n)
	}
	mustContain(t, section(t, nav, "sidebar-nav__item--active", "</li>"), `href="/guide/"`)

	// Directory pages are current themselves, not ancestors.
	guide := section(t, get(t, files, "guide/index.html"), `<nav class="sidebar-nav"`, "</nav>")
	mustContain(t, guide, `href="/guide/" aria-current="page">Guide</a>`)
	mustNotContain(t, guide, "sidebar-nav__item--active")

	// The home page marks the site title instead of a nav link.
	home := get(t, files, "index.html")
	mustContain(t, home, `<a class="site-header__title" href="/" aria-current="page">Welcome</a>`)
	mustNotContain(t, section(t, home, `<nav class="sidebar-nav"`, "</nav>"), "aria-current")
}

func TestTableOfContents(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())

	gs := get(t, files, "getting-started/index.html")
	toc := section(t, gs, `<nav class="toc" aria-label="On this page">`, "</nav>")
	mustContain(t, toc,
		"On this page",
		`<li class="toc__item"><a class="toc__link" href="#install">Install</a></li>`,
		`<li class="toc__item toc__item--sub"><a class="toc__link" href="#from-source">From source</a></li>`,
	)
	mustNotContain(t, gs, "site-layout--no-toc")

	// One heading is not worth a table of contents.
	guide := get(t, files, "guide/index.html")
	mustNotContain(t, guide, `aria-label="On this page"`)
	mustContain(t, guide, "site-layout--no-toc")
}

func TestPager(t *testing.T) {
	files := build(t, docsFS(), defaultOpts())
	pager := func(name string) string {
		return section(t, get(t, files, name), `<nav class="pager" aria-label="Pagination">`, "</nav>")
	}

	// Nav order: home, Reference/API, Getting started, Guide, Guide/Install.
	home := pager("index.html")
	mustNotContain(t, home, "Previous")
	mustContain(t, home, `href="/reference/api/" rel="next"`, "Next")

	gs := pager("getting-started/index.html")
	mustContain(t, gs,
		`href="/reference/api/" rel="prev"`, "Previous", "API &lt;script&gt;",
		`href="/guide/" rel="next"`, "Next", "Guide",
	)

	last := pager("guide/install/index.html")
	mustContain(t, last, `href="/guide/" rel="prev"`)
	mustNotContain(t, last, "Next")
}

func TestNotFoundPage(t *testing.T) {
	opts := defaultOpts()
	opts.BasePath = "docs"
	files := build(t, docsFS(), opts)
	doc := get(t, files, "404.html")
	mustContain(t, doc,
		"<h1>Page not found</h1>",
		`<a href="/docs/">Go to the home page</a>`,
		`<link rel="stylesheet" href="/docs/_documango/style.css">`,
		`<nav class="sidebar-nav" aria-label="Documentation">`,
	)
	mustNotContain(t, doc, "aria-current", `class="pager"`, `class="toc"`)
}

var urlAttr = regexp.MustCompile(`\b(?:href|src|data-base|data-livereload)="([^"]*)"`)

func TestBasePathPrefixesInternalURLs(t *testing.T) {
	opts := defaultOpts()
	opts.BasePath = "/docs"
	opts.LiveReload = "/docs/_documango/events"
	files := build(t, docsFS(), opts)
	for name, data := range files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		for _, m := range urlAttr.FindAllStringSubmatch(string(data), -1) {
			u := m[1]
			if strings.HasPrefix(u, "#") {
				continue
			}
			if !strings.HasPrefix(u, "/docs/") {
				t.Errorf("%s: URL %q is not under the base path", name, u)
			}
		}
	}
	mustContain(t, get(t, files, "guide/install/index.html"), `href="/docs/guide/"`)
}

func TestPageURLsAreEscaped(t *testing.T) {
	fsys := fstest.MapFS{
		"index.md":         file("# Home\n"),
		"my notes/a b.md":  file("# Spaced\n"),
		"my notes/q&a.md":  file("# QA\n"),
		"my notes/pic.png": file("x"),
	}
	doc := get(t, build(t, fsys, defaultOpts()), "index.html")
	mustContain(t, doc, `href="/my%20notes/a%20b/"`, `href="/my%20notes/q&amp;a/"`)
}

func TestLiveReloadAttribute(t *testing.T) {
	opts := defaultOpts()
	opts.LiveReload = "/_documango/events"
	files := build(t, docsFS(), opts)
	mustContain(t, get(t, files, "index.html"), `data-livereload="/_documango/events"`)
	mustContain(t, get(t, files, "404.html"), `data-livereload="/_documango/events"`)
}

func TestSearchIndex(t *testing.T) {
	long := strings.Repeat("lorem ipsum ", 1000) // 12000 characters
	fsys := docsFS()
	fsys["long.md"] = file("# Long\n\n" + long + "\n")
	files := build(t, fsys, defaultOpts())

	var entries []struct {
		Title    string   `json:"title"`
		URL      string   `json:"url"`
		Headings []string `json:"headings"`
		Text     string   `json:"text"`
	}
	if err := json.Unmarshal(files["_documango/search.json"], &entries); err != nil {
		t.Fatalf("decode search index: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("got %d entries, want 6", len(entries))
	}
	byURL := map[string]int{}
	for i, e := range entries {
		byURL[e.URL] = i
	}

	gs := entries[byURL["/getting-started/"]]
	if gs.Title != "Getting started" {
		t.Errorf("title = %q", gs.Title)
	}
	if strings.Join(gs.Headings, "|") != "Install|From source" {
		t.Errorf("headings = %q", gs.Headings)
	}
	if !strings.Contains(gs.Text, "Build it.") {
		t.Errorf("text = %q", gs.Text)
	}
	if in := entries[byURL["/guide/install/"]]; strings.HasPrefix(in.Text, in.Title) {
		t.Errorf("text repeats the h1 title: %q", in.Text)
	}
	if entries[byURL["/"]].Headings == nil {
		t.Error("headings should encode as an empty array, not null")
	}

	text := entries[byURL["/long/"]].Text
	if n := len([]rune(text)); n > maxSearchText || n < maxSearchText-10 {
		t.Errorf("long text has %d runes, want just under %d", n, maxSearchText)
	}
	if !strings.HasSuffix(text, "lorem") && !strings.HasSuffix(text, "ipsum") {
		t.Errorf("long text not cut on a word boundary: ...%q", text[len(text)-20:])
	}
}

func TestStylesheet(t *testing.T) {
	css := get(t, build(t, docsFS(), defaultOpts()), "_documango/style.css")
	light := scheme("aa").CSSVars()
	dark := scheme("dd").CSSVars()
	mustContain(t, css,
		":root { color-scheme: light; "+light+" }",
		"@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) { color-scheme: dark; "+dark+" }\n}",
		":root[data-theme=\"dark\"] { color-scheme: dark; "+dark+" }",
		theme.SyntaxCSS(),
		".callout--caution",
		".sidebar-nav",
	)
	mustNotContain(t, css, "@import")
	// Theme variables come before the static rules that use them, and syntax
	// rules come last so they win over generic code styles.
	if strings.Index(css, light) > strings.Index(css, "box-sizing") {
		t.Error("theme variables should precede the static stylesheet")
	}
	if strings.Index(css, ".sidebar-nav") > strings.Index(css, ".chroma") {
		t.Error("syntax rules should follow the static stylesheet")
	}
}

func TestEveryStylesheetIsBundled(t *testing.T) {
	listed := map[string]bool{}
	for _, name := range cssFiles {
		listed[name] = true
	}
	err := fs.WalkDir(cssFS, "assets/css", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if name := strings.TrimPrefix(p, "assets/css/"); !listed[name] {
			t.Errorf("%s is not in cssFiles", name)
		}
		delete(listed, strings.TrimPrefix(p, "assets/css/"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name := range listed {
		t.Errorf("cssFiles lists missing file %s", name)
	}
}

func TestScript(t *testing.T) {
	js := get(t, build(t, docsFS(), defaultOpts()), "_documango/app.js")
	mustContain(t, js, "EventSource", "search.json", "localStorage")
}

func TestRenderErrors(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		opts func(*Options)
		src  func(fstest.MapFS) fs.FS
		want string
	}{
		{
			name: "missing dark scheme",
			fsys: docsFS(),
			opts: func(o *Options) { o.Dark = theme.Scheme{} },
			want: "dark scheme",
		},
		{
			name: "missing light scheme",
			fsys: docsFS(),
			opts: func(o *Options) { o.Light = theme.Scheme{} },
			want: "light scheme",
		},
		{
			name: "asset collides with page",
			fsys: fstest.MapFS{"index.md": file("# Home\n"), "index.html": file("<p>raw</p>")},
			want: `asset "index.html"`,
		},
		{
			name: "asset collides with generated file",
			fsys: fstest.MapFS{"index.md": file("# Home\n"), "404.html": file("<p>raw</p>")},
			want: `asset "404.html"`,
		},
		{
			name: "unreadable asset",
			fsys: docsFS(),
			src: func(m fstest.MapFS) fs.FS {
				delete(m, "img/logo.png")
				return m
			},
			want: "img/logo.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOpts()
			if tt.opts != nil {
				tt.opts(&opts)
			}
			s, err := site.Load(tt.fsys, site.Options{})
			if err != nil {
				t.Fatalf("site.Load: %v", err)
			}
			var src fs.FS = tt.fsys
			if tt.src != nil {
				src = tt.src(tt.fsys)
			}
			_, err = Render(s, src, opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want mention of %q", err, tt.want)
			}
		})
	}
}

func TestSiteWithoutHome(t *testing.T) {
	fsys := fstest.MapFS{"a.md": file("# A\n")}
	files := build(t, fsys, defaultOpts())
	mustContain(t, get(t, files, "a/index.html"), `<a class="site-header__title" href="/">Documentation</a>`)
	if _, ok := files["index.html"]; ok {
		t.Error("no home page should be rendered")
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keep, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"index.html":               []byte("home"),
		"guide/install/index.html": []byte("install"),
	}
	if err := Write(files, dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || string(got) != string(want) {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
	info, err := os.Stat(filepath.Join(dir, "guide", "install", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("file mode = %v, want 0644", perm)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("existing file removed: %v", err)
	}
}

func TestWriteError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "guide")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Write(map[string][]byte{"guide/index.html": []byte("x")}, dir)
	if err == nil || !strings.Contains(err.Error(), "guide") {
		t.Fatalf("err = %v, want failure mentioning guide", err)
	}
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Errorf("error should wrap a *fs.PathError, got %T", err)
	}
}

func TestWriteRejectsEscapingPaths(t *testing.T) {
	dir := t.TempDir()
	err := Write(map[string][]byte{"../outside.html": []byte("x")}, dir)
	if err == nil {
		t.Fatal("Write accepted a path outside dir")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "outside.html")); statErr == nil {
		t.Error("file written outside dir")
	}
}
