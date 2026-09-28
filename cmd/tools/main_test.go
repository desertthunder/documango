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
)

type tarEntry struct {
	name string
	body string
	dir  bool
}

func makeTarball(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.dir {
			hdr = &tar.Header{Name: e.name, Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !e.dir {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
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

func serve(t *testing.T, wantPath string, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
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

func TestRunSchemes(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, []tarEntry{
		{name: "schemes-spec-0.11/", dir: true},
		{name: "schemes-spec-0.11/LICENSE", body: "MIT License"},
		{name: "schemes-spec-0.11/README.md", body: "readme"},
		{name: "schemes-spec-0.11/base16/", dir: true},
		{name: "schemes-spec-0.11/base16/tomorrow.yaml", body: "name: Tomorrow"},
		{name: "schemes-spec-0.11/base16/tomorrow-night.yaml", body: "name: Tomorrow Night"},
		{name: "schemes-spec-0.11/base16/notes.txt", body: "skip"},
		{name: "schemes-spec-0.11/base16/nested/deep.yaml", body: "skip"},
		{name: "schemes-spec-0.11/base24/dracula.yaml", body: "skip"},
	})
	srv := serve(t, "/tar.gz/spec-0.11", tarball)

	out := filepath.Join(t.TempDir(), "schemes")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"stale.yaml": "old", "tomorrow.yaml": "old", "keep.txt": "keep"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"schemes", "-base", srv.URL + "/tar.gz", "-out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}

	got := listDir(t, out)
	want := []string{"LICENSE", "keep.txt", "tomorrow-night.yaml", "tomorrow.yaml"}
	if !slices.Equal(got, want) {
		t.Errorf("out dir = %v, want %v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(out, "tomorrow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "name: Tomorrow" {
		t.Errorf("tomorrow.yaml = %q, not replaced", data)
	}
	if !strings.Contains(stdout.String(), "2 schemes") {
		t.Errorf("stdout = %q, want scheme count", stdout.String())
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".schemes-*")); len(leftovers) != 0 {
		t.Errorf("staging dirs left behind: %v", leftovers)
	}
}

func TestRunSchemesCreatesOutDir(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, []tarEntry{
		{name: "root/LICENSE", body: "MIT"},
		{name: "root/base16/a.yaml", body: "a"},
	})
	srv := serve(t, "/tar.gz/main", tarball)
	out := filepath.Join(t.TempDir(), "new", "schemes")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"schemes", "-ref", "main", "-base", srv.URL + "/tar.gz", "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if got := listDir(t, out); !slices.Equal(got, []string{"LICENSE", "a.yaml"}) {
		t.Errorf("out dir = %v", got)
	}
}

func TestRunSchemesFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		body    []byte
		wantErr string
	}{
		{name: "not gzip", body: []byte("plain text"), wantErr: "gzip"},
		{name: "no schemes", body: makeTarball(t, []tarEntry{{name: "r/LICENSE", body: "MIT"}}), wantErr: "no base16 schemes"},
		{name: "no license", body: makeTarball(t, []tarEntry{{name: "r/base16/a.yaml", body: "a"}}), wantErr: "LICENSE"},
		{name: "truncated tar", body: makeTarball(t, []tarEntry{{name: "r/base16/a.yaml", body: "a"}})[:30], wantErr: "read tarball"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := serve(t, "/tar.gz/spec-0.11", tt.body)
			out := filepath.Join(t.TempDir(), "schemes")
			if err := os.MkdirAll(out, 0o755); err != nil {
				t.Fatal(err)
			}
			existing := filepath.Join(out, "existing.yaml")
			if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			code := run([]string{"schemes", "-base", srv.URL + "/tar.gz", "-out", out}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
			if _, err := os.Stat(existing); err != nil {
				t.Errorf("existing scheme removed on failure: %v", err)
			}
		})
	}
}

func TestRunSchemesHTTPError(t *testing.T) {
	t.Parallel()
	srv := serve(t, "/nothing", nil)
	var stdout, stderr bytes.Buffer
	code := run([]string{"schemes", "-base", srv.URL, "-out", t.TempDir()}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "404") {
		t.Errorf("exit %d, stderr %q; want 1 and 404", code, stderr.String())
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
			if code := run(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Errorf("exit %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestRunSchemesUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"schemes", "-base", srv.URL, "-out", t.TempDir()}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "download") {
		t.Errorf("exit %d, stderr %q; want 1 and download error", code, stderr.String())
	}
}
