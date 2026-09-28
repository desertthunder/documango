package pagefind

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeModeEnv switches the test binary into a fake pagefind; see TestMain.
const fakeModeEnv = "DOCUMANGO_FAKE_PAGEFIND"

// TestMain lets the test binary stand in for pagefind when fakeModeEnv is set.
func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeModeEnv); mode != "" {
		os.Exit(fakePagefind(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakePagefind mimics `pagefind --site <dir> --output-subdir <sub>`. It writes
// a bundle listing the HTML pages it saw and the site dir it ran in.
func fakePagefind(mode string, args []string) int {
	switch mode {
	case "fail":
		fmt.Fprintln(os.Stderr, "  Error: Pagefind was not able to build an index.  ")
		return 1
	case "hang":
		time.Sleep(time.Minute)
		return 0
	case "nobundle":
		return 0
	}
	var site, sub string
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--site":
			site = args[i+1]
		case "--output-subdir":
			sub = args[i+1]
		}
	}
	var pages []string
	_ = filepath.WalkDir(site, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(site, p)
			pages = append(pages, filepath.ToSlash(rel))
		}
		return nil
	})
	out := filepath.Join(site, sub)
	if err := os.MkdirAll(filepath.Join(out, "fragment"), 0o755); err != nil {
		return 2
	}
	slices.Sort(pages)
	for name, data := range map[string]string{
		"pagefind.js":               "export const search = () => {}",
		"pagefind-entry.json":       `{"version":"` + Version + `"}`,
		"fragment/en_1.pf_fragment": strings.Join(pages, "\n"),
		"site.txt":                  site,
	} {
		if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(name)), []byte(data), 0o644); err != nil {
			return 2
		}
	}
	return 0
}

func TestIndex(t *testing.T) {
	t.Setenv(fakeModeEnv, "ok")
	files := map[string][]byte{
		"index.html":          []byte("<html><body>home</body></html>"),
		"guide/install.html":  []byte("<html><body>install</body></html>"),
		"assets/site.css":     []byte("body{}"),
		"pagefind/stale.html": []byte("old"),
	}

	got, err := Index(context.Background(), os.Args[0], files)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}

	keys := slices.Sorted(maps.Keys(got))
	want := []string{"pagefind/fragment/en_1.pf_fragment", "pagefind/pagefind-entry.json", "pagefind/pagefind.js", "pagefind/site.txt"}
	if !slices.Equal(keys, want) {
		t.Errorf("bundle keys = %v, want %v", keys, want)
	}
	if pages := string(got["pagefind/fragment/en_1.pf_fragment"]); pages != "guide/install.html\nindex.html" {
		t.Errorf("staged pages = %q, want only the HTML pages", pages)
	}
	if _, err := os.Stat(string(got["pagefind/site.txt"])); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temp site dir not removed: stat err = %v", err)
	}
}

func TestIndexErrors(t *testing.T) {
	page := map[string][]byte{"index.html": []byte("<body>x</body>")}
	tests := []struct {
		name    string
		mode    string
		bin     string
		files   map[string][]byte
		timeout time.Duration
		want    string
	}{
		{name: "binary output surfaced", mode: "fail", files: page, want: "exit status 1: Error: Pagefind was not able to build an index."},
		{name: "no html pages", mode: "ok", files: map[string][]byte{"a.css": nil}, want: "no HTML pages"},
		{name: "path escapes site", mode: "ok", files: map[string][]byte{"../evil.html": nil}, want: `invalid site path "../evil.html"`},
		{name: "missing bundle", mode: "nobundle", files: page, want: "read bundle"},
		{name: "missing binary", mode: "ok", bin: filepath.Join(t.TempDir(), "nope"), files: page, want: "index with"},
		{name: "context cancelled", mode: "hang", files: page, timeout: 100 * time.Millisecond, want: context.DeadlineExceeded.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(fakeModeEnv, tt.mode)
			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}
			bin := tt.bin
			if bin == "" {
				bin = os.Args[0]
			}
			_, err := Index(ctx, bin, tt.files)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Index error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestIndexE2E downloads the real pagefind release and indexes a small site.
// It needs network access and runs only when DOCUMANGO_E2E=1.
func TestIndexE2E(t *testing.T) {
	if os.Getenv("DOCUMANGO_E2E") != "1" {
		t.Skip("set DOCUMANGO_E2E=1 to download and run the real pagefind")
	}
	f := &Finder{
		CacheDir: t.TempDir(),
		Getenv:   func(string) string { return "" },
		LookPath: func(string) (string, error) { return "", errors.New("not on PATH") },
	}
	ctx := context.Background()
	start := time.Now()
	bin, err := f.Find(ctx)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	t.Logf("installed %s in %s", bin, time.Since(start))

	page := func(title, body string) []byte {
		return []byte(`<!doctype html><html lang="en"><head><title>` + title + `</title></head><body><nav>menu</nav><main data-pagefind-body><h1>` + title + `</h1><p>` + body + `</p></main></body></html>`)
	}
	files := map[string][]byte{
		"index.html":         page("Home", "Welcome to documango."),
		"guide/install.html": page("Install", "Install documango with go install."),
		"assets/site.css":    []byte("body{}"),
	}
	start = time.Now()
	bundle, err := Index(ctx, bin, files)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	t.Logf("indexed 2 pages into %d files in %s", len(bundle), time.Since(start))
	start = time.Now()
	if _, err := Index(ctx, bin, files); err != nil {
		t.Fatalf("second Index: %v", err)
	}
	t.Logf("re-indexed in %s", time.Since(start))
	for _, key := range []string{"pagefind/pagefind.js", "pagefind/pagefind-entry.json"} {
		if len(bundle[key]) == 0 {
			t.Errorf("bundle missing %s", key)
		}
	}
	if !strings.Contains(string(bundle["pagefind/pagefind-entry.json"]), `"page_count":2`) {
		t.Errorf("pagefind-entry.json = %s, want page_count 2", bundle["pagefind/pagefind-entry.json"])
	}
}
