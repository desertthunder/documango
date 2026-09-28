package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// manifestPath is where a build lists every file it wrote, relative to the
// output folder, so the next build can remove the ones it no longer writes.
const manifestPath = "_documango/manifest.json"

// addManifest adds the manifest, which lists itself, to files.
func addManifest(files map[string][]byte) error {
	files[manifestPath] = nil
	data, err := json.MarshalIndent(slices.Sorted(maps.Keys(files)), "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	files[manifestPath] = append(data, '\n')
	return nil
}

// readManifest returns the paths listed by the build in dir. A missing or
// corrupt manifest lists nothing.
func readManifest(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(manifestPath)))
	if err != nil {
		return nil
	}
	var paths []string
	if json.Unmarshal(data, &paths) != nil {
		return nil
	}
	return paths
}

// removeStale deletes the regular files in dir listed in old but absent from
// files, then any directories those deletions left empty. Paths that are
// invalid or resolve outside dir are skipped.
func removeStale(dir string, old []string, files map[string][]byte) error {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("remove stale files: %w", err)
	}
	for _, name := range old {
		if _, ok := files[name]; ok || name == "." || !fs.ValidPath(name) {
			continue
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Join(root, filepath.FromSlash(name))))
		if err != nil || !isWithin(parent, root) {
			continue
		}
		p := filepath.Join(parent, filepath.Base(name))
		if info, err := os.Lstat(p); err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove stale file %s: %w", name, err)
		}
		for d := parent; d != root && isWithin(d, root); d = filepath.Dir(d) {
			if os.Remove(d) != nil {
				break
			}
		}
	}
	return nil
}
