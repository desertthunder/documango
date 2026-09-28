package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/desertthunder/documango/internal/theme"
)

// makeTarball builds an upstream tarball holding LICENSE and a base16 file
// per slug.
func makeTarball(t *testing.T, slugs ...string) []byte {
	t.Helper()
	files := map[string]string{"LICENSE": "MIT License"}
	for _, s := range slugs {
		files["base16/"+s+".yaml"] = "name: " + s
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "schemes-abc/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, ref string, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/tinted-theming/schemes/tarball/"+ref {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunSchemes(t *testing.T) {
	t.Parallel()
	curated := theme.Curated()
	base := serve(t, theme.DefaultRef, makeTarball(t, append(curated, "zenburn", "monokai")...))
	out := filepath.Join(t.TempDir(), "schemes")
	writeFiles(t, out, map[string]string{"stale.yaml": "old", "tomorrow.yaml": "old", "keep.txt": "keep"})

	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"schemes", "-base", base, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}

	want := []string{"LICENSE", "keep.txt"}
	for _, s := range curated {
		want = append(want, s+".yaml")
	}
	slices.Sort(want)
	if got := listDir(t, out); !slices.Equal(got, want) {
		t.Errorf("out dir = %v\nwant %v", got, want)
	}
	if data, _ := os.ReadFile(filepath.Join(out, "tomorrow.yaml")); string(data) != "name: tomorrow" {
		t.Errorf("tomorrow.yaml = %q, not replaced", data)
	}
	if want := "wrote 24 schemes"; len(curated) != 24 || !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".schemes-*")); len(leftovers) != 0 {
		t.Errorf("staging dirs left behind: %v", leftovers)
	}
}

func TestRunSchemesCreatesOutDir(t *testing.T) {
	t.Parallel()
	base := serve(t, "main", makeTarball(t, theme.Curated()...))
	out := filepath.Join(t.TempDir(), "new", "schemes")
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"schemes", "-ref", "main", "-base", base, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if got := len(listDir(t, out)); got != len(theme.Curated())+1 {
		t.Errorf("out dir has %d files", got)
	}
}

func TestRunSchemesFailures(t *testing.T) {
	t.Parallel()
	curated := theme.Curated()
	tests := []struct {
		name    string
		base    string
		wantErr string
	}{
		{name: "http error", base: serve(t, "other", nil), wantErr: "404"},
		{name: "curated scheme missing", base: serve(t, theme.DefaultRef, makeTarball(t, curated[1:]...)), wantErr: "curated schemes missing upstream: " + curated[0]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := filepath.Join(t.TempDir(), "schemes")
			writeFiles(t, out, map[string]string{"existing.yaml": "x"})

			var stdout, stderr bytes.Buffer
			if code := run(t.Context(), []string{"schemes", "-base", tt.base, "-out", out}, &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
			if got := listDir(t, out); !slices.Equal(got, []string{"existing.yaml"}) {
				t.Errorf("out dir changed on failure: %v", got)
			}
		})
	}
}

func TestRunUsage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "no subcommand", args: nil, wantCode: 2, wantErr: "usage"},
		{name: "unknown subcommand", args: []string{"bogus"}, wantCode: 2, wantErr: `unknown command "bogus"`},
		{name: "bad flag", args: []string{"schemes", "-nope"}, wantCode: 2, wantErr: "-nope"},
		{name: "extra args", args: []string{"schemes", "extra"}, wantCode: 2, wantErr: "unexpected arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := run(t.Context(), tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Errorf("exit %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}
