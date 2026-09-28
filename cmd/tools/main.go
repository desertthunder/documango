// Command tools holds development helpers for documango.
//
// Usage:
//
//	go run ./cmd/tools schemes [-ref spec-0.11] [-out internal/theme/schemes]
//	go run ./cmd/tools browsers [-with-deps]
//	go run ./cmd/tools screenshots [-out .github/assets] [-docs docs] [-only NAME,...]
//
// The schemes subcommand downloads the tinted-theming/schemes repository at the
// given ref and copies the curated base16 schemes (see theme.Curated) and the
// LICENSE into the out directory.
//
// The browsers subcommand installs the Playwright driver and Chromium used by
// the browser tests in e2e. -with-deps also installs the system packages
// Chromium needs, which requires root on Linux.
//
// The screenshots subcommand writes the README images: terminal captures made
// with freeze and framed captures of the docs site made with Chromium. Run it
// from the repository root; -only takes a comma-separated list of shot names.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mxschmitt/playwright-go"

	"github.com/desertthunder/documango/internal/theme"
)

const (
	defaultOut = "internal/theme/schemes"
	usage      = "usage: tools schemes [-ref REF] [-out DIR]\n       tools browsers [-with-deps]\n       tools screenshots [-out DIR] [-docs DIR] [-only NAME,...]\n"
)

// install is playwright.Install, replaced in tests.
var install = playwright.Install

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "schemes":
		return runSchemes(ctx, args[1:], stdout, stderr)
	case "browsers":
		return runBrowsers(args[1:], stdout, stderr)
	case "screenshots":
		return runScreenshots(ctx, args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
	return 2
}

func runSchemes(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("schemes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	ref := fs.String("ref", theme.DefaultRef, "tinted-theming/schemes git ref to download")
	out := fs.String("out", defaultOut, "directory to write scheme files into")
	base := fs.String("base", theme.DefaultBaseURL, "GitHub API base URL")
	if !parse(fs, args, stderr) {
		return 2
	}

	n, err := syncSchemes(ctx, &theme.Catalog{Ref: *ref, BaseURL: *base}, *out)
	if err != nil {
		fmt.Fprintf(stderr, "schemes: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %d schemes to %s\n", n, *out)
	return 0
}

func runBrowsers(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("browsers", flag.ContinueOnError)
	fs.SetOutput(stderr)
	withDeps := fs.Bool("with-deps", false, "also install the system packages Chromium needs")
	if !parse(fs, args, stderr) {
		return 2
	}
	opts := &playwright.RunOptions{Browsers: []string{"chromium"}, WithDeps: *withDeps, Stdout: stdout, Stderr: stderr}
	if err := install(opts); err != nil {
		fmt.Fprintf(stderr, "browsers: %v\n", err)
		return 1
	}
	return 0
}

// parse parses args into fs and rejects positional arguments.
func parse(fs *flag.FlagSet, args []string, stderr io.Writer) bool {
	if err := fs.Parse(args); err != nil {
		return false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n%s", fs.Args(), usage)
		return false
	}
	return true
}

// syncSchemes downloads the curated schemes and LICENSE from cat and replaces
// the YAML files in out with them. It fails without touching out when any
// curated scheme is missing upstream, and returns the number written.
func syncSchemes(ctx context.Context, cat *theme.Catalog, out string) (int, error) {
	parent := filepath.Dir(filepath.Clean(out))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", parent, err)
	}
	stage, err := os.MkdirTemp(parent, ".schemes-*")
	if err != nil {
		return 0, fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stage)

	curated := theme.Curated()
	got, err := cat.Download(ctx, stage, func(slug string) bool { return slices.Contains(curated, slug) })
	if err != nil {
		return 0, err
	}
	var missing []string
	for _, slug := range curated {
		if !slices.Contains(got, slug) {
			missing = append(missing, slug)
		}
	}
	if len(missing) > 0 {
		return 0, fmt.Errorf("curated schemes missing upstream: %s", strings.Join(missing, ", "))
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", out, err)
	}
	stale, err := filepath.Glob(filepath.Join(out, "*.yaml"))
	if err != nil {
		return 0, fmt.Errorf("list %s: %w", out, err)
	}
	for _, f := range stale {
		if err := os.Remove(f); err != nil {
			return 0, fmt.Errorf("remove stale scheme: %w", err)
		}
	}
	staged, err := os.ReadDir(stage)
	if err != nil {
		return 0, fmt.Errorf("read staging dir: %w", err)
	}
	for _, e := range staged {
		if err := os.Rename(filepath.Join(stage, e.Name()), filepath.Join(out, e.Name())); err != nil {
			return 0, fmt.Errorf("move %s: %w", e.Name(), err)
		}
	}
	return len(got), nil
}
