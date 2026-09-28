package cli

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const initExample = `  # Create a starter site in ./docs
  documango init

  # Create it in another folder with your project's name
  documango init website --title "Acme Docs"

  # Write the config file as YAML instead of TOML
  documango init --format yaml

  # Add the starter files that are missing from an existing folder
  documango init docs --force`

//go:embed scaffold
var scaffoldFS embed.FS

// initOptions are the flags of the init command.
type initOptions struct {
	title, format string
	force         bool
}

func (a *app) newInitCmd() *cobra.Command {
	opts := &initOptions{}
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Create a starter docs folder",
		Long: `Create a starter docs site in dir (docs by default): a home page, a short
guide on writing pages, and a documango.toml config file. Missing folders
are created.

The site title defaults to the name of the project folder: the folder that
holds dir when dir is named docs, else dir itself.

init refuses to write into a folder that isn't empty. With --force it adds
the starter files that are missing and leaves the rest alone. It never
overwrites a file.`,
		Example: initExample,
		Args:    usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := "docs"
			if len(args) > 0 {
				dir = args[0]
			}
			return a.initSite(dir, opts)
		},
	}
	fs := cmd.Flags()
	fs.StringVar(&opts.title, "title", "", "site title (default: the project folder's name)")
	fs.StringVar(&opts.format, "format", "toml", "config file format: toml or yaml")
	fs.BoolVar(&opts.force, "force", false, "write into a folder that isn't empty, skipping files that already exist")
	return cmd
}

// initSite writes the starter site into dir.
func (a *app) initSite(dir string, o *initOptions) error {
	if o.format != "toml" && o.format != "yaml" {
		return &usageError{fmt.Errorf("invalid --format %q: use toml or yaml", o.format)}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}
	title := o.title
	if title == "" {
		title = inferTitle(abs)
	}
	config := "documango." + o.format
	data := struct{ Title, Config string }{title, config}

	entries, err := os.ReadDir(dir)
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	var created, skipped, existing []string
	files := map[string][]byte{}
	for _, name := range []string{config, "index.md", "guide/index.md", "guide/writing.md"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		_, err := os.Lstat(path)
		switch {
		case err == nil:
			skipped = append(skipped, path)
			existing = append(existing, filepath.FromSlash(name))
			continue
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("check %s: %w", path, err)
		}
		body, err := renderScaffold(name, data)
		if err != nil {
			return err
		}
		created = append(created, path)
		files[path] = body
	}

	switch {
	case o.force || len(entries) == 0:
	case len(existing) > 0:
		return withHint(fmt.Errorf("%s already has %s", dir, strings.Join(existing, ", ")),
			"Pass --force to add only the missing starter files. Existing files are never overwritten.")
	default:
		return withHint(fmt.Errorf("%s is not empty", dir),
			"Pick a new or empty folder, or pass --force to add the starter files next to what's there.")
	}
	for _, path := range created {
		if err := writeNewFile(path, files[path]); err != nil {
			return err
		}
	}

	if a.quiet {
		return nil
	}
	w := a.stderr
	if len(created) > 0 {
		fmt.Fprintln(w, successStyle.Render("Created"))
		for _, p := range created {
			fmt.Fprintln(w, "  "+accentStyle.Render(p))
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintln(w, hintStyle.Render("Skipped")+dimStyle.Render(" (already exist)"))
		for _, p := range skipped {
			fmt.Fprintln(w, "  "+p)
		}
	}
	arg := dir
	if strings.ContainsAny(arg, " \t'\"$&;|<>()*?[]#~\\`") {
		arg = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, boldStyle.Render("Next steps"))
	width := len("documango build " + arg)
	fmt.Fprintf(w, "  %s  %s\n", accentStyle.Render(fmt.Sprintf("%-*s", width, "documango "+arg)), dimStyle.Render("preview the site in your browser"))
	fmt.Fprintf(w, "  %s  %s\n", accentStyle.Render("documango build "+arg), dimStyle.Render("build the static site into _site"))
	return nil
}

// renderScaffold fills in the embedded starter file name.
func renderScaffold(name string, data any) ([]byte, error) {
	src, err := scaffoldFS.ReadFile("scaffold/" + name)
	if err != nil {
		return nil, fmt.Errorf("read starter file %s: %w", name, err)
	}
	t, err := template.New(name).Funcs(template.FuncMap{"quote": quoteString}).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse starter file %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render starter file %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// writeNewFile creates path and its parent folders, failing if path exists.
func writeNewFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// inferTitle names the site after the project folder: the parent of dir when
// dir is a docs or doc folder, else dir itself.
func inferTitle(dir string) string {
	name := filepath.Base(dir)
	if strings.EqualFold(name, "docs") || strings.EqualFold(name, "doc") {
		name = filepath.Base(filepath.Dir(dir))
	}
	words := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || unicode.IsSpace(r) || r == filepath.Separator
	})
	for i, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	if len(words) == 0 {
		return "Documentation"
	}
	return strings.Join(words, " ")
}

// quoteString returns s as a double-quoted string that is valid in both TOML
// and YAML.
func quoteString(s string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", fmt.Errorf("quote %q: %w", s, err)
	}
	return strings.ReplaceAll(strings.TrimSuffix(buf.String(), "\n"), "\x7f", `\u007f`), nil
}
