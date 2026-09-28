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
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/fonts"
	"github.com/desertthunder/documango/internal/pagefind"
	"github.com/desertthunder/documango/internal/render"
	"github.com/desertthunder/documango/internal/server"
	"github.com/desertthunder/documango/internal/site"
	"github.com/desertthunder/documango/internal/theme"
)

const rootLong = `documango turns a folder of Markdown files into a documentation website.

Run it with no command to preview a folder (the current one by default) in
your browser. Pages reload as you edit. This is the same as 'documango serve'.
Run 'documango build' to write a static site you can host anywhere.`

const rootExample = `  # Preview the docs in the current folder
  documango

  # Preview the docs folder and open it in your browser
  documango docs --open

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
	root.AddCommand(a.newInitCmd())
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
	title, darkTheme, lightTheme, basePath, search, config string
	// flags reports which options were set on the command line; those win
	// over the config file.
	flags *pflag.FlagSet
}

func (o *siteOptions) addFlags(fs *pflag.FlagSet) {
	o.flags = fs
	fs.StringVar(&o.title, "title", "", `site title (default: the home page's title, else "Documentation")`)
	fs.StringVar(&o.darkTheme, "dark-theme", "tomorrow-night",
		"dark mode themes, comma-separated: names from 'documango themes' or base16 YAML files; the first is the default")
	fs.StringVar(&o.lightTheme, "light-theme", "tomorrow",
		"light mode themes, comma-separated: names from 'documango themes' or base16 YAML files; the first is the default")
	fs.StringVar(&o.basePath, "base-path", "/", "URL path the site lives under, such as /docs/")
	fs.StringVar(&o.search, "search", "pagefind", "search engine: pagefind (full-text, downloaded on first use) or builtin")
	fs.StringVar(&o.config, "config", "", "config file to use instead of documango.toml or documango.yaml in the docs folder")
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

// fontsAPIURL and fontsCDNURL override where fonts come from, for tests.
var fontsAPIURL, fontsCDNURL string

func (a *app) catalog() *theme.Catalog {
	return &theme.Catalog{CacheDir: a.cacheDir("schemes"), Logger: a.logger()}
}

// newBuilder checks the site flags and returns a builder for the docs in
// dir. The config file is read, and the themes loaded, on every build.
func (a *app) newBuilder(dir string, o *siteOptions) (*builder, error) {
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
	b := &builder{
		opts: o, dark: darkNames, light: lightNames, dir: dir, version: a.version,
		catalog: a.catalog(), log: a.logger(),
		// The config file never becomes part of the site.
		exclude: slices.Clone(config.Names),
		finder:  &pagefind.Finder{CacheDir: a.cacheDir("pagefind", pagefind.Version), Getenv: a.getenv, Logger: a.logger()},
		fonts:   &fonts.Loader{CacheDir: a.cacheDir("fonts"), APIURL: fontsAPIURL, CDNURL: fontsCDNURL, Logger: a.logger()},
	}
	if o.config != "" {
		absDir, err := resolvePath(dir)
		if err != nil {
			return nil, err
		}
		abs, err := resolvePath(o.config)
		if err != nil {
			return nil, err
		}
		if isWithin(abs, absDir) {
			rel, _ := filepath.Rel(absDir, abs)
			b.exclude = append(b.exclude, filepath.ToSlash(rel))
		}
	}
	return b, nil
}

// builder loads and renders the docs in one directory.
type builder struct {
	opts        *siteOptions
	dark, light []string // from the flags
	dir         string
	version     string
	exclude     []string
	catalog     *theme.Catalog
	log         *slog.Logger
	// serving enables live reload and fixes the base path after the first
	// build, since the server routes by it.
	serving bool

	// Set by configure on each build.
	load   site.Options
	render render.Options
	search string

	finder       *pagefind.Finder
	pagefindPath string
	// fellBack is set once pagefind failed; the site then uses the built-in
	// search.
	fellBack bool

	fonts *fonts.Loader
	// fontsKey is the [fonts] config that fontFiles were loaded for.
	fontsKey  string
	fontFiles fonts.Result
	// fontsFailed is set once fonts could not be downloaded; the site then
	// uses the system fonts.
	fontsFailed bool
}

// configure reads the config file and settles the site options: flags set
// on the command line win over the config file, which wins over defaults.
func (b *builder) configure(ctx context.Context) error {
	cfg, err := config.Load(b.dir, b.opts.config)
	if err != nil {
		return err
	}
	setting := func(name, fromConfig string) string {
		if f := b.opts.flags.Lookup(name); f.Changed || fromConfig == "" {
			return f.Value.String()
		}
		return fromConfig
	}
	load := func(mode string, fromFlag, fromConfig []string) ([]theme.Scheme, error) {
		names, source := fromFlag, "--"+mode+"-theme"
		if !b.opts.flags.Changed(mode+"-theme") && len(fromConfig) > 0 {
			names, source = fromConfig, fmt.Sprintf("theme.%s in %s", mode, cfg.Path)
		}
		schemes := make([]theme.Scheme, len(names))
		for i, name := range names {
			s, err := b.catalog.Load(ctx, name)
			if err != nil {
				return nil, withHint(fmt.Errorf("%s theme: %w", mode, err), "Run 'documango themes' to list the available themes.")
			}
			if s.Variant != mode {
				b.log.Warn(fmt.Sprintf("%s is a %s theme but is listed in %s", name, s.Variant, source))
			}
			schemes[i] = s
		}
		return schemes, nil
	}
	dark, err := load("dark", b.dark, cfg.Theme.Dark)
	if err != nil {
		return err
	}
	light, err := load("light", b.light, cfg.Theme.Light)
	if err != nil {
		return err
	}

	base := path.Clean("/" + setting("base-path", cfg.BasePath))
	if base != "/" {
		base += "/"
	}
	if b.serving && b.render.BasePath != "" && base != b.render.BasePath {
		b.log.Warn("restart documango to serve the site under a new base path", "base_path", base)
		base = b.render.BasePath
	}
	links := make([]render.Link, len(cfg.Links))
	for i, l := range cfg.Links {
		links[i] = render.Link{Title: l.Title, URL: l.URL}
	}
	font, err := b.loadFonts(ctx, cfg)
	if err != nil {
		return err
	}
	b.load = site.Options{Title: setting("title", cfg.Title), BasePath: base, Exclude: b.exclude}
	b.render = render.Options{
		Dark: dark, Light: light, BasePath: base, Version: b.version,
		Description: cfg.Description, URL: cfg.URL, Author: cfg.Author, Language: cfg.Language,
		Favicon: cfg.Favicon, Logo: cfg.Logo, Links: links,
		FontCSS: font.CSS, FontFiles: font.Files,
	}
	if b.serving {
		b.render.LiveReload = server.EventsURLFor(base)
	}
	b.search = setting("search", cfg.Theme.Search)
	return nil
}

// loadFonts returns the fonts the config asks for. They are loaded again
// only when the [fonts] config changes. When they cannot be downloaded, it
// warns once and the site keeps the system fonts until documango restarts.
func (b *builder) loadFonts(ctx context.Context, cfg *config.Config) (fonts.Result, error) {
	f := cfg.Fonts
	key := strings.Join([]string{f.Body, f.Heading, f.Mono}, "\x00")
	if b.fontsFailed || key == b.fontsKey {
		return b.fontFiles, nil
	}
	res, err := b.fonts.Load(ctx, f.Body, f.Heading, f.Mono)
	if ctx.Err() != nil {
		return fonts.Result{}, ctx.Err()
	}
	if errors.Is(err, fonts.ErrUnavailable) {
		b.log.Warn("fonts unavailable; using system fonts (connect once to download them)", "err", err)
		b.fontsFailed = true
		b.fontFiles = fonts.Result{}
		return b.fontFiles, nil
	}
	if err != nil {
		return fonts.Result{}, withHint(fmt.Errorf("fonts in %s: %w", cfg.Path, err),
			"Use a family name from https://fontsource.org, such as \"Inter\".")
	}
	b.fontsKey, b.fontFiles = key, res
	return res, nil
}

func (b *builder) build(ctx context.Context) (*site.Site, map[string][]byte, error) {
	if err := b.configure(ctx); err != nil {
		return nil, nil, err
	}
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
	if b.search != "pagefind" || b.fellBack {
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
		b.fellBack = true
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
