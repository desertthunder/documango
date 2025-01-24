package view

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/md"
	"github.com/desertthunder/documango/internal/utils"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	_ "embed"
)

var Caser = cases.Title(language.AmericanEnglish)

//go:embed base.html
var DefaultLayoutTemplate []byte

type NavLink struct {
	Name string
	Path string
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

type View struct {
	Path        string
	Markdown    *md.MD
	HTML        []byte
	templateDir string
	Templ       *template.Template
	Links       []*NavLink
}

func NewViews(contentDir, templateDir string) ([]*View, error) {
	mdFiles, err := md.ReadContentDirectory(contentDir, templateDir)
	if err != nil {
		return []*View{}, err
	}

	views := make([]*View, 0, len(mdFiles))
	for _, m := range mdFiles {
		views = append(views, &View{
			Path:     m.FilePath,
			Markdown: m,
		})
	}

	return WithNavigation(views), nil
}

// function WithNavigation populates a NavLink
// list in the View struct to build context when
// rendering the layout
func WithNavigation(views []*View) []*View {
	links := make([]*NavLink, len(views))
	for i, v := range views {
		path := strings.ToLower(v.Name())
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

	for i := range views {
		views[i].Links = links
	}

	return views
}

// function getTemplate checks for the presence of a template dir, then
// for the following patterns before following back to the default
// embedded above
//
//  1. {template_dir}/{layout}.html
//  2. {template_dir}/{name}.html
//  3. {template_dir}/base.html
//  4. DefaultLayoutTemplate (/cmd/build/views/base.html)
func (v *View) GetTemplate() error {
	_, err := os.ReadDir(v.templateDir)
	if err != nil {
		if v.Markdown.Frontmatter != nil && v.Markdown.Frontmatter.Layout != "" {
			v.Templ, err = template.ParseGlob(fmt.Sprintf("%v/%v.html", v.templateDir, v.Markdown.Frontmatter.Layout))

			if err != nil {
				err = fmt.Errorf("layout (%v) defined in frontmatter for %v not found: %v", v.Markdown.Frontmatter.Layout, v.Name(), err.Error())
			}
		}

		for _, p := range []string{v.Name(), "base"} {
			v.Templ, err = template.ParseGlob(fmt.Sprintf("%v/%v.html", v.templateDir, p))
			if err != nil {
				err = fmt.Errorf("unable to parse parse glob for %v: %v", p, err.Error())
			}
		}
	}

	if v.Templ == nil {
		v.Templ, err = template.New("layout").Parse(string(DefaultLayoutTemplate))
	}

	return err
}

// func Render executes and writes the template with included frontmatter
func (v *View) Render(w io.Writer, conf *config.Config) error {
	templ_ctx := Context{
		Contents:  template.HTML(v.Markdown.HTML()),
		Theme:     "dark",
		DocTitle:  conf.Metadata.Name,
		PageTitle: conf.Metadata.Name,
		Links:     v.Links,
	}

	if v.Markdown.Frontmatter != nil {
		if templ_ctx.DocTitle != v.Markdown.Frontmatter.Title {
			templ_ctx.DocTitle = fmt.Sprintf("%v | %v", v.Markdown.Frontmatter.Title, templ_ctx.DocTitle)
		}
		templ_ctx.PageTitle = v.Markdown.Frontmatter.Title
	}

	err := v.Templ.Execute(w, templ_ctx)

	return err
}

func (v *View) BuildHTMLFileContents(c *config.Config) (string, error) {
	p := fmt.Sprintf("%v/%v.html", c.Options.BuildDir, v.Path)
	f, err := os.Create(p)
	if err != nil {
		return v.Path, err
	}

	defer f.Close()

	v.GetTemplate()

	b := bytes.NewBuffer([]byte{})
	err = v.Render(b, c)
	if err != nil {
		return "", fmt.Errorf("unable to render %v \n%v", v.Name(), err.Error())
	}

	v.HTML = b.Bytes()

	_, err = f.Write(v.HTML)
	if err != nil {
		return "", fmt.Errorf("unable to render %v \n%v", v.Name(), err.Error())
	}

	if v.Path == "index" {
		return "/", err
	} else {
		return "/" + v.Path, err
	}
}

func (v View) Name() string {
	p := strings.Split(v.Markdown.FilePath, "/")
	f := p[len(p)-1]
	return strings.Split(f, ".")[0]
}

func (v View) Handler(logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if code, err := w.Write(v.HTML); err != nil {
			data := utils.CreateErrorJSON(http.StatusInternalServerError, err)

			w.WriteHeader(http.StatusInternalServerError)
			w.Write(data)

			logger.Errorf("unable to execute template with code %v: %v", err.Error(), code)
		}
	}
}
