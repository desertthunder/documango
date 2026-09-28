// Command tools holds development helpers for documango.
//
// Usage:
//
//	go run ./cmd/tools schemes [-ref spec-0.11] [-out internal/theme/schemes]
//
// The schemes subcommand downloads the tinted-theming/schemes repository at the
// given ref and copies its base16 YAML files and LICENSE into the out directory.
package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultBase = "https://codeload.github.com/tinted-theming/schemes/tar.gz"
	defaultRef  = "spec-0.11"
	defaultOut  = "internal/theme/schemes"
	usage       = "usage: tools schemes [-ref REF] [-out DIR]\n"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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
	ref := fs.String("ref", defaultRef, "tinted-theming/schemes git ref to download")
	out := fs.String("out", defaultOut, "directory to write scheme files into")
	base := fs.String("base", defaultBase, "tarball download base URL")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %v\n%s", fs.Args(), usage)
		return 2
	}

	n, err := syncSchemes(*base+"/"+*ref, *out)
	if err != nil {
		fmt.Fprintf(stderr, "schemes: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %d schemes to %s\n", n, *out)
	return 0
}

// syncSchemes downloads the tarball at url and replaces the YAML files in out
// with its base16 schemes. It returns the number of schemes written.
func syncSchemes(url, out string) (int, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s: %s", url, resp.Status)
	}

	parent := filepath.Dir(filepath.Clean(out))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", parent, err)
	}
	stage, err := os.MkdirTemp(parent, ".schemes-*")
	if err != nil {
		return 0, fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stage)

	n, err := extract(resp.Body, stage)
	if err != nil {
		return 0, err
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
	return n, nil
}

// extract writes base16/*.yaml and LICENSE from the gzipped tarball r into dir.
// Paths in the archive are expected under a single top-level directory.
func extract(r io.Reader, dir string) (int, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	schemes, license := 0, false
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read tarball: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		_, rel, ok := strings.Cut(hdr.Name, "/")
		if !ok {
			continue
		}
		var name string
		switch {
		case rel == "LICENSE":
			name, license = "LICENSE", true
		case path.Dir(rel) == "base16" && path.Ext(rel) == ".yaml":
			name = path.Base(rel)
			schemes++
		default:
			continue
		}
		if err := writeFile(filepath.Join(dir, name), tr); err != nil {
			return 0, err
		}
	}
	if schemes == 0 {
		return 0, errors.New("no base16 schemes found in tarball")
	}
	if !license {
		return 0, errors.New("LICENSE not found in tarball")
	}
	return schemes, nil
}

func writeFile(name string, r io.Reader) error {
	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	return nil
}
