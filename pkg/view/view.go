/*
package view creates in-memory HTML documents for use by
the server & build commands.

In its simplest form, our view type contains a reference
to the contents of a markdown file and contains implementations
for methods that create a document using one of the following:

 1. a template in its frontmatter
 2. a template with the same name as the file (sans extensions)
 3. the base template

Then executes (renders) the template by placing it in some stream,
be it file, stdout or stderr.
*/
package view

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/desertthunder/documango/pkg/libs/logs"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type View struct {
	Path        string
	content     []byte
	html        []byte
	templateDir string
	templ       *template.Template
}

var logger = logs.CreateConsoleLogger("[view]")

// ReadContentDirectory recursively calls constructors on a
// provided directory and creates pointers to Views
func ReadContentDirectory(dir string, tdir string) []*View {
	entries, err := os.ReadDir(dir)

	if err != nil {
		logger.Fatalf("unable to create views for directory %v: %v", dir, err.Error())
	}

	views := []*View{}

	for _, entry := range entries {
		fpath := fmt.Sprintf("%v/%v", dir, entry.Name())

		if entry.IsDir() {
			nested_views := ReadContentDirectory(fpath, tdir)
			views = slices.Concat(views, nested_views)
			continue
		}

		if isNotMarkdown(entry.Name()) {
			continue
		}

		if v := openFile(fpath, tdir); v != nil {
			views = append(views, v)
		}
	}

	return views
}

func isNotMarkdown(n string) bool {
	p := strings.Split(n, ".")
	return p[len(p)-1] != "md"
}

func openFile(p string, t string) *View {
	data, err := os.ReadFile(p)

	if err != nil {
		logger.Errorf("unable to read data from %v: %v", p, err.Error())
		return nil
	}

	v := View{p, data, []byte{}, t, nil}
	v.toHTML()

	return &v
}

func (v *View) toHTML() {
	ext := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(ext)

	doc := p.Parse(v.content)

	flags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: flags}
	renderer := html.NewRenderer(opts)

	v.html = markdown.Render(doc, renderer)
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
			logger.Errorf("unable to parse parse glob for %v: %v.html",
				p, err.Error())
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
	v.toHTML()
	v.getTemplate()
	return v
}

// Template Context
type Context = map[string]template.HTML

// func Render executes and writes the template
func (v *View) Render(w io.Writer) Context {
	view := v.Build()

	templ_ctx := map[string]template.HTML{
		"contents": template.HTML(v.HTML()),
	}

	view.templ.Execute(w, templ_ctx)

	return templ_ctx
}

func (v View) name() string {
	p := strings.Split(v.Path, "/")
	f := p[len(p)-1]
	return strings.Split(f, ".")[0]
}

func (v View) Content() string {
	return string(v.content)
}

func (v View) HTML() string {
	return string(v.html)
}
