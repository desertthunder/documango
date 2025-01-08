// package build tests read from the example directory and builds a site
// in the temp directory
//
// {root}/example
package build

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

func TestBuild(t *testing.T) {
	logger = libs.CreateConsoleLogger("[test]")
	logger.SetLevel(log.ErrorLevel)

	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)

	var conf *config.Config
	var views []*View

	t.Run("load config from toml file", func(t *testing.T) {
		defaultConf := config.NewDefaultConfig()
		conf = config.OpenConfig(fmt.Sprintf("%v/%v", base_path, "config.toml"))

		defaultOpts := defaultConf.Options
		opts := conf.Options

		if opts.ContentDir == defaultOpts.ContentDir {
			t.Log("content directory not updated")
			t.Fail()
		}

		if opts.TemplateDir == defaultOpts.TemplateDir {
			t.Log("template directory not updated")
			t.Fail()
		}

		if opts.StaticDir == defaultOpts.StaticDir {
			t.Log("static directory not updated")
			t.Fail()
		}

		if opts.BuildDir == defaultOpts.BuildDir {
			t.Log("build directory not updated")
			t.Fail()
		}

		if sp := opts.GetStaticPath(); sp == defaultOpts.GetStaticPath() {
			t.Logf("%v should have changed", sp)
		}
	})

	t.Run("updates log level based on config", func(t *testing.T) {
		original := logger.GetLevel()

		conf.UpdateLogLevel(logger)

		got := logger.GetLevel()

		if got == original {
			t.Logf("%v should not be %v", got.String(), original.String())
			t.Fail()
		}
	})

	// NOTE: At this point, we're not working from the root directory
	// like a user would be so we're going to mutate the opts in the
	// config struct
	conf.Options.BuildDir = fmt.Sprintf("%v/%v", base_path, conf.Options.BuildDir)
	conf.Options.TemplateDir = fmt.Sprintf("%v/%v", base_path, conf.Options.TemplateDir)
	conf.Options.ContentDir = fmt.Sprintf("%v/%v", base_path, conf.Options.ContentDir)
	conf.Options.StaticDir = fmt.Sprintf("%v/%v", base_path, conf.Options.StaticDir)

	t.Run("creates new views from content & template dir", func(t *testing.T) {
		views = NewViews(conf.Options.ContentDir, conf.Options.TemplateDir)
		views = WithNavigation(views)
		if len(views) < 1 {
			t.Fatalf("there should be at least 1 view, got %v", len(views))
		}

		for _, v := range views {
			desc := fmt.Sprintf("creates HTML markup from markdown for %v", v.name())
			t.Run(desc, func(t *testing.T) {
				if len(v.html_content) == 0 {
					t.Logf("%v should have content but it does not", v.name())
					t.Fail()
				}

				// We know there are headings in our test files, so we check
				// that there are id= occurrences in the string representation
				// of the HTML
				if !strings.Contains(string(v.content), "# ") {
					if !strings.Contains(string(v.html_content), "id=") {
						t.Logf("%v should have occurrences of id= for anchors/linking", v.name())
						t.Fail()
					}
				}
			})

			desc = fmt.Sprintf("adds non-nil pointer to frontmatter if it exists for %v", v.name())
			t.Run(desc, func(t *testing.T) {
				if v.front != nil {
					if len(v.front.Title) == 0 {
						t.Logf("%v should have a title but it does not", v.name())
						t.Fail()
					}
				}
			})

			desc = fmt.Sprintf("should have a not nil pointer to a template %v", v.name())
			t.Run(desc, func(t *testing.T) {
				v.getTemplate()
				if v.templ == nil {
					t.Logf("%v should have a defined pointer to a template", v.name())
					t.Fail()
				}
			})

			desc = fmt.Sprintf("adds navigation links to %v", v.name())
			t.Run(desc, func(t *testing.T) {
				if len(v.links) != len(views) {
					t.Logf("there should be a link for each view (%v total) but there is not", len(views))
					t.Fail()
				}
			})

		}

		t.Run("skips drafts & other types of markup", func(t *testing.T) {
			d, err := os.ReadDir(conf.Options.ContentDir)

			if err != nil {
				t.Fatalf("unable to open dir %v", conf.Options.ContentDir)
			}

			t.Run("IsNotMarkdown returns false for files that aren't md files", func(t *testing.T) {
				for _, f := range d {
					if strings.HasSuffix(f.Name(), ".md") != libs.IsNotMarkdown(f.Name()) {
						continue
					} else {
						t.Logf("%v should be marked as not markdown but it was", f.Name())
						t.Fail()
					}
				}

			})

			if len(views) == len(d) {
				t.Fatalf(
					"there should not be views created for every file in directory %v",
					conf.Options.ContentDir,
				)
			}
		})

	})

	t.Run("collects static files", func(t *testing.T) {
		t.Skip()
		t.Run("copies static files from source to dest (build)", func(t *testing.T) {
		})

		t.Run("builds a theme based on configured options", func(t *testing.T) {

			t.Run("stores stylesheet it in the static build dir", func(t *testing.T) {
			})
		})

	})

	t.Run("builds site to configured build directory", func(t *testing.T) {
		t.Skip()
		t.Run("stores copy of full markup in view", func(t *testing.T) {
		})
	})
}
