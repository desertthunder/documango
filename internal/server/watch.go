package server

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch watches Dir recursively and rebuilds after each debounced batch of
// changes, until ctx is done. It returns nil on cancellation. Watch should be
// called at most once; when it returns, open event streams are closed.
func (s *Server) Watch(ctx context.Context) error {
	defer s.doneOnce.Do(func() { close(s.done) })

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("server: create watcher: %w", err)
	}
	defer w.Close()

	if err := s.addTree(w, s.dir); err != nil {
		return err
	}
	close(s.ready)

	timer := time.NewTimer(s.debounce)
	timer.Stop()
	defer timer.Stop()
	var changed []string

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			s.log.Warn("watcher error", "err", err)
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op == fsnotify.Chmod || s.isIgnored(ev.Name) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if err := s.addTree(w, ev.Name); err != nil {
						s.log.Warn("watch new directory", "err", err)
					}
				}
			}
			if !slices.Contains(changed, ev.Name) {
				changed = append(changed, ev.Name)
			}
			timer.Reset(s.debounce)
		case <-timer.C:
			s.log.Debug("source changed", "paths", changed)
			changed = changed[:0]
			_ = s.Rebuild() // failures are logged and broadcast by Rebuild
		}
	}
}

// addTree watches root and every non-ignored directory beneath it.
func (s *Server) addTree(w *fsnotify.Watcher, root string) error {
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if s.isIgnored(p) {
			return filepath.SkipDir
		}
		return w.Add(p)
	})
	if err != nil {
		return fmt.Errorf("server: watch %s: %w", root, err)
	}
	return nil
}

// isIgnored reports whether changes at the absolute path p should be skipped:
// paths outside Dir, hidden files or directories, and configured ignores.
func (s *Server) isIgnored(p string) bool {
	rel, err := filepath.Rel(s.dir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	if rel != "." {
		for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
			if strings.HasPrefix(part, ".") {
				return true
			}
		}
	}
	for _, ig := range s.ignore {
		if p == ig || strings.HasPrefix(p, ig+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
