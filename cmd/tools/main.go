// Command tools holds development helpers for documango.
//
// Usage:
//
//	go run ./cmd/tools schemes [-ref spec-0.11] [-out internal/theme/schemes]
//
// The schemes subcommand downloads the tinted-theming/schemes repository at the
// given ref and copies the curated base16 schemes (see theme.Curated) and the
// LICENSE into the out directory.
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

	"github.com/desertthunder/documango/internal/theme"
)

const (
	defaultOut = "internal/theme/schemes"
	usage      = "usage: tools schemes [-ref REF] [-out DIR]\n"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if args[0] != "schemes" {
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}

	fs := flag.NewFlagSet("schemes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	ref := fs.String("ref", theme.DefaultRef, "tinted-theming/schemes git ref to download")
	out := fs.String("out", defaultOut, "directory to write scheme files into")
	base := fs.String("base", theme.DefaultBaseURL, "GitHub API base URL")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n%s", fs.Args(), usage)
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
