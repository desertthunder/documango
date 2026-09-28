// Package site loads a directory of Markdown files into an in-memory docs
// site: pages with pretty URLs, a navigation tree, and assets to copy.
package site

import (
	"cmp"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/desertthunder/documango/internal/frontmatter"
	"github.com/desertthunder/documango/internal/markdown"
)

// Options controls how a site is loaded.
type Options struct {
	Title    string   // site title; defaults to the home page title, else "Documentation"
	BasePath string   // URL prefix, normalized to start and end with "/" (default "/")
	Exclude  []string // slash-separated paths relative to the fs root to skip entirely
}

// Page is a single rendered Markdown document.
type Page struct {
	Source      string // slash path relative to the fs root, e.g. "guide/install.md"
	URL         string // absolute URL including BasePath, e.g. "/guide/install/"
	OutPath     string // output file relative to the build dir, e.g. "guide/install/index.html"
	Meta        frontmatter.Meta
	Title       string // Meta.Title, else the first h1, else a humanized file or directory name
	HasH1       bool   // Content already contains an h1
	Description string
	Content     template.HTML
	Headings    []markdown.Heading
	Text        string
	Prev, Next  *Page // neighbours in flattened nav order; nil at the ends
}

// NavItem is a node in the navigation tree.
type NavItem struct {
	Title    string
	URL      string // "" for a directory without an index page
	Page     *Page  // nil for a directory without an index page
	Children []*NavItem
}

// Site is a loaded docs directory.
type Site struct {
	Title string
	// Pages lists every page in flattened nav order, home first.
	Pages []*Page
	// Nav is the top level of the navigation tree. The home page is not part
	// of it: templates link to it through the site title.
	Nav []*NavItem
	// Assets lists the slash paths of non-Markdown files to copy verbatim, sorted.
	Assets []string
	// Home is the root index page, or nil if there is none.
	Home *Page
	// Warnings lists problems found while loading, each prefixed with the
	// source file, such as a heading with no text.
	Warnings []string

	byURL map[string]*Page
}

// PageByURL returns the page whose URL is exactly u, or nil.
func (s *Site) PageByURL(u string) *Page {
	return s.byURL[u]
}

// dirNode groups the pages of one directory while building the nav.
type dirNode struct {
	path    string
	index   *Page
	pages   []*Page
	subdirs []*dirNode
}

type loader struct {
	basePath string
	bySource map[string]*Page
	byDir    map[string]*Page // directory path ("." for root) -> index page
}

// Load reads every Markdown file in fsys and builds the site model.
//
// Hidden entries, entries starting with "_", excluded paths, and drafts are
// skipped. index.md (or README.md when there is no index.md) becomes its
// directory's page.
func Load(fsys fs.FS, opts Options) (*Site, error) {
	l := &loader{
		basePath: normalizeBasePath(opts.BasePath),
		bySource: map[string]*Page{},
		byDir:    map[string]*Page{},
	}

	exclude := map[string]bool{}
	for _, e := range opts.Exclude {
		exclude[path.Clean(strings.Trim(e, "/"))] = true
	}

	var sources []string
	s := &Site{byURL: map[string]*Page{}}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") || exclude[p] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case d.IsDir():
		case isMarkdown(p):
			sources = append(sources, p)
		default:
			s.Assets = append(s.Assets, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan docs: %w", err)
	}
	slices.Sort(s.Assets)

	// Parse front matter first so drafts are gone before index pages are chosen.
	bodies := map[*Page][]byte{}
	var pages []*Page
	for _, src := range sources {
		data, err := fs.ReadFile(fsys, src)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src, err)
		}
		meta, body, err := frontmatter.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src, err)
		}
		if meta.Draft {
			continue
		}
		p := &Page{Source: src, Meta: meta, Description: meta.Description}
		bodies[p] = body
		pages = append(pages, p)
	}

	hasIndex := map[string]bool{}
	for _, p := range pages {
		if stem(p.Source) == "index" {
			hasIndex[path.Dir(p.Source)] = true
		}
	}
	pages = slices.DeleteFunc(pages, func(p *Page) bool {
		return stem(p.Source) == "readme" && hasIndex[path.Dir(p.Source)]
	})

	for _, p := range pages {
		dir := path.Dir(p.Source)
		rel := strings.TrimSuffix(path.Base(p.Source), path.Ext(p.Source))
		if isIndex(p.Source) {
			rel = ""
			l.byDir[dir] = p
		}
		rel = path.Join(dir, rel)
		if rel == "." {
			p.URL, p.OutPath = l.basePath, "index.html"
		} else {
			p.URL, p.OutPath = l.basePath+rel+"/", rel+"/index.html"
		}
		if prev, ok := s.byURL[p.URL]; ok {
			return nil, fmt.Errorf("duplicate page URL %q: %s and %s", p.URL, prev.Source, p.Source)
		}
		s.byURL[p.URL] = p
		l.bySource[p.Source] = p
	}

	md := markdown.New()
	for _, p := range pages {
		res, err := md.Render(bodies[p], l.resolver(p))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Source, err)
		}
		p.Content, p.Headings, p.Text = res.HTML, res.Headings, res.Text
		for _, w := range res.Warnings {
			s.Warnings = append(s.Warnings, p.Source+": "+w)
		}
		p.HasH1 = res.Title != ""
		p.Title = cmp.Or(p.Meta.Title, res.Title)
		if p.Title == "" {
			name := path.Base(p.Source)
			if isIndex(p.Source) {
				name = path.Base(path.Dir(p.Source))
			}
			if name == "." {
				name = "home"
			}
			p.Title = humanize(strings.TrimSuffix(name, path.Ext(name)))
		}
	}

	// Build the directory tree, creating ancestors of every page's directory.
	nodes := map[string]*dirNode{".": {path: "."}}
	var nodeFor func(dir string) *dirNode
	nodeFor = func(dir string) *dirNode {
		if n, ok := nodes[dir]; ok {
			return n
		}
		n := &dirNode{path: dir}
		nodes[dir] = n
		parent := nodeFor(path.Dir(dir))
		parent.subdirs = append(parent.subdirs, n)
		return n
	}
	for _, p := range pages {
		n := nodeFor(path.Dir(p.Source))
		if isIndex(p.Source) {
			n.index = p
		} else {
			n.pages = append(n.pages, p)
		}
	}

	root := nodes["."]
	s.Home = root.index
	s.Nav = navChildren(root)
	if s.Home != nil {
		s.Pages = append(s.Pages, s.Home)
	}
	var flatten func([]*NavItem)
	flatten = func(items []*NavItem) {
		for _, it := range items {
			if it.Page != nil {
				s.Pages = append(s.Pages, it.Page)
			}
			flatten(it.Children)
		}
	}
	flatten(s.Nav)
	for i, p := range s.Pages {
		if i > 0 {
			p.Prev = s.Pages[i-1]
		}
		if i < len(s.Pages)-1 {
			p.Next = s.Pages[i+1]
		}
	}

	s.Title = opts.Title
	// A home page titled only by the "Home" fallback does not name the site.
	if s.Title == "" && s.Home != nil && (s.Home.Meta.Title != "" || s.Home.HasH1) {
		s.Title = s.Home.Title
	}
	s.Title = cmp.Or(s.Title, "Documentation")
	return s, nil
}

// navChildren returns the sorted nav items for a directory's pages and
// subdirectories, ordered by (Order, lowercase title, URL).
func navChildren(n *dirNode) []*NavItem {
	type entry struct {
		order int
		item  *NavItem
	}
	var entries []entry
	for _, p := range n.pages {
		entries = append(entries, entry{p.Meta.Order, &NavItem{Title: p.Title, URL: p.URL, Page: p}})
	}
	for _, sub := range n.subdirs {
		item := &NavItem{Title: humanize(path.Base(sub.path)), Children: navChildren(sub)}
		order := 0
		if sub.index != nil {
			item.Title, item.URL, item.Page = sub.index.Title, sub.index.URL, sub.index
			order = sub.index.Meta.Order
		}
		entries = append(entries, entry{order, item})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return cmp.Or(
			cmp.Compare(a.order, b.order),
			cmp.Compare(strings.ToLower(a.item.Title), strings.ToLower(b.item.Title)),
			cmp.Compare(a.item.URL, b.item.URL),
		)
	})
	items := make([]*NavItem, len(entries))
	for i, e := range entries {
		items[i] = e.item
	}
	return items
}

// resolver maps relative Markdown links on page p to site URLs. Targets are
// resolved against p's source directory; known pages and page directories map
// to their URLs, anything else to BasePath plus the cleaned path. Paths that
// escape the root are returned unchanged.
func (l *loader) resolver(p *Page) markdown.Resolver {
	dir := path.Dir(p.Source)
	return func(dest string) string {
		raw, suffix := dest, ""
		if i := strings.IndexAny(dest, "?#"); i >= 0 {
			raw, suffix = dest[:i], dest[i:]
		}
		if raw == "" {
			return dest
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil {
			decoded = raw
		}
		target := path.Join(dir, decoded)
		if target == ".." || strings.HasPrefix(target, "../") {
			return dest
		}
		if page, ok := l.bySource[target]; ok {
			return page.URL + suffix
		}
		if page, ok := l.byDir[target]; ok {
			return page.URL + suffix
		}
		fallback := path.Join(dir, raw)
		if fallback == ".." || strings.HasPrefix(fallback, "../") {
			return dest
		}
		if fallback == "." {
			fallback = ""
		}
		return l.basePath + fallback + suffix
	}
}

func normalizeBasePath(bp string) string {
	bp = strings.Trim(bp, "/")
	if bp == "" {
		return "/"
	}
	return "/" + bp + "/"
}

func isMarkdown(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	return ext == ".md" || ext == ".markdown"
}

// stem returns the lowercase file name without its extension.
func stem(p string) string {
	base := path.Base(p)
	return strings.ToLower(strings.TrimSuffix(base, path.Ext(base)))
}

func isIndex(p string) bool {
	s := stem(p)
	return s == "index" || s == "readme"
}

// humanize turns a file name like "getting-started" into "Getting started".
func humanize(name string) string {
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	}), " ")
	r, size := utf8.DecodeRuneInString(name)
	if size == 0 {
		return name
	}
	return string(unicode.ToUpper(r)) + name[size:]
}
