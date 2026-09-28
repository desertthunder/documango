// Package render turns a loaded site into the files of a docs website: one
// HTML page per Markdown page, a 404 page, the site's assets, and the shared
// stylesheet, script, and search index under _documango/.
package render

import (
	"bytes"
	"cmp"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/desertthunder/documango/internal/markdown"
	"github.com/desertthunder/documango/internal/site"
	"github.com/desertthunder/documango/internal/theme"
)

// Options controls how a site is rendered.
type Options struct {
	// Dark and Light each need at least one scheme; the first is the default
	// for its mode. Pages follow the reader's prefers-color-scheme setting and
	// offer a toggle that is remembered. When a mode has more than one scheme,
	// readers can pick among them; every scheme after the first needs a
	// unique Slug.
	Dark, Light []theme.Scheme
	// BasePath is the URL prefix the site is served under. It must match the
	// value given to site.Load; it is normalized the same way ("/" or "/docs/").
	BasePath string
	// LiveReload is the Server-Sent Events URL pages subscribe to for reloads
	// and build errors. Empty disables live reload, as for static builds.
	LiveReload string
	// Version is the documango version reported in <meta name="generator">.
	Version string

	// Description is the meta description of pages without their own.
	Description string
	// URL is the absolute address of the published site. When set, pages
	// get canonical links and full Open Graph metadata.
	URL      string
	Author   string
	Language string // <html lang>; default "en"
	// Favicon and Logo are asset paths relative to the site root; "" for none.
	Favicon, Logo string
	// Links are shown in the header, in order.
	Links []Link
}

// Link is a header link.
type Link struct{ Title, URL string }

// Output paths of the shared files, relative to the output root.
const (
	stylePath  = "_documango/style.css"
	scriptPath = "_documango/app.js"
	searchPath = "_documango/search.json"
)

// maxSearchText caps the characters of page text stored in the search index.
const maxSearchText = 5000

// cssFiles is the order in which the stylesheets under assets/css are
// concatenated into style.css: reset, tokens, element defaults, page layout,
// then one file per component.
var cssFiles = []string{
	"reset.css",
	"tokens.css",
	"base.css",
	"layout.css",
	"components/skip-link.css",
	"components/site-header.css",
	"components/search.css",
	"components/site-links.css",
	"components/scheme-menu.css",
	"components/theme-toggle.css",
	"components/menu-toggle.css",
	"components/menu-backdrop.css",
	"components/sidebar-nav.css",
	"components/toc.css",
	"components/prose.css",
	"components/callout.css",
	"components/pager.css",
	"components/site-footer.css",
	"components/reload-banner.css",
}

var (
	//go:embed assets/css
	cssFS embed.FS
	//go:embed assets/app.js
	appJS []byte
	//go:embed templates/*.html
	templateFS embed.FS

	templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
)

// navNode is a sidebar entry as seen from one page.
type navNode struct {
	Title    string
	URL      string
	Current  bool // the entry is the page being rendered
	Active   bool // the entry is an ancestor of the page being rendered
	Children []navNode
}

// headerLink is a header link as the template sees it.
type headerLink struct {
	Title, URL string
	External   bool // opens another site
}

// faviconTypes maps favicon file extensions to their media types.
var faviconTypes = map[string]string{
	".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon", ".gif": "image/gif",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".avif": "image/avif",
}

// layoutData is the input to the layout template. Page is nil on the 404 page.
type layoutData struct {
	Site    *site.Site
	Page    *site.Page
	Base    string
	Title   string
	OGTitle string
	// Description is the page's description, else the site's.
	Description string
	// Canonical is the page's absolute URL; "" without a site URL and on the
	// 404 page.
	Canonical   string
	Lang        string
	Author      string
	Favicon     string
	FaviconType string
	Logo        string
	Links       []headerLink
	Heading     string // h1 to render above the content; "" when it has one
	Nav         []navNode
	TOC         []markdown.Heading
	LiveReload  string
	Version     string
	// Dark and Light fill the color scheme menu, shown when either has more
	// than one scheme.
	Dark, Light []theme.Scheme
	SchemeMenu  bool
}

type searchEntry struct {
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Headings []string `json:"headings"`
	Text     string   `json:"text"`
}

// Render renders the site and copies its assets from src, the docs file
// system. Keys of the result are slash paths relative to the output root, in
// the same shape as server.Files.
func Render(s *site.Site, src fs.FS, opts Options) (map[string][]byte, error) {
	if err := checkSchemes("dark", opts.Dark); err != nil {
		return nil, err
	}
	if err := checkSchemes("light", opts.Light); err != nil {
		return nil, err
	}

	base := "/"
	if trimmed := strings.Trim(opts.BasePath, "/"); trimmed != "" {
		base = "/" + trimmed + "/"
	}
	links := make([]headerLink, len(opts.Links))
	for i, l := range opts.Links {
		u, err := url.Parse(l.URL)
		external := err == nil && (u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "" && u.Host != "")
		links[i] = headerLink{Title: l.Title, URL: l.URL, External: external}
	}
	files := map[string][]byte{}
	data := layoutData{
		Site: s, Base: base, LiveReload: opts.LiveReload, Version: opts.Version,
		Dark: opts.Dark, Light: opts.Light, SchemeMenu: len(opts.Dark) > 1 || len(opts.Light) > 1,
		Description: opts.Description, Lang: cmp.Or(opts.Language, "en"), Author: opts.Author,
		Favicon: opts.Favicon, FaviconType: faviconTypes[strings.ToLower(path.Ext(opts.Favicon))],
		Logo: opts.Logo, Links: links,
	}

	for _, p := range s.Pages {
		d := data
		d.Page = p
		d.Title = p.Title + " · " + s.Title
		d.OGTitle = p.Title
		if p == s.Home {
			d.Title, d.OGTitle = s.Title, s.Title
		}
		d.Description = cmp.Or(p.Description, opts.Description)
		if opts.URL != "" {
			d.Canonical = strings.TrimSuffix(opts.URL, "/") + "/" + strings.TrimPrefix(p.URL, base)
		}
		if !p.HasH1 {
			d.Heading = p.Title
		}
		d.Nav, _ = navNodes(s.Nav, p)
		if len(p.Headings) >= 2 {
			d.TOC = p.Headings
		}
		out, err := execute(d)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", p.Source, err)
		}
		files[p.OutPath] = out
	}

	notFound := data
	notFound.Title = "Page not found · " + s.Title
	notFound.Heading = "Page not found"
	notFound.Nav, _ = navNodes(s.Nav, nil)
	out, err := execute(notFound)
	if err != nil {
		return nil, fmt.Errorf("render 404 page: %w", err)
	}
	files["404.html"] = out

	var css strings.Builder
	writeSchemes(&css, opts.Dark, opts.Light)
	for _, name := range cssFiles {
		b, err := cssFS.ReadFile("assets/css/" + name)
		if err != nil {
			return nil, fmt.Errorf("read stylesheet %s: %w", name, err)
		}
		css.WriteString("\n/* " + name + " */\n")
		css.Write(b)
	}
	css.WriteString("\n/* syntax highlighting */\n")
	css.WriteString(theme.SyntaxCSS())
	files[stylePath] = []byte(css.String())
	files[scriptPath] = appJS

	index := make([]searchEntry, len(s.Pages))
	for i, p := range s.Pages {
		headings := make([]string, len(p.Headings))
		for j, h := range p.Headings {
			headings[j] = h.Text
		}
		// The title is indexed on its own; drop the h1 that repeats it.
		text := p.Text
		if p.HasH1 {
			text = strings.TrimSpace(strings.TrimPrefix(text, p.Title))
		}
		if runes := []rune(text); len(runes) > maxSearchText {
			text = string(runes[:maxSearchText])
			if cut := strings.LastIndexByte(text, ' '); cut > 0 {
				text = text[:cut]
			}
		}
		index[i] = searchEntry{Title: p.Title, URL: p.URL, Headings: headings, Text: text}
	}
	if files[searchPath], err = json.Marshal(index); err != nil {
		return nil, fmt.Errorf("encode search index: %w", err)
	}

	for _, name := range s.Assets {
		if _, ok := files[name]; ok {
			return nil, fmt.Errorf("asset %q collides with a generated file", name)
		}
		b, err := fs.ReadFile(src, name)
		if err != nil {
			return nil, fmt.Errorf("copy asset: %w", err)
		}
		files[name] = b
	}
	return files, nil
}

func checkSchemes(mode string, schemes []theme.Scheme) error {
	if len(schemes) == 0 {
		return fmt.Errorf("render: a %s scheme is required", mode)
	}
	seen := map[string]bool{}
	for i, sc := range schemes {
		if sc.Palette[0] == "" {
			return fmt.Errorf("render: %s scheme %q has no colors", mode, sc.Slug)
		}
		if i == 0 {
			seen[sc.Slug] = true
			continue
		}
		if sc.Slug == "" {
			return fmt.Errorf("render: %s scheme %d has no name", mode, i+1)
		}
		if seen[sc.Slug] {
			return fmt.Errorf("render: %s scheme %q is listed twice", mode, sc.Slug)
		}
		seen[sc.Slug] = true
	}
	return nil
}

// writeSchemes writes the color variables of every scheme. The defaults
// follow prefers-color-scheme and the data-theme toggle. The others apply
// through data-light-scheme and data-dark-scheme, which the page sets from
// the reader's choice; they come after the defaults of their mode, and dark
// rules outrank light ones, so each only applies in its own mode.
func writeSchemes(w *strings.Builder, dark, light []theme.Scheme) {
	fmt.Fprintf(w, ":root { color-scheme: light; %s }\n", schemeVars(light[0]))
	for _, s := range light[1:] {
		fmt.Fprintf(w, ":root[data-light-scheme=%s] { color-scheme: light; %s }\n", cssString(s.Slug), schemeVars(s))
	}
	const system, toggled = `:root:not([data-theme="light"])`, `:root[data-theme="dark"]`
	fmt.Fprintf(w, "@media (prefers-color-scheme: dark) {\n  %s { color-scheme: dark; %s }\n", system, schemeVars(dark[0]))
	for _, s := range dark[1:] {
		fmt.Fprintf(w, "  %s[data-dark-scheme=%s] { color-scheme: dark; %s }\n", system, cssString(s.Slug), schemeVars(s))
	}
	w.WriteString("}\n")
	fmt.Fprintf(w, "%s { color-scheme: dark; %s }\n", toggled, schemeVars(dark[0]))
	for _, s := range dark[1:] {
		fmt.Fprintf(w, "%s[data-dark-scheme=%s] { color-scheme: dark; %s }\n", toggled, cssString(s.Slug), schemeVars(s))
	}
}

// cssString quotes s as a CSS string, escaping everything but ASCII letters,
// digits, '-' and '_' so any slug is safe inside a selector.
func cssString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "\\%x ", r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// navNodes converts items to their view from page cur and reports whether
// cur is one of them or their descendants.
func navNodes(items []*site.NavItem, cur *site.Page) ([]navNode, bool) {
	nodes := make([]navNode, len(items))
	found := false
	for i, it := range items {
		children, inside := navNodes(it.Children, cur)
		n := navNode{Title: it.Title, URL: it.URL, Children: children, Active: inside}
		n.Current = cur != nil && it.Page == cur
		nodes[i] = n
		found = found || inside || n.Current
	}
	return nodes, found
}

func execute(d layoutData) ([]byte, error) {
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, "layout.html", d); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Write writes files under dir, creating directories as needed. It does not
// delete anything already in dir.
func Write(files map[string][]byte, dir string) error {
	for name, data := range files {
		if !fs.ValidPath(name) {
			return fmt.Errorf("write %q: invalid output path", name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
