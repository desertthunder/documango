/*
package build creates in-memory HTML documents for use by
the server & build commands.

In its simplest form, our View type contains a reference
to the contents of a markdown file and contains implementations
for methods that create a document using one of the following:

 1. a template in its frontmatter
 2. a template with the same name as the file (sans extensions)
 3. the base template

Then executes (renders) the template by placing it in some stream,
be it file, stdout or stderr.
*/
package build

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/desertthunder/documango/cmd/libs"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var Caser = cases.Title(language.AmericanEnglish)

type View struct {
	Path         string
	front        *Frontmatter
	content      []byte
	html_content []byte
	html_page    []byte
	templateDir  string
	templ        *template.Template
	links        []*NavLink
}

// function NewView is a constructor for a View
// taking a path (filename), its file contents, and
// template directory.
func NewView(p string, c []byte, t string) View {
	return View{Path: p, content: c, templateDir: t, links: []*NavLink{}}
}

// function WithNavigation populates a NavLink
// list in the View struct to build context when
// rendering the layout
func WithNavigation(views []*View) []*View {
	links := make([]*NavLink, len(views))
	for i, v := range views {
		path := strings.ToLower(v.name())
		route := fmt.Sprintf("/%v", path)
		l := NavLink{Name: Caser.String(path)}

		if path == "index" || path == "readme" {
			route = "/"
			path = "index"

			l.Name = "Home"
		}

		v.Path = path
		l.Path = route

		links[i] = &l
	}

	for i, v := range views {
		views[i].links = links

		logger.Info(v.Path)

	}

	return views
}

// readContentDirectory recursively calls constructors on a
// provided directory and creates pointers to views
func readContentDirectory(dir string, tdir string) []*View {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Fatalf("unable to create views for directory %v: %v", dir, err.Error())
	}

	views := []*View{}
	for _, entry := range entries {
		fpath := fmt.Sprintf("%v/%v", dir, entry.Name())
		if entry.IsDir() {
			nested_views := readContentDirectory(fpath, tdir)
			views = slices.Concat(views, nested_views)
			continue
		}

		if libs.IsNotMarkdown(entry.Name()) {
			continue
		}

		v := openContentFile(fpath, tdir)

		if v == nil ||
			(v.front != nil && v.front.Draft) {
			continue
		}

		views = append(views, v)

	}

	views = WithNavigation(views)

	return views
}

func openContentFile(p string, t string) *View {
	var v View
	data, err := os.ReadFile(p)
	if err != nil {
		logger.Errorf("unable to read data from %v: %v", p, err.Error())
		return nil
	}

	frontmatter, content, err := SplitFrontmatter(data)

	if err != nil {
		logger.Warnf("unable to read content %v", err)
		// p, nil, data, []byte{}, t, nil, []*NavLink{}

		v = NewView(p, data, t)
		v.renderContentToHTML()
		return &v
	}

	if frontmatter == nil {
		v = NewView(p, data, t)
		v.renderContentToHTML()
		return &v
	}

	logger.Info(frontmatter.Title)
	v = NewView(p, content, t)
	v.front = frontmatter
	v.renderContentToHTML()
	return &v
}

func (v *View) renderContentToHTML() {
	p := parser.NewWithExtensions(
		parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock)
	doc := p.Parse(v.content)
	renderer := html.NewRenderer(
		html.RendererOptions{
			Flags: html.CommonFlags | html.HrefTargetBlank,
		})

	v.html_content = markdown.Render(doc, renderer)
}

func (v *View) getTemplate() {
	patterns := []string{v.name(), "base"}
	var err error
	for _, p := range patterns {
		v.templ, err = template.ParseGlob(
			fmt.Sprintf("%v/%v.html",
				v.templateDir, p,
			),
		)

		if err != nil {
			logger.Debugf("unable to parse parse glob for %v: %v",
				p, err.Error(),
			)
		}

		if v.templ != nil {
			return
		}
	}

	if v.templ == nil {
		logger.Fatalf("unable to find patterns %v in %v", patterns, v.templateDir)
	}
}

func (v *View) Build() *View {
	v.renderContentToHTML()
	v.getTemplate()

	b := bytes.NewBuffer([]byte{})
	err := v.Render(b)
	if err != nil {
		logger.Fatalf("unable to render %v \n%v", v.name(), err.Error())
	}

	v.html_page = b.Bytes()

	return v
}

// Template Context
type Context struct {
	Contents template.HTML
	// Configurable Attributes
	Links     []*NavLink
	Theme     string
	DocTitle  string
	PageTitle string
}

// func Render executes and writes the template with included frontmatter
func (v *View) Render(w io.Writer) error {
	templ_ctx := Context{
		Contents:  template.HTML(v.HTMLContent()),
		Theme:     "dark",
		DocTitle:  "Owais J.",
		PageTitle: "Owais J.",
		Links:     v.links,
	}

	if v.front != nil {
		if templ_ctx.DocTitle != v.front.Title {
			templ_ctx.DocTitle = fmt.Sprintf("%v | %v", v.front.Title, templ_ctx.DocTitle)
		}
		templ_ctx.PageTitle = v.front.Title
	}

	err := v.templ.Execute(w, templ_ctx)

	return err
}

func (v View) name() string {
	p := strings.Split(v.Path, "/")
	f := p[len(p)-1]
	return strings.Split(f, ".")[0]
}

func (v View) Content() string {
	return string(v.content)
}

// function HTMLContent is a getter for the parsed and rendered
// Markdown content as a string
func (v View) HTMLContent() string {
	return string(v.html_content)
}

func (v View) HTML() []byte {
	return v.html_page
}

type NavLink struct {
	Name string
	Path string
}

func NewViews(content, templates string) []*View {
	return readContentDirectory(content, templates)
}