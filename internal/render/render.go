// Package render turns a loaded site into the files of a docs website: one
// HTML page per Markdown page, a 404 page, the site's assets, and the shared
// stylesheet, script, and search index under _documango/.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/desertthunder/documango/internal/markdown"
	"github.com/desertthunder/documango/internal/site"
	"github.com/desertthunder/documango/internal/theme"
)

// Options controls how a site is rendered.
type Options struct {
	// Dark and Light are both required. Pages follow the reader's
	// prefers-color-scheme setting and offer a toggle that is remembered.
	Dark, Light theme.Scheme
	// BasePath is the URL prefix the site is served under. It must match the
	// value given to site.Load; it is normalized the same way ("/" or "/docs/").
	BasePath string
	// LiveReload is the Server-Sent Events URL pages subscribe to for reloads
	// and build errors. Empty disables live reload, as for static builds.
	LiveReload string
	// Version is the documango version reported in <meta name="generator">.
	Version string
}

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
	"components/theme-toggle.css",
	"components/menu-toggle.css",
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

// layoutData is the input to the layout template. Page is nil on the 404 page.
type layoutData struct {
	Site       *site.Site
	Page       *site.Page
	Base       string
	Title      string
	Heading    string // h1 to render above the content; "" when it has one
	Nav        []navNode
	TOC        []markdown.Heading
	LiveReload string
	Version    string
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
	if opts.Dark.Palette[0] == "" {
		return nil, errors.New("render: a dark scheme is required")
	}
	if opts.Light.Palette[0] == "" {
		return nil, errors.New("render: a light scheme is required")
	}

	base := "/"
	if trimmed := strings.Trim(opts.BasePath, "/"); trimmed != "" {
		base = "/" + trimmed + "/"
	}
	files := map[string][]byte{}
	data := layoutData{Site: s, Base: base, LiveReload: opts.LiveReload, Version: opts.Version}

	for _, p := range s.Pages {
		d := data
		d.Page = p
		d.Title = p.Title + " · " + s.Title
		if p == s.Home {
			d.Title = s.Title
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
	fmt.Fprintf(&css, ":root { color-scheme: light; %s }\n", opts.Light.CSSVars())
	fmt.Fprintf(&css, "@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) { color-scheme: dark; %s }\n}\n", opts.Dark.CSSVars())
	fmt.Fprintf(&css, ":root[data-theme=\"dark\"] { color-scheme: dark; %s }\n", opts.Dark.CSSVars())
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
