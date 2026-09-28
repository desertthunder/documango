package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/desertthunder/documango/internal/pagefind"
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

  # Offer readers several dark themes; the first is the default
  documango docs --dark-theme nord,gruvbox-dark-medium,./my-theme.yaml

  # Pick a dark theme interactively
  documango docs --dark-theme "$(documango themes pick --variant dark)"

  # List every theme
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
	pf.BoolVarP(&a.quiet, "quiet", "q", false, "only print warnings and errors (serve still prints its address)")
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
	title, darkTheme, lightTheme, basePath, search string
}

func (o *siteOptions) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.title, "title", "", `site title (default: the home page's title, else "Documentation")`)
	fs.StringVar(&o.darkTheme, "dark-theme", "tomorrow-night",
		"dark mode themes, comma-separated: names from 'documango themes' or base16 YAML files; the first is the default")
	fs.StringVar(&o.lightTheme, "light-theme", "tomorrow",
		"light mode themes, comma-separated: names from 'documango themes' or base16 YAML files; the first is the default")
	fs.StringVar(&o.basePath, "base-path", "/", "URL path the site lives under, such as /docs/")
	fs.StringVar(&o.search, "search", "pagefind", "search engine: pagefind (full-text, downloaded on first use) or builtin")
}

// themeList splits a comma-separated theme flag value into its entries.
func themeList(flag, value string) ([]string, error) {
	var list []string
	seen := map[string]bool{}
	for entry := range strings.SplitSeq(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, &usageError{fmt.Errorf("--%s has an empty entry in %q", flag, value)}
		}
		key := strings.ToLower(entry)
		if seen[key] {
			return nil, &usageError{fmt.Errorf("--%s lists %q more than once", flag, entry)}
		}
		seen[key] = true
		list = append(list, entry)
	}
	return list, nil
}

// cacheDir returns the folder set by DOCUMANGO_CACHE_DIR joined with elem,
// or "" so the libraries use their default.
func (a *app) cacheDir(elem ...string) string {
	dir := a.getenv("DOCUMANGO_CACHE_DIR")
	if dir == "" {
		return ""
	}
	return filepath.Join(append([]string{dir}, elem...)...)
}

func (a *app) catalog() *theme.Catalog {
	return &theme.Catalog{CacheDir: a.cacheDir("schemes"), Logger: a.logger()}
}

// newBuilder checks the site flags, loads the themes and returns a builder
// for the docs in dir.
func (a *app) newBuilder(ctx context.Context, dir string, o *siteOptions) (*builder, error) {
	darkNames, err := themeList("dark-theme", o.darkTheme)
	if err != nil {
		return nil, err
	}
	lightNames, err := themeList("light-theme", o.lightTheme)
	if err != nil {
		return nil, err
	}
	if o.search != "pagefind" && o.search != "builtin" {
		return nil, &usageError{fmt.Errorf("invalid --search %q: use pagefind or builtin", o.search)}
	}

	log := a.logger()
	catalog := a.catalog()
	load := func(mode string, names []string) ([]theme.Scheme, error) {
		schemes := make([]theme.Scheme, len(names))
		for i, name := range names {
			s, err := catalog.Load(ctx, name)
			if err != nil {
				return nil, withHint(fmt.Errorf("%s theme: %w", mode, err), "Run 'documango themes' to list the available themes.")
			}
			if s.Variant != mode {
				log.Warn(fmt.Sprintf("%s is a %s theme but is listed in --%s-theme", name, s.Variant, mode))
			}
			schemes[i] = s
		}
		return schemes, nil
	}
	dark, err := load("dark", darkNames)
	if err != nil {
		return nil, err
	}
	light, err := load("light", lightNames)
	if err != nil {
		return nil, err
	}

	base := path.Clean("/" + o.basePath)
	if base != "/" {
		base += "/"
	}
	b := &builder{
		dir:    dir,
		load:   site.Options{Title: o.title, BasePath: base},
		render: render.Options{Dark: dark, Light: light, BasePath: base, Version: a.version},
		log:    log,
	}
	if o.search == "pagefind" {
		b.finder = &pagefind.Finder{CacheDir: a.cacheDir("pagefind", pagefind.Version), Getenv: a.getenv, Logger: log}
	}
	return b, nil
}

// builder loads and renders the docs in one directory.
type builder struct {
	dir    string
	load   site.Options
	render render.Options
	log    *slog.Logger

	// finder is nil when the site uses the built-in search, either by
	// choice or after pagefind failed.
	finder       *pagefind.Finder
	pagefindPath string
	fellBack     bool
}

func (b *builder) build(ctx context.Context) (*site.Site, map[string][]byte, error) {
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
	if b.finder == nil {
		return s, files, nil
	}
	if b.pagefindPath == "" {
		b.pagefindPath, err = b.finder.Find(ctx)
	}
	var bundle map[string][]byte
	if err == nil {
		// Index only the rendered pages: HTML files copied from the docs
		// folder, such as an old build, would show up as duplicate results.
		pages := make(map[string][]byte, len(s.Pages))
		for _, p := range s.Pages {
			pages[p.OutPath] = files[p.OutPath]
		}
		bundle, err = pagefind.Index(ctx, b.pagefindPath, pages)
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if err != nil {
		b.log.Warn("pagefind search is unavailable; using the built-in search", "err", err)
		b.finder, b.fellBack = nil, true
	}
	maps.Copy(files, bundle)
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
