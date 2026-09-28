package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/desertthunder/documango/internal/render"
	"github.com/desertthunder/documango/internal/site"
	"github.com/desertthunder/documango/internal/theme"
)

const rootLong = `documango turns a folder of Markdown files into a documentation website.

Run it with no command to preview a folder (the current one by default) in
your browser. Pages reload as you edit. This is the same as 'documango serve'.
Run 'documango build' to write a static site you can host anywhere.`

const rootExample = `  # Preview the docs in the current folder
  documango

  # Preview the docs folder
  documango docs

  # Build a static site into _site
  documango build docs

  # List the built-in themes
  documango themes`

// newRootCmd builds the command tree for one invocation.
func (a *app) newRootCmd() *cobra.Command {
	opts := &serveOptions{}
	root := &cobra.Command{
		Use:     "documango [dir]",
		Short:   "Turn a folder of Markdown into a docs site",
		Long:    rootLong,
		Example: rootExample,
		Args:    usageArgs(cobra.MaximumNArgs(1)),
		Version: a.version,

		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
		PersistentPreRunE: a.setup,
	}
	root.RunE = func(cmd *cobra.Command, args []string) error {
		return a.serve(cmd.Context(), dirArg(args), opts, root)
	}
	root.SetOut(a.env.Stdout)
	root.SetErr(a.env.Stderr)
	root.SetVersionTemplate("documango {{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &usageError{err} })
	root.SuggestionsMinimumDistance = 2

	pf := root.PersistentFlags()
	pf.BoolVarP(&a.quiet, "quiet", "q", false, "only print errors")
	pf.BoolVarP(&a.verbose, "verbose", "v", false, "print debug logs, such as which files changed")
	pf.BoolVar(&a.noColor, "no-color", false, "turn off coloured output")
	opts.addFlags(root.Flags())

	root.AddCommand(a.newServeCmd(), a.newBuildCmd(), a.newThemesCmd())
	return root
}

// dirArg returns the directory argument, defaulting to the current directory.
func dirArg(args []string) string {
	if len(args) == 0 {
		return "."
	}
	return args[0]
}

// siteOptions are the flags shared by every command that renders the site.
type siteOptions struct {
	title, darkTheme, lightTheme, basePath string
}

func (o *siteOptions) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.title, "title", "", `site title (default: the home page's title, else "Documentation")`)
	fs.StringVar(&o.darkTheme, "dark-theme", "tomorrow-night", "dark mode theme: a built-in name or a base16 YAML file")
	fs.StringVar(&o.lightTheme, "light-theme", "tomorrow", "light mode theme: a built-in name or a base16 YAML file")
	fs.StringVar(&o.basePath, "base-path", "/", "URL path the site lives under, such as /docs/")
}

// builder loads the themes and returns a builder for the docs in dir.
func (o *siteOptions) builder(dir, version string) (*builder, error) {
	dark, err := theme.Load(o.darkTheme)
	if err != nil {
		return nil, withHint(fmt.Errorf("dark theme: %w", err), "Run 'documango themes' to list the built-in themes.")
	}
	light, err := theme.Load(o.lightTheme)
	if err != nil {
		return nil, withHint(fmt.Errorf("light theme: %w", err), "Run 'documango themes' to list the built-in themes.")
	}
	base := path.Clean("/" + o.basePath)
	if base != "/" {
		base += "/"
	}
	return &builder{
		dir:    dir,
		load:   site.Options{Title: o.title, BasePath: base},
		render: render.Options{Dark: dark, Light: light, BasePath: base, Version: version},
	}, nil
}

// builder loads and renders the docs in one directory.
type builder struct {
	dir    string
	load   site.Options
	render render.Options
}

func (b *builder) build() (*site.Site, map[string][]byte, error) {
	fsys := os.DirFS(b.dir)
	s, err := site.Load(fsys, b.load)
	if err != nil {
		return nil, nil, fmt.Errorf("load %s: %w", b.dir, err)
	}
	if len(s.Pages) == 0 {
		return nil, nil, withHint(fmt.Errorf("no Markdown files found in %s", b.dir),
			"Add an index.md or README.md, or pass the folder that holds your docs, such as 'documango docs'.")
	}
	files, err := render.Render(s, fsys, b.render)
	if err != nil {
		return nil, nil, fmt.Errorf("render %s: %w", b.dir, err)
	}
	return s, files, nil
}

// checkDir reports a clear error unless dir is an existing directory. When
// root is non-nil and dir looks like a mistyped command, the error suggests it.
func checkDir(dir string, root *cobra.Command) error {
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		err = fmt.Errorf("directory %q does not exist", dir)
		if root != nil {
			if s := root.SuggestionsFor(dir); len(s) > 0 {
				return withHint(err, fmt.Sprintf("did you mean %q?", s[0]))
			}
		}
		return withHint(err, "Pass the folder that holds your Markdown files.")
	case err != nil:
		return fmt.Errorf("open docs directory: %w", err)
	case !info.IsDir():
		return fmt.Errorf("%q is not a directory", dir)
	}
	return nil
}
