// Package markdown renders documango Markdown pages to HTML and extracts the
// metadata the site needs: title, table of contents, and search text.
package markdown

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"unicode"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Heading is an entry in a page's table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// Result is a rendered Markdown document.
type Result struct {
	HTML     template.HTML
	Title    string    // plain text of the first level-1 heading, "" if none
	Headings []Heading // h2 and h3 only, in document order
	Text     string    // whitespace-collapsed plain text for search indexing
	Warnings []string  // problems worth telling the author about, such as a heading with no text
}

// Resolver rewrites a relative link or image destination. It is called only
// for destinations without a scheme that do not start with '/', including
// same-page fragments such as "#install"; the returned value is used verbatim.
//
// Links written as raw HTML (<a href>, <img src>) are not passed to the
// Resolver.
type Resolver func(dest string) string

// Renderer converts Markdown to HTML. It is safe for concurrent use.
type Renderer struct {
	md          goldmark.Markdown
	externalRel string
}

// Option configures a Renderer.
type Option func(*Renderer)

// WithExternalLinkRel sets the rel attribute of links to other sites: http
// and https URLs and protocol-relative ones such as //example.com/.
func WithExternalLinkRel(rel string) Option {
	return func(r *Renderer) { r.externalRel = rel }
}

// New returns a Renderer configured for documango pages.
func New(opts ...Option) *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
				highlighting.WithWrapperRenderer(wrapCodeBlock),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithAttribute(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(nodeRenderer{}, 100)),
		),
	)
	r := &Renderer{md: md}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Render converts src to HTML. resolve may be nil.
func (r *Renderer) Render(src []byte, resolve Resolver) (Result, error) {
	ctx := parser.NewContext(parser.WithIDs(&slugIDs{seen: map[string]bool{}}))
	doc := r.md.Parser().Parse(text.NewReader(src), parser.WithContext(ctx))

	var res Result
	var quotes []*ast.Blockquote
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			id, _ := n.AttributeString("id")
			idStr, _ := id.([]byte)
			// Image alt text names a heading made of an image.
			txt := plainText(n, src, true)
			if txt == "" {
				res.Warnings = append(res.Warnings, emptyHeadingWarning(n))
				break
			}
			if n.Level == 1 && res.Title == "" {
				res.Title = txt
			}
			if n.Level == 2 || n.Level == 3 {
				res.Headings = append(res.Headings, Heading{Level: n.Level, ID: string(idStr), Text: txt})
			}
		case *ast.Blockquote:
			quotes = append(quotes, n)
		case *ast.Link:
			if resolve != nil && isRelative(n.Destination) {
				n.Destination = []byte(resolve(string(n.Destination)))
			}
			r.markExternal(n, n.Destination)
		case *ast.AutoLink:
			if n.AutoLinkType == ast.AutoLinkURL {
				r.markExternal(n, n.URL(src))
			}
		case *ast.Image:
			if resolve != nil && isRelative(n.Destination) {
				n.Destination = []byte(resolve(string(n.Destination)))
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("walk markdown: %w", err)
	}
	for _, q := range quotes {
		convertAlert(q, src)
	}
	res.Text = plainText(doc, src, false)

	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, src, doc); err != nil {
		return Result{}, fmt.Errorf("render markdown: %w", err)
	}
	res.HTML = template.HTML(buf.String())
	return res, nil
}

// markExternal sets the configured rel attribute on link n when dest is on
// another site.
func (r *Renderer) markExternal(n ast.Node, dest []byte) {
	if r.externalRel == "" {
		return
	}
	lower := bytes.ToLower(dest)
	if bytes.HasPrefix(lower, []byte("http://")) || bytes.HasPrefix(lower, []byte("https://")) || bytes.HasPrefix(dest, []byte("//")) {
		n.SetAttributeString("rel", []byte(r.externalRel))
	}
}

var schemeRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

func isRelative(dest []byte) bool {
	return len(dest) > 0 && dest[0] != '/' && !schemeRE.Match(dest)
}

// plainText returns the whitespace-collapsed text content of n, skipping raw
// HTML and inserting spaces between blocks. Image alt text is included only
// when alt is true.
func plainText(n ast.Node, src []byte, alt bool) string {
	var b strings.Builder
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Type() == ast.TypeBlock {
			b.WriteByte(' ')
		}
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Image:
			if !alt {
				return ast.WalkSkipChildren, nil
			}
		case *ast.RawHTML, *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			b.Write(n.Value(src))
			if n.SoftLineBreak() || n.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(n.Value)
		case *ast.AutoLink:
			b.Write(n.Label(src))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines := n.Lines()
			for i := range lines.Len() {
				seg := lines.At(i)
				b.Write(seg.Value(src))
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

// emptyHeadingWarning describes heading h, which has no text, naming its
// first image so the author can find it.
func emptyHeadingWarning(h *ast.Heading) string {
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if img, ok := c.(*ast.Image); ok {
			return fmt.Sprintf("a level-%d heading has no text; give its image %q alt text", h.Level, img.Destination)
		}
	}
	return fmt.Sprintf("a level-%d heading has no text", h.Level)
}

// slugIDs generates GitHub-style heading IDs: lowercase, letters, digits,
// '-' and '_' kept, spaces turned into '-', everything else dropped.
// Duplicates get "-1", "-2", ... suffixes.
type slugIDs struct {
	seen map[string]bool
}

func (s *slugIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	var b strings.Builder
	for _, r := range strings.TrimSpace(string(value)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '-' || r == '_':
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	base := b.String()
	if base == "" {
		base = "heading"
		if kind != ast.KindHeading {
			base = "id"
		}
	}
	id := base
	for i := 1; s.seen[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	s.seen[id] = true
	return []byte(id)
}

func (s *slugIDs) Put(value []byte) {
	s.seen[string(value)] = true
}

var alertTitles = map[string]string{
	"note":      "Note",
	"tip":       "Tip",
	"important": "Important",
	"warning":   "Warning",
	"caution":   "Caution",
}

var alertRE = regexp.MustCompile(`^\[!([A-Za-z]+)\]$`)

// kindCallout is the AST node kind for GitHub-style alerts.
var kindCallout = ast.NewNodeKind("Callout")

type callout struct {
	ast.BaseBlock
	kind string // lowercase alert type, e.g. "note"
}

func (c *callout) Kind() ast.NodeKind { return kindCallout }

func (c *callout) Dump(src []byte, level int) {
	ast.DumpHelper(c, src, level, map[string]string{"Kind": c.kind}, nil)
}

// convertAlert replaces q with a callout when its first line is an alert
// marker such as "[!NOTE]".
func convertAlert(q *ast.Blockquote, src []byte) {
	para, ok := q.FirstChild().(*ast.Paragraph)
	if !ok || para.Lines().Len() == 0 {
		return
	}
	first := para.Lines().At(0)
	m := alertRE.FindSubmatch(bytes.TrimSpace(first.Value(src)))
	if m == nil {
		return
	}
	kind := strings.ToLower(string(m[1]))
	if _, ok := alertTitles[kind]; !ok {
		return
	}

	// Drop the inline nodes that make up the marker line.
	for c := para.FirstChild(); c != nil; {
		t, ok := c.(*ast.Text)
		if !ok || t.Segment.Start >= first.Stop {
			break
		}
		next := c.NextSibling()
		para.RemoveChild(para, c)
		c = next
	}
	if para.ChildCount() == 0 {
		q.RemoveChild(q, para)
	}

	c := &callout{kind: kind}
	for child := q.FirstChild(); child != nil; {
		next := child.NextSibling()
		c.AppendChild(c, child)
		child = next
	}
	q.Parent().ReplaceChild(q.Parent(), q, c)
}

// nodeRenderer renders callouts and headings with self-link anchors.
type nodeRenderer struct{}

func (nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindCallout, renderCallout)
	reg.Register(ast.KindHeading, renderHeading)
}

func renderCallout(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("</div>\n")
		return ast.WalkContinue, nil
	}
	c := n.(*callout)
	_, _ = fmt.Fprintf(w, "<div class=\"callout callout--%s\">\n<p class=\"callout__title\">%s</p>\n",
		c.kind, alertTitles[c.kind])
	return ast.WalkContinue, nil
}

func renderHeading(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if entering {
		_, _ = fmt.Fprintf(w, "<h%d", n.Level)
		if n.Attributes() != nil {
			html.RenderAttributes(w, n, html.HeadingAttributeFilter)
		}
		_ = w.WriteByte('>')
		return ast.WalkContinue, nil
	}
	if id, ok := n.AttributeString("id"); ok {
		if idBytes, ok := id.([]byte); ok {
			_, _ = w.WriteString(`<a class="heading-anchor" href="#`)
			_, _ = w.Write(util.EscapeHTML(util.URLEscape(idBytes, false)))
			_, _ = w.WriteString(`" aria-label="Link to this section" data-pagefind-ignore>#</a>`)
		}
	}
	_, _ = fmt.Fprintf(w, "</h%d>\n", n.Level)
	return ast.WalkContinue, nil
}

// wrapCodeBlock surrounds highlighted code with a container carrying the
// fenced language, and renders unhighlighted code as a plain <pre><code>.
func wrapCodeBlock(w util.BufWriter, ctx highlighting.CodeBlockContext, entering bool) {
	lang, hasLang := ctx.Language()
	if ctx.Highlighted() {
		if entering {
			_, _ = w.WriteString(`<div class="highlight" data-lang="`)
			_, _ = w.Write(util.EscapeHTML(lang))
			_, _ = w.WriteString(`">`)
		} else {
			_, _ = w.WriteString("</div>\n")
		}
		return
	}
	if !entering {
		_, _ = w.WriteString("</code></pre>\n")
		return
	}
	if !hasLang {
		_, _ = w.WriteString("<pre><code>")
		return
	}
	escaped := util.EscapeHTML(lang)
	_, _ = fmt.Fprintf(w, `<pre><code class="language-%s" data-lang="%s">`, escaped, escaped)
}
