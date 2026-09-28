package markdown

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func render(t *testing.T, src string, resolve Resolver) Result {
	t.Helper()
	res, err := New().Render([]byte(src), resolve)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return res
}

func assertContains(t *testing.T, html string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(html, w) {
			t.Errorf("output missing %q\n--- got ---\n%s", w, html)
		}
	}
}

func assertNotContains(t *testing.T, html string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(html, w) {
			t.Errorf("output unexpectedly contains %q\n--- got ---\n%s", w, html)
		}
	}
}

func TestRenderFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		want    []string
		notWant []string
	}{
		{
			name: "table",
			src:  "| a | b |\n|---|---|\n| 1 | 2 |\n",
			want: []string{"<table>", "<th>a</th>", "<td>2</td>"},
		},
		{
			name: "strikethrough",
			src:  "~~gone~~",
			want: []string{"<del>gone</del>"},
		},
		{
			name: "linkify",
			src:  "see https://example.com now",
			want: []string{`<a href="https://example.com">https://example.com</a>`},
		},
		{
			name: "task list",
			src:  "- [x] done\n- [ ] todo\n",
			want: []string{`checked="" disabled="" type="checkbox"`, `disabled="" type="checkbox"`},
		},
		{
			name: "footnote",
			src:  "text[^1]\n\n[^1]: the note\n",
			want: []string{`class="footnote-ref"`, "the note"},
		},
		{
			name: "definition list",
			src:  "Term\n: Definition\n",
			want: []string{"<dl>", "<dt>Term</dt>", "<dd>Definition</dd>"},
		},
		{
			name: "raw html allowed",
			src:  "<div class=\"custom\">hi</div>\n\ninline <kbd>Ctrl</kbd>\n",
			want: []string{`<div class="custom">hi</div>`, "<kbd>Ctrl</kbd>"},
		},
		{
			name: "heading anchor",
			src:  "## Getting Started\n",
			want: []string{
				`<h2 id="getting-started">Getting Started<a class="heading-anchor" href="#getting-started" aria-label="Link to this section" data-pagefind-ignore>#</a></h2>`,
			},
		},
		{
			name: "custom heading id",
			src:  "## Install {#setup}\n",
			want: []string{`<h2 id="setup">Install<a class="heading-anchor" href="#setup"`},
		},
		{
			name: "heading attribute class kept",
			src:  "## Styled {.fancy}\n",
			want: []string{`id="styled"`, `class="fancy"`},
		},
		{
			name: "highlighted code uses classes not inline styles",
			src:  "```go\nfunc main() {}\n```\n",
			want: []string{`class="chroma"`, `data-lang="go"`, `<span class="kd">func</span>`},
			notWant: []string{
				"style=",
			},
		},
		{
			name: "unknown language is escaped plain code",
			src:  "```nosuchlang\n<script>alert(1)</script>\n```\n",
			want: []string{
				`<pre><code class="language-nosuchlang" data-lang="nosuchlang">`,
				"&lt;script&gt;alert(1)&lt;/script&gt;",
			},
			notWant: []string{"<script>"},
		},
		{
			name:    "no language is plain code",
			src:     "```\na < b\n```\n",
			want:    []string{"<pre><code>a &lt; b\n</code></pre>"},
			notWant: []string{"chroma", "data-lang"},
		},
		{
			name: "indented code block",
			src:  "para\n\n    x := 1 < 2\n",
			want: []string{"<pre><code>x := 1 &lt; 2\n</code></pre>"},
		},
		{
			name: "language attribute is escaped",
			src:  "```a\"b\nx\n```\n",
			want: []string{`data-lang="a&quot;b"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			html := string(render(t, tt.src, nil).HTML)
			assertContains(t, html, tt.want...)
			assertNotContains(t, html, tt.notWant...)
		})
	}
}

func TestHeadings(t *testing.T) {
	t.Parallel()

	src := strings.Join([]string{
		"# The *Title*",
		"## Intro",
		"### Details with `code`",
		"#### Too deep",
		"## Intro",
		"## Intro",
		"## Café & Crème",
		"## snake_case name",
		"## Custom {#my-id}",
		"## !!!",
		"# Second h1",
	}, "\n\n")
	res := render(t, src, nil)

	if res.Title != "The Title" {
		t.Errorf("Title = %q, want %q", res.Title, "The Title")
	}
	want := []Heading{
		{Level: 2, ID: "intro", Text: "Intro"},
		{Level: 3, ID: "details-with-code", Text: "Details with code"},
		{Level: 2, ID: "intro-1", Text: "Intro"},
		{Level: 2, ID: "intro-2", Text: "Intro"},
		{Level: 2, ID: "café--crème", Text: "Café & Crème"},
		{Level: 2, ID: "snake_case-name", Text: "snake_case name"},
		{Level: 2, ID: "my-id", Text: "Custom"},
		{Level: 2, ID: "heading", Text: "!!!"},
	}
	if !reflect.DeepEqual(res.Headings, want) {
		t.Errorf("Headings =\n%#v\nwant\n%#v", res.Headings, want)
	}
	assertNotContains(t, res.Title, "#")
	for _, h := range res.Headings {
		assertNotContains(t, h.Text, "#")
	}
	assertContains(t, string(res.HTML), `<h4 id="too-deep">`)
}

func TestNoTitle(t *testing.T) {
	t.Parallel()
	res := render(t, "## Only h2\n\ntext\n", nil)
	if res.Title != "" {
		t.Errorf("Title = %q, want empty", res.Title)
	}
}

func TestSetextHeadingTitle(t *testing.T) {
	t.Parallel()
	res := render(t, "Big Title\n=========\n\nSub\n---\n", nil)
	if res.Title != "Big Title" {
		t.Errorf("Title = %q", res.Title)
	}
	if len(res.Headings) != 1 || res.Headings[0].ID != "sub" {
		t.Errorf("Headings = %#v", res.Headings)
	}
}

func TestSearchText(t *testing.T) {
	t.Parallel()

	src := "# Title\n\nSome *emphasis*   and\nsoft break.\n\n" +
		"```go\nfunc main() {}\n```\n\n" +
		"<div class=\"x\">raw block</div>\n\n" +
		"inline <b>bold</b> tag and `span`.\n\n" +
		"- item one\n- item two\n\n" +
		"visit https://example.com\n\n" +
		"![Acme logo](logo.svg)\n\n" +
		"See [![badge alt](b.svg) the badge](x.md).\n"
	res := render(t, src, nil)

	want := "Title Some emphasis and soft break. func main() {} inline bold tag and span. item one item two visit https://example.com See the badge."
	if res.Text != want {
		t.Errorf("Text =\n%q\nwant\n%q", res.Text, want)
	}
	assertNotContains(t, res.Text, "<", "#", "raw block", "Acme logo", "badge alt")
	if !strings.Contains(string(res.HTML), `alt="Acme logo"`) {
		t.Errorf("HTML lost the image alt text:\n%s", res.HTML)
	}
}

func TestCallouts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		want    []string
		notWant []string
	}{
		{
			name:    "note",
			src:     "> [!NOTE]\n> Useful info.\n",
			want:    []string{`<div class="callout callout--note">`, `<p class="callout__title">Note</p>`, "<p>Useful info.</p>"},
			notWant: []string{"[!NOTE]", "<blockquote>"},
		},
		{
			name: "case insensitive tip",
			src:  "> [!tip]\n> Try this.\n",
			want: []string{`callout--tip`, `>Tip</p>`, "Try this."},
		},
		{
			name: "important",
			src:  "> [!IMPORTANT]\n> Read me.\n",
			want: []string{`callout--important`, `>Important</p>`},
		},
		{
			name: "warning with multiple paragraphs",
			src:  "> [!WARNING]\n> First.\n>\n> Second **bold**.\n",
			want: []string{`callout--warning`, `>Warning</p>`, "<p>First.</p>", "<strong>bold</strong>"},
		},
		{
			name:    "caution marker alone",
			src:     "> [!CAUTION]\n",
			want:    []string{`callout--caution`, `>Caution</p>`},
			notWant: []string{"<p></p>"},
		},
		{
			name:    "marker followed by separate paragraph",
			src:     "> [!NOTE]\n>\n> Body.\n",
			want:    []string{`callout--note`, "<p>Body.</p>"},
			notWant: []string{"<p></p>", "[!NOTE]"},
		},
		{
			name:    "plain blockquote untouched",
			src:     "> just a quote\n",
			want:    []string{"<blockquote>", "just a quote"},
			notWant: []string{"callout"},
		},
		{
			name:    "unknown marker untouched",
			src:     "> [!DANGER]\n> x\n",
			want:    []string{"<blockquote>", "[!DANGER]"},
			notWant: []string{"callout"},
		},
		{
			name:    "marker not alone on first line untouched",
			src:     "> [!NOTE] inline text\n",
			want:    []string{"<blockquote>"},
			notWant: []string{"callout"},
		},
		{
			name:    "blockquote starting with list untouched",
			src:     "> - item\n",
			want:    []string{"<blockquote>"},
			notWant: []string{"callout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := render(t, tt.src, nil)
			html := string(res.HTML)
			assertContains(t, html, tt.want...)
			assertNotContains(t, html, tt.notWant...)
			if strings.Contains(html, `class="callout`) {
				assertNotContains(t, res.Text, "[!")
			}
		})
	}
}

func TestResolver(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seen []string
	resolve := func(dest string) string {
		mu.Lock()
		seen = append(seen, dest)
		mu.Unlock()
		return "/resolved/" + dest
	}

	src := strings.Join([]string{
		"[rel](guide/install.md#step)",
		"[parent](../x.md)",
		"![img](img/logo.png)",
		"[abs](/abs/path)",
		"[frag](#section)",
		"[web](https://example.com/a.md)",
		"[mail](mailto:a@b.c)",
		"[proto](//cdn.example.com/x.js)",
		"<https://auto.example.com>",
		"[ref][r]",
		"",
		"[r]: ref/target.md",
	}, "\n\n")
	html := string(render(t, src, resolve).HTML)

	assertContains(t, html,
		`href="/resolved/guide/install.md#step"`,
		`href="/resolved/../x.md"`,
		`src="/resolved/img/logo.png"`,
		`href="/resolved/ref/target.md"`,
		`href="/abs/path"`,
		`href="/resolved/#section"`,
		`href="https://example.com/a.md"`,
		`href="mailto:a@b.c"`,
		`href="//cdn.example.com/x.js"`,
		`href="https://auto.example.com"`,
	)
	wantSeen := []string{"guide/install.md#step", "../x.md", "img/logo.png", "#section", "ref/target.md"}
	if !reflect.DeepEqual(seen, wantSeen) {
		t.Errorf("resolver called with %q, want %q", seen, wantSeen)
	}
}

func TestResolverNotAppliedToRawHTML(t *testing.T) {
	t.Parallel()
	html := string(render(t, `<a href="page.md">raw</a>`, func(string) string { return "/changed/" }).HTML)
	assertContains(t, html, `href="page.md"`)
}

func TestNilResolverLeavesLinks(t *testing.T) {
	t.Parallel()
	html := string(render(t, "[a](b.md)", nil).HTML)
	assertContains(t, html, `href="b.md"`)
}

func TestConcurrentRender(t *testing.T) {
	t.Parallel()
	r := New()
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src := fmt.Sprintf("# Doc %d\n\n## Part\n\n> [!NOTE]\n> n\n\n```go\nx := %d\n```\n", i, i)
			res, err := r.Render([]byte(src), func(d string) string { return d })
			if err != nil {
				errs <- err
				return
			}
			if res.Title != fmt.Sprintf("Doc %d", i) || len(res.Headings) != 1 || res.Headings[0].ID != "part" {
				errs <- fmt.Errorf("doc %d: unexpected result %+v", i, res)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestImageHeadings(t *testing.T) {
	t.Parallel()

	src := "# ![Acme](logo.svg)\n\n" +
		"## ![Install steps](i.png) Install\n\n" +
		"## ![](diagram.svg)\n\n" +
		"### Usage\n\nBody.\n\n##\n"
	res := render(t, src, nil)

	if res.Title != "Acme" {
		t.Errorf("Title = %q, want the image alt text %q", res.Title, "Acme")
	}
	var got []string
	for _, h := range res.Headings {
		got = append(got, h.Text)
	}
	if want := []string{"Install steps Install", "Usage"}; !slices.Equal(got, want) {
		t.Errorf("Headings = %q, want %q (a heading with no text stays out of the table of contents)", got, want)
	}
	if want := []string{
		`a level-2 heading has no text; give its image "diagram.svg" alt text`,
		"a level-2 heading has no text",
	}; !slices.Equal(res.Warnings, want) {
		t.Errorf("Warnings = %q, want %q", res.Warnings, want)
	}
	assertNotContains(t, res.Text, "Acme", "Install steps")
}

func TestExternalLinkRel(t *testing.T) {
	src := "[Site](https://acme.dev) [Proto](//cdn.acme.dev/x) [Mail](mailto:a@acme.dev) [Page](guide.md) [Top](#top) [Root](/about/) <https://auto.dev> www.bare.dev\n"
	res, err := New(WithExternalLinkRel("noopener")).Render([]byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := string(res.HTML)
	for _, w := range []string{
		`<a href="https://acme.dev" rel="noopener">Site</a>`,
		`<a href="//cdn.acme.dev/x" rel="noopener">Proto</a>`,
		`<a href="https://auto.dev" rel="noopener">https://auto.dev</a>`,
		`<a href="http://www.bare.dev" rel="noopener">www.bare.dev</a>`,
		`<a href="mailto:a@acme.dev">Mail</a>`,
		`<a href="guide.md">Page</a>`,
		`<a href="#top">Top</a>`,
		`<a href="/about/">Root</a>`,
	} {
		if !strings.Contains(got, w) {
			t.Errorf("output lacks %s\n%s", w, got)
		}
	}
	if strings.Contains(got, "target=") {
		t.Errorf("links open a new tab: %s", got)
	}
	if plain := render(t, src, nil); strings.Contains(string(plain.HTML), "rel=") {
		t.Errorf("default renderer adds rel: %s", plain.HTML)
	}
}
