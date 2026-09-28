// Package site loads a directory of Markdown files into an in-memory docs
// site: pages with pretty URLs, a navigation tree, and assets to copy.
package site

import (
	"cmp"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"net/url"
	"path"
	"regexp"
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
	// Include lists patterns of files to publish even when no page links to
	// them: slash-separated paths relative to the fs root, matched segment by
	// segment with path.Match, where a final "/**" matches everything under
	// a folder. A "*" never matches a name starting with "." or "_"; such a
	// segment is matched only by writing it out.
	Include []string
	// Publish lists files published even when no page links to them, such as
	// the favicon and logo.
	Publish []string
	// Footer is the Markdown shown on every page; files it links to are published.
	Footer string
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
	// Generated is set on the contents page documango makes when the docs
	// folder has no home page. It has no Source and is not searchable.
	Generated bool
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
	// Assets lists the slash paths of files to copy verbatim, sorted: files a
	// page or the footer links to, Options.Publish and Options.Include files,
	// and well-known site files such as CNAME at the root.
	Assets []string
	// Skipped lists the slash paths of the other non-Markdown files, sorted.
	// Files in folders starting with "." or "_" are not listed.
	Skipped []string
	// Home is the root index page. Without one, it is a generated contents
	// page; it is nil only when there are no pages.
	Home *Page
	// Warnings lists problems found while loading, each prefixed with the
	// source file, such as a heading with no text or a broken relative link.
	Warnings []string

	byURL   map[string]*Page
	resolve markdown.Resolver
}

// PageByURL returns the page whose URL is exactly u, or nil.
func (s *Site) PageByURL(u string) *Page {
	return s.byURL[u]
}

// Resolver maps relative links in Markdown shown outside the pages, such as
// the footer, to site URLs. Links resolve as they would in a page at the root
// of the docs folder. It is nil for a Site that was not loaded.
func (s *Site) Resolver() markdown.Resolver {
	return s.resolve
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
	drafts   map[string]bool  // sources of draft pages
	assets   map[string]bool  // files that may be published
	linked   map[string]bool  // assets a page or the footer links to
	links    map[*Page][]link // relative links in each page, in document order
}

// siteFiles are the well-known files published from the root of the docs
// folder even when no page links to them, as Include patterns.
var siteFiles = []string{
	"CNAME", "robots.txt", "favicon.ico", "humans.txt", ".nojekyll", "_headers", "_redirects", ".well-known/**",
}

// link is a relative link or image in a page, checked once every page is
// rendered and its anchors are known.
type link struct {
	dest    string
	problem string // why the target isn't in the site, or ""
	target  *Page  // the page dest points to, if any
	frag    string // the #fragment without '#', or ""
}

// Load reads every Markdown file in fsys and builds the site model.
//
// Hidden entries, entries starting with "_", node_modules folders, excluded
// paths, drafts, and symbolic links to folders are skipped, apart from files
// matched by Options.Include and the well-known site files. index.md (or
// README.md when there is no index.md) becomes its directory's page.
func Load(fsys fs.FS, opts Options) (*Site, error) {
	l := &loader{
		basePath: normalizeBasePath(opts.BasePath),
		bySource: map[string]*Page{},
		byDir:    map[string]*Page{},
		drafts:   map[string]bool{},
		assets:   map[string]bool{},
		linked:   map[string]bool{},
		links:    map[*Page][]link{},
	}
	include := make([][]string, 0, len(siteFiles)+len(opts.Include))
	for _, p := range slices.Concat(siteFiles, opts.Include) {
		include = append(include, strings.Split(p, "/"))
	}

	exclude := map[string]bool{}
	for _, e := range opts.Exclude {
		exclude[path.Clean(strings.Trim(e, "/"))] = true
	}

	var sources, files []string
	s := &Site{byURL: map[string]*Page{}}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		name := d.Name()
		if name == "node_modules" || exclude[p] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Hidden paths hold only the files that include names.
		segs := strings.Split(p, "/")
		if slices.ContainsFunc(segs, hidden) {
			switch {
			case d.IsDir() && !slices.ContainsFunc(include, func(pat []string) bool { return entersDir(pat, segs) }):
				return fs.SkipDir
			case !d.IsDir() && !isMarkdown(p) && slices.ContainsFunc(include, func(pat []string) bool { return matchPath(pat, segs) }):
				files = append(files, p)
			}
			return nil
		}
		// WalkDir doesn't follow symbolic links. Following one to a folder
		// could loop, so skip it; a link to a file reads like the file.
		if d.Type()&fs.ModeSymlink != 0 {
			if info, err := fs.Stat(fsys, p); err == nil && info.IsDir() {
				s.Warnings = append(s.Warnings, p+"/ is a symbolic link to a folder; documango skips it")
				return nil
			}
		}
		switch {
		case d.IsDir():
		case isMarkdown(p):
			sources = append(sources, p)
		default:
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan docs: %w", err)
	}
	for _, f := range files {
		l.assets[f] = true
	}

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
			l.drafts[src] = true
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

	for _, p := range pages {
		for _, w := range l.linkWarnings(p) {
			s.Warnings = append(s.Warnings, p.Source+": "+w)
		}
	}
	if opts.Footer != "" {
		// Rendered only to find the files it links to.
		if _, err := md.Render([]byte(opts.Footer), l.resolver(nil)); err != nil {
			return nil, fmt.Errorf("footer: %w", err)
		}
	}
	for _, p := range opts.Publish {
		l.linked[p] = true
	}
	for _, f := range files {
		segs := strings.Split(f, "/")
		if l.linked[f] || slices.ContainsFunc(include, func(pat []string) bool { return matchPath(pat, segs) }) {
			s.Assets = append(s.Assets, f)
		} else {
			s.Skipped = append(s.Skipped, f)
		}
	}
	slices.Sort(s.Assets)
	slices.Sort(s.Skipped)

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
	s.resolve = l.resolver(nil)
	s.Nav = navChildren(root)
	s.Title = opts.Title
	// A home page titled only by the "Home" fallback does not name the site.
	if s.Title == "" && s.Home != nil && (s.Home.Meta.Title != "" || s.Home.HasH1) {
		s.Title = s.Home.Title
	}
	s.Title = cmp.Or(s.Title, "Documentation")
	if s.Home == nil && len(pages) > 0 {
		var content strings.Builder
		if err := contentsTemplate.Execute(&content, s.Nav); err != nil {
			return nil, fmt.Errorf("contents page: %w", err)
		}
		s.Home = &Page{URL: l.basePath, OutPath: "index.html", Title: s.Title, Content: template.HTML(content.String()), Generated: true}
		s.byURL[s.Home.URL] = s.Home
		s.Warnings = append(s.Warnings, "no index.md or README.md at the top of the docs folder; documango generated a contents page")
	}
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

	return s, nil
}

// contentsTemplate renders the nav tree as nested lists of links for the
// generated contents page.
var contentsTemplate = template.Must(template.New("list").Parse(
	`<ul>{{range .}}<li>{{if .URL}}<a href="{{.URL}}">{{.Title}}</a>{{else}}{{.Title}}{{end}}` +
		`{{with .Children}}{{template "list" .}}{{end}}</li>{{end}}</ul>`))

// hidden reports whether a path segment names a hidden or "_" entry.
func hidden(seg string) bool {
	return strings.HasPrefix(seg, ".") || strings.HasPrefix(seg, "_")
}

// matchSegment matches one path segment against one pattern segment. Hidden
// segments match only when the pattern names them exactly.
func matchSegment(pat, seg string) bool {
	if hidden(seg) {
		return pat == seg
	}
	ok, _ := path.Match(pat, seg)
	return ok
}

// matchPath reports whether the split file path segs matches the split
// Include pattern pat.
func matchPath(pat, segs []string) bool {
	if n := len(pat); n > 0 && pat[n-1] == "**" {
		return len(segs) >= n && entersDir(pat, segs[:len(segs)-1]) && !hidden(segs[len(segs)-1])
	}
	return len(segs) == len(pat) && entersDir(pat, segs[:len(segs)-1]) && matchSegment(pat[len(pat)-1], segs[len(segs)-1])
}

// entersDir reports whether files matching the split Include pattern pat may
// lie in the folder whose split path is dir.
func entersDir(pat, dir []string) bool {
	prefix, all := pat, false
	if n := len(pat); n > 0 && pat[n-1] == "**" {
		prefix, all = pat[:n-1], true
	}
	if !all && len(dir) >= len(prefix) {
		return false
	}
	for i, seg := range dir {
		if i < len(prefix) && !matchSegment(prefix[i], seg) || i >= len(prefix) && hidden(seg) {
			return false
		}
	}
	return true
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

// resolver maps relative Markdown links written in page from to site URLs,
// and records them for linkWarnings. A nil from resolves links as if written
// at the root and records nothing. Known pages and page directories map to
// their URLs, anything else to BasePath plus the cleaned path. Paths that
// escape the root are returned unchanged.
func (l *loader) resolver(from *Page) markdown.Resolver {
	dir := "."
	if from != nil {
		dir = path.Dir(from.Source)
	}
	record := func(dest, problem string, target *Page, suffix string) {
		if from == nil {
			return
		}
		_, frag, _ := strings.Cut(suffix, "#")
		l.links[from] = append(l.links[from], link{dest: dest, problem: problem, target: target, frag: frag})
	}
	return func(dest string) string {
		raw, suffix := dest, ""
		if i := strings.IndexAny(dest, "?#"); i >= 0 {
			raw, suffix = dest[:i], dest[i:]
		}
		if raw == "" {
			record(dest, "", from, suffix)
			return dest
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil {
			decoded = raw
		}
		target := path.Join(dir, decoded)
		if escapes(target) {
			record(dest, "points outside the docs folder", nil, "")
			return dest
		}
		page := l.bySource[target]
		if page == nil {
			page = l.byDir[target]
		}
		// A link without an extension, as other docs tools write them, may
		// name a Markdown page.
		extensionless := page == nil && !l.assets[target] && path.Ext(target) == ""
		if extensionless {
			page = cmp.Or(l.bySource[target+".md"], l.bySource[target+".markdown"])
		}
		if page != nil {
			record(dest, "", page, suffix)
			return page.URL + suffix
		}
		switch {
		case l.drafts[target] || extensionless && (l.drafts[target+".md"] || l.drafts[target+".markdown"]):
			record(dest, "is a draft", nil, "")
		case isMarkdown(target):
			record(dest, "does not match a page", nil, "")
		case !l.assets[target]:
			record(dest, "does not match a page or file", nil, "")
		default:
			l.linked[target] = true
		}
		fallback := path.Join(dir, raw)
		if escapes(fallback) {
			return dest
		}
		if fallback == "." {
			fallback = ""
		}
		return l.basePath + fallback + suffix
	}
}

// idRE matches id attributes in rendered HTML: heading IDs, footnote
// anchors, and ids written in raw HTML.
var idRE = regexp.MustCompile(`\sid=(?:"([^"]*)"|'([^']*)')`)

// linkWarnings describes page p's broken relative links, once each and in
// document order. It runs after every page is rendered, when heading IDs are
// known.
func (l *loader) linkWarnings(p *Page) []string {
	var out []string
	seen := map[string]bool{}
	ids := map[*Page]map[string]bool{}
	for _, lk := range l.links[p] {
		msg := lk.problem
		if msg == "" && lk.frag != "" {
			if ids[lk.target] == nil {
				ids[lk.target] = pageIDs(lk.target)
			}
			frag, err := url.PathUnescape(lk.frag)
			if err != nil {
				frag = lk.frag
			}
			if !ids[lk.target][lk.frag] && !ids[lk.target][frag] {
				msg = fmt.Sprintf("the target page has no heading %q", "#"+lk.frag)
			}
		}
		if msg == "" {
			continue
		}
		sep := " "
		if lk.problem == "" {
			sep = ": "
		}
		w := fmt.Sprintf("link to %q%s%s", lk.dest, sep, msg)
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// pageIDs returns the set of element ids in p's rendered content.
func pageIDs(p *Page) map[string]bool {
	ids := map[string]bool{}
	for _, m := range idRE.FindAllStringSubmatch(string(p.Content), -1) {
		ids[html.UnescapeString(m[1]+m[2])] = true
	}
	return ids
}

// escapes reports whether the cleaned slash path p leaves the root.
func escapes(p string) bool {
	return p == ".." || strings.HasPrefix(p, "../")
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
