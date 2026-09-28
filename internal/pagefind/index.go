package pagefind

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// BundleDir is the site-relative directory holding the generated search bundle.
const BundleDir = "pagefind"

// Index runs the pagefind binary bin over the HTML pages in files, keyed by
// slash-separated site paths, and returns the generated bundle keyed by site
// path (for example "pagefind/pagefind.js"). Non-HTML files and existing
// bundle files are ignored.
func Index(ctx context.Context, bin string, files map[string][]byte) (map[string][]byte, error) {
	dir, err := os.MkdirTemp("", "documango-pagefind-*")
	if err != nil {
		return nil, fmt.Errorf("pagefind: create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	pages := 0
	for name, data := range files {
		if path.Ext(name) != ".html" || strings.HasPrefix(name, BundleDir+"/") {
			continue
		}
		local := filepath.FromSlash(name)
		if !filepath.IsLocal(local) {
			return nil, fmt.Errorf("pagefind: invalid site path %q", name)
		}
		dst := filepath.Join(dir, local)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("pagefind: stage %s: %w", name, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, fmt.Errorf("pagefind: stage %s: %w", name, err)
		}
		pages++
	}
	if pages == 0 {
		return nil, errors.New("pagefind: no HTML pages to index")
	}

	// Run from the temp dir so a pagefind config file in the caller's working
	// directory cannot change the output.
	cmd := exec.CommandContext(ctx, bin, "--site", dir, "--output-subdir", BundleDir, "--silent")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("pagefind: index: %w", ctx.Err())
		}
		return nil, fmt.Errorf("pagefind: index with %s: %w: %s", bin, err, strings.TrimSpace(string(out)))
	}

	bundle := make(map[string][]byte)
	err = filepath.WalkDir(filepath.Join(dir, BundleDir), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		bundle[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pagefind: read bundle: %w", err)
	}
	return bundle, nil
}
