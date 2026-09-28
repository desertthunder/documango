package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/desertthunder/documango/internal/render"
)

const buildExample = `  # Build the docs in the current folder into _site
  documango build

  # Build the docs folder into public, replacing an earlier build
  documango build docs --out public --clean

  # Build for a site hosted at https://example.com/project/
  documango build docs --base-path /project/

  # Build with settings from a config file outside the docs folder
  documango build docs --config site/documango.toml

  # Offer two light themes and skip the Pagefind download
  documango build docs --light-theme tomorrow,catppuccin-latte --search builtin`

// buildOptions are the flags of the build command.
type buildOptions struct {
	site  siteOptions
	out   string
	clean bool
}

func (a *app) newBuildCmd() *cobra.Command {
	opts := &buildOptions{}
	cmd := &cobra.Command{
		Use:   "build [dir]",
		Short: "Build a static site",
		Long: `Build the docs in dir (the current folder by default) into a static site
you can upload to any web host. The output folder is skipped when it sits
inside dir.`,
		Example: buildExample,
		Args:    usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.build(cmd.Context(), dirArg(args), opts)
		},
	}
	fs := cmd.Flags()
	opts.site.addFlags(fs)
	fs.StringVarP(&opts.out, "out", "o", "_site", "folder to write the site to")
	fs.BoolVar(&opts.clean, "clean", false, "empty the output folder first; refuses if it holds anything but an earlier build")
	return cmd
}

// isWithin reports whether path is base or inside it; both must be absolute.
func isWithin(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolvePath returns the absolute path of p with symlinks resolved. When p
// does not exist, its nearest existing ancestor is resolved instead.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	var missing []string
	for dir := abs; ; dir = filepath.Dir(dir) {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			slices.Reverse(missing)
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(dir) == dir {
			return "", fmt.Errorf("resolve %s: %w", p, err)
		}
		missing = append(missing, filepath.Base(dir))
	}
}

// build renders the docs in dir and writes them to the output folder.
func (a *app) build(ctx context.Context, dir string, o *buildOptions) error {
	start := time.Now()
	if err := checkDir(dir, nil); err != nil {
		return err
	}
	absDir, err := resolvePath(dir)
	if err != nil {
		return err
	}
	out, err := resolvePath(o.out)
	if err != nil {
		return err
	}
	if isWithin(absDir, out) {
		return withHint(fmt.Errorf("cannot build into %s: it contains the docs", o.out),
			"Pick an output folder outside your docs, such as the default _site.")
	}

	b, err := a.newBuilder(dir, &o.site)
	if err != nil {
		return err
	}
	if isWithin(out, absDir) {
		rel, _ := filepath.Rel(absDir, out)
		b.exclude = append(b.exclude, filepath.ToSlash(rel))
	}
	s, files, err := b.build(ctx)
	if err != nil {
		return err
	}
	if o.clean {
		if err := cleanDir(out, o.out); err != nil {
			return err
		}
	}
	if err := addManifest(files); err != nil {
		return err
	}
	old := readManifest(out)
	if err := render.Write(files, out); err != nil {
		return fmt.Errorf("write site: %w", err)
	}
	if err := removeStale(out, old, files); err != nil {
		return err
	}

	if !a.quiet {
		pages := "pages"
		if len(s.Pages) == 1 {
			pages = "page"
		}
		elapsed := time.Since(start)
		if elapsed < time.Second {
			elapsed = elapsed.Round(time.Millisecond)
		} else {
			elapsed = elapsed.Round(10 * time.Millisecond)
		}
		search := ""
		if b.fellBack {
			search = " with the built-in search"
		}
		fmt.Fprintf(a.stderr, "%s %d %s to %s%s in %s\n", successStyle.Render("Built"), len(s.Pages), pages,
			accentStyle.Render(o.out), search, dimStyle.Render(elapsed.String()))
	}
	return nil
}

// cleanDir empties dir, which the user named as name. It refuses unless dir
// is missing, empty, or holds an earlier documango build, so user files are
// never deleted.
func cleanDir(dir, name string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		return fmt.Errorf("cannot clean %s: it is not a directory", name)
	}
	if err != nil {
		return fmt.Errorf("clean %s: %w", name, err)
	}
	isBuild := slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return e.Name() == "_documango" && e.IsDir()
	})
	if len(entries) > 0 && !isBuild {
		return withHint(fmt.Errorf("refusing to clean %s: it is not empty and does not look like a documango build", name),
			"Pick an empty output folder, or delete its contents yourself.")
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("clean %s: %w", name, err)
		}
	}
	return nil
}
