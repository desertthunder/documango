package pagefind

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTarget(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "aarch64-apple-darwin"},
		{"darwin", "amd64", "x86_64-apple-darwin"},
		{"linux", "arm64", "aarch64-unknown-linux-musl"},
		{"linux", "amd64", "x86_64-unknown-linux-musl"},
		{"windows", "arm64", "aarch64-pc-windows-msvc"},
		{"windows", "amd64", "x86_64-pc-windows-msvc"},
		{"linux", "386", ""},
		{"freebsd", "amd64", ""},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got, err := Target(tt.goos, tt.goarch)
			if tt.want == "" {
				if err == nil || !strings.Contains(err.Error(), EnvBinary) {
					t.Fatalf("Target error = %v, want one suggesting %s", err, EnvBinary)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Target = %q, %v; want %q", got, err, tt.want)
			}
			if len(checksums[got]) != 64 {
				t.Errorf("no pinned checksum for %s", got)
			}
		})
	}
}

// tarball builds a gzipped tar archive with the given regular file entries.
func tarball(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, body); err != nil {
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

// releaseServer serves archive for every request and counts the requests.
func releaseServer(t *testing.T, status int, archive []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/pagefind-v"+Version+"-aarch64-apple-darwin.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// offlineFinder returns a Finder with no env override, nothing on PATH, and a
// darwin/arm64 platform whose pinned checksum is that of archive.
func offlineFinder(t *testing.T, baseURL string, archive []byte) *Finder {
	t.Helper()
	sum := sha256.Sum256(archive)
	return &Finder{
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Getenv:    func(string) string { return "" },
		LookPath:  func(string) (string, error) { return "", errors.New("not found") },
		BaseURL:   baseURL,
		Logger:    slog.New(slog.DiscardHandler),
		goos:      "darwin",
		goarch:    "arm64",
		checksums: map[string]string{"aarch64-apple-darwin": hex.EncodeToString(sum[:])},
	}
}

func TestFindPrecedence(t *testing.T) {
	dir := t.TempDir()
	envBin := filepath.Join(dir, "env-pagefind")
	if err := os.WriteFile(envBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := tarball(t, map[string]string{"pagefind": "downloaded"})

	tests := []struct {
		name    string
		env     string
		path    string
		cached  bool
		want    func(f *Finder) string
		wantErr string
		hits    int
	}{
		{name: "env wins", env: envBin, path: "/usr/bin/pagefind", cached: true, want: func(*Finder) string { return envBin }},
		{name: "env missing file", env: filepath.Join(dir, "nope"), path: "/usr/bin/pagefind", wantErr: EnvBinary},
		{name: "env is a directory", env: dir, wantErr: "is a directory"},
		{name: "PATH before cache", path: "/usr/bin/pagefind", cached: true, want: func(*Finder) string { return "/usr/bin/pagefind" }},
		{name: "cache before download", cached: true, want: func(f *Finder) string { return filepath.Join(f.CacheDir, "pagefind") }},
		{name: "download", want: func(f *Finder) string { return filepath.Join(f.CacheDir, "pagefind") }, hits: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, hits := releaseServer(t, http.StatusOK, archive)
			f := offlineFinder(t, srv.URL, archive)
			f.Getenv = func(k string) string {
				if k == EnvBinary {
					return tt.env
				}
				return ""
			}
			if tt.path != "" {
				f.LookPath = func(string) (string, error) { return tt.path, nil }
			}
			if tt.cached {
				if err := os.MkdirAll(f.CacheDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(f.CacheDir, "pagefind"), []byte("cached"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			got, err := f.Find(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Find error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			if want := tt.want(f); got != want {
				t.Errorf("Find = %q, want %q", got, want)
			}
			if int(hits.Load()) != tt.hits {
				t.Errorf("downloads = %d, want %d", hits.Load(), tt.hits)
			}
		})
	}
}

func TestFindDownload(t *testing.T) {
	archive := tarball(t, map[string]string{"README.md": "docs", "./pagefind": "binary"})
	srv, hits := releaseServer(t, http.StatusOK, archive)
	var logs bytes.Buffer
	f := offlineFinder(t, srv.URL+"/", archive)
	f.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	bin, err := f.Find(context.Background())
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	data, err := os.ReadFile(bin)
	if err != nil || string(data) != "binary" {
		t.Fatalf("installed binary = %q, %v; want %q", data, err, "binary")
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed binary mode = %v, want executable", info.Mode())
	}
	entries, err := os.ReadDir(f.CacheDir)
	if err != nil || len(entries) != 1 {
		t.Errorf("cache dir entries = %v, %v; want only the binary", entries, err)
	}
	if !strings.Contains(logs.String(), "downloading pagefind "+Version) {
		t.Errorf("log = %q, want a download message", logs.String())
	}

	if again, err := f.Find(context.Background()); err != nil || again != bin || hits.Load() != 1 {
		t.Errorf("second Find = %q, %v with %d downloads; want cached %q with 1 download", again, err, hits.Load(), bin)
	}
}

func TestFindWindowsExe(t *testing.T) {
	archive := tarball(t, map[string]string{"pagefind": "wrong", "pagefind.exe": "binary"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	f := offlineFinder(t, srv.URL, archive)
	f.goos, f.goarch = "windows", "amd64"
	f.checksums = map[string]string{"x86_64-pc-windows-msvc": f.checksums["aarch64-apple-darwin"]}

	bin, err := f.Find(context.Background())
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if filepath.Base(bin) != "pagefind.exe" {
		t.Errorf("Find = %q, want pagefind.exe", bin)
	}
	if data, _ := os.ReadFile(bin); string(data) != "binary" {
		t.Errorf("installed %q, want the .exe entry", data)
	}
}

func TestFindDownloadErrors(t *testing.T) {
	good := tarball(t, map[string]string{"pagefind": "binary"})
	tests := []struct {
		name    string
		status  int
		served  []byte
		pinned  []byte
		setup   func(f *Finder)
		wantErr string
	}{
		{name: "http error", status: http.StatusNotFound, served: good, pinned: good, wantErr: "404 Not Found"},
		{name: "checksum mismatch", status: http.StatusOK, served: good, pinned: []byte("other"), wantErr: "checksum mismatch"},
		{name: "binary missing", status: http.StatusOK, served: tarball(t, map[string]string{"LICENSE": "x"}), wantErr: "has no pagefind entry"},
		{name: "not gzip", status: http.StatusOK, served: []byte("plain"), wantErr: "read archive"},
		{name: "corrupt tar", status: http.StatusOK, served: gzipped(t, "not a tar archive at all"), wantErr: "read archive"},
		{name: "unsupported platform", status: http.StatusOK, served: good, setup: func(f *Finder) { f.goos = "plan9" }, wantErr: "set " + EnvBinary},
		{name: "no pinned checksum", status: http.StatusOK, served: good, setup: func(f *Finder) { f.checksums = map[string]string{} }, wantErr: "no pinned checksum"},
		{name: "bad base url", status: http.StatusOK, served: good, setup: func(f *Finder) { f.BaseURL = "::" }, wantErr: "build request"},
		{name: "unreachable", status: http.StatusOK, served: good, setup: func(f *Finder) { f.BaseURL = "http://127.0.0.1:1" }, wantErr: "download"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := releaseServer(t, tt.status, tt.served)
			pinned := tt.pinned
			if pinned == nil {
				pinned = tt.served
			}
			f := offlineFinder(t, srv.URL, pinned)
			if tt.setup != nil {
				tt.setup(f)
			}

			_, err := f.Find(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Find error = %v, want it to contain %q", err, tt.wantErr)
			}
			if _, err := os.Stat(f.CacheDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("cache dir exists after failed download: %v", err)
			}
		})
	}
}

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := io.WriteString(gz, s); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstall(t *testing.T) {
	t.Run("replaces existing binary atomically", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "pagefind")
		if err := os.WriteFile(bin, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := install(dir, bin, []byte("new")); err != nil {
			t.Fatalf("install: %v", err)
		}
		if data, _ := os.ReadFile(bin); string(data) != "new" {
			t.Errorf("binary = %q, want %q", data, "new")
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Errorf("leftover files: %v", entries)
		}
	})
	t.Run("cache dir is a file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(dir, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := install(dir, filepath.Join(dir, "pagefind"), nil); err == nil || !strings.Contains(err.Error(), "create cache dir") {
			t.Fatalf("install error = %v, want cache dir error", err)
		}
	})
	t.Run("rename target is a directory", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "pagefind")
		if err := os.MkdirAll(filepath.Join(bin, "child"), 0o755); err != nil {
			t.Fatal(err)
		}
		// The existing (non-empty) directory blocks the rename but satisfies
		// the "someone else installed it" check, so install succeeds.
		if err := install(dir, bin, []byte("new")); err != nil {
			t.Fatalf("install: %v", err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Errorf("leftover temp files: %v", entries)
		}
	})
}

func TestFindDefaults(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookup needs an .exe on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "pagefind")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	t.Setenv(EnvBinary, "")
	if got, err := new(Finder).Find(context.Background()); err != nil || got != bin {
		t.Errorf("Find via PATH = %q, %v; want %q", got, err, bin)
	}

	t.Setenv("PATH", "")
	t.Setenv(EnvBinary, bin)
	if got, err := new(Finder).Find(context.Background()); err != nil || got != bin {
		t.Errorf("Find via %s = %q, %v; want %q", EnvBinary, got, err, bin)
	}
}
