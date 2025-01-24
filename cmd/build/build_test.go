// package build tests read from the example directory and builds a site
// in the temp directory
//
// {root}/example
package build

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/libs"
)

func setupConf() (string, string, *config.Config) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)
	conf := config.OpenConfig(fmt.Sprintf("%v/%v", base_path, "config.toml"))
	return root, base_path, conf
}

func mutateConf(conf *config.Config, contextDir string) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)

	conf.Options.BuildDir = fmt.Sprintf("%v/%v/%v", base_path, conf.Options.BuildDir, contextDir)
	conf.Options.TemplateDir = fmt.Sprintf("%v/%v", base_path, conf.Options.TemplateDir)
	conf.Options.ContentDir = fmt.Sprintf("%v/%v", base_path, conf.Options.ContentDir)
	conf.Options.StaticDir = fmt.Sprintf("%v/%v", base_path, conf.Options.StaticDir)
}

func TestBuild(t *testing.T) {
	sb := strings.Builder{}
	BuildLogger = log.Default()
	BuildLogger.SetLevel(log.ErrorLevel)
	BuildLogger.SetOutput(&sb)

	_, base_path, conf := setupConf()

	var views []*View

	t.Run("load config from toml file", func(t *testing.T) {
		defaultConf := config.NewDefaultConfig()

		defaultOpts := defaultConf.Options
		opts := conf.Options

		if opts.ContentDir == defaultOpts.ContentDir {
			t.Error("content directory not updated")
		}

		if opts.TemplateDir == defaultOpts.TemplateDir {
			t.Error("template directory not updated")
		}

		if opts.StaticDir == defaultOpts.StaticDir {
			t.Error("static directory not updated")
		}

		if opts.BuildDir == defaultOpts.BuildDir {
			t.Error("build directory not updated")
		}

		if sp := opts.GetStaticPath(); sp == defaultOpts.GetStaticPath() {
			t.Errorf("%v should have changed", sp)
		}
	})

	t.Run("updates log level based on config", func(t *testing.T) {
		original := BuildLogger.GetLevel()

		conf.UpdateLogLevel(BuildLogger)

		got := BuildLogger.GetLevel()

		if got == original {
			t.Errorf("%v should not be %v", got.String(), original.String())
		}
	})

	// NOTE: At this point, we're not working from the root directory
	// like a user would be so we're going to mutate the opts in the
	// config struct
	mutateConf(conf, "build")

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
					t.Errorf("%v should have content but it does not", v.name())
				}

				// We know there are headings in our test files, so we check
				// that there are id= occurrences in the string representation
				// of the HTML
				if !strings.Contains(string(v.content), "# ") {
					if !strings.Contains(string(v.html_content), "id=") {
						t.Errorf("%v should have occurrences of id= for anchors/linking", v.name())
					}
				}
			})

			desc = fmt.Sprintf("adds non-nil pointer to frontmatter if it exists for %v", v.name())
			t.Run(desc, func(t *testing.T) {
				if v.front != nil {
					if len(v.front.Title) == 0 {
						t.Errorf("%v should have a title but it does not", v.name())
					}
				}
			})

			desc = fmt.Sprintf("should have a not nil pointer to a template %v", v.name())
			t.Run(desc, func(t *testing.T) {
				v.getTemplate()
				if v.templ == nil {
					t.Errorf("%v should have a defined pointer to a template", v.name())
				}
			})

			desc = fmt.Sprintf("adds navigation links to %v", v.name())
			t.Run(desc, func(t *testing.T) {
				if len(v.links) != len(views) {
					t.Errorf("there should be a link for each view (%v total) but there is not", len(views))
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
						t.Errorf("%v should be marked as not markdown but it was", f.Name())
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

	t.Run("build command", func(t *testing.T) {
		s := strings.Builder{}
		sb := strings.Builder{}
		BuildLogger.SetOutput(&s)

		ctx := context.Background()
		ctx = context.WithValue(ctx, config.LoggerKey, BuildLogger)
		ctx = context.WithValue(ctx, config.ConfKey, conf)

		cmd := BuildCommand
		cmd.Writer = &sb

		args := os.Args[0:1]
		args = append(args, "--file")
		args = append(args, fmt.Sprintf("%v/%v", base_path, "config.toml"))

		err := Run(ctx, BuildCommand)

		if err != nil {
			t.Errorf("command should run %v", err.Error())
		}

		dir := conf.Options.BuildDir

		d, err := os.Stat(dir)

		if err != nil {
			t.Errorf("unable to check build dir presence %v", err.Error())
		}

		if !d.IsDir() {
			t.Errorf("should have created tmp dir %v", err.Error())
		}
	})

	t.Run("collects static files", func(t *testing.T) {
		t.Run("copies static files from source to dest (build)", func(t *testing.T) {
			fp, err := CollectStatic(conf)

			if err != nil {
				t.Errorf("failed to copy files %v", err.Error())
			}

			if len(fp) < 1 {
				t.Error("nothing was copied")
			}

			found := false
			for _, f := range fp {
				if strings.Contains(f.FileP, "css") {
					found = true
				}
			}

			if !found {
				t.Error("no css file copied")
			}
		})

		t.Run("creates a copy of the theme.js directory if there is no JS in the static files dir", func(t *testing.T) {
			has_js := false

			entries, err := os.ReadDir(conf.Options.StaticDir)

			if err != nil && err == os.ErrNotExist {
				err = nil
			} else if err != nil {
				t.Errorf("something went wrong %v", err.Error())
			} else {
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".js") {
						has_js = true
						break
					}
				}
			}

			err = CopyJS(conf)

			if err != nil {
				t.Errorf("operation failed %v", err.Error())
			}

			if !has_js {
				_, err := os.ReadFile(fmt.Sprintf("%v/%v", conf.Options.BuildDir, "theme.js"))

				if err != nil && err == os.ErrNotExist {
					t.Errorf("the script should have been copied but it was not %v", err.Error())
				} else if err != nil {
					t.Errorf("something went wrong %v", err.Error())
				}
			}

		})
	})

	t.Run("error states", func(t *testing.T) {
		sb := strings.Builder{}
		failBuildAndExit = func(msg string) {
			sb.WriteString(msg)
		}

		t.Run("CopyStaticFiles returns an error if the dir doesn't exist", func(t *testing.T) {
			c := config.NewDefaultConfig()
			mutateConf(&c, "build")
			c.Options.StaticDir = "nonsense"
			fp, err := CopyStaticFiles(&c)

			if err == nil {
				t.Fatal("CopyStaticFiles should fail because the dir in the config does not exist")
			} else {
				t.Logf("error received with fp: %v", fp)
			}
		})
	})
}
