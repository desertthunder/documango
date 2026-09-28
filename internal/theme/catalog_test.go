package theme

import (
	"archive/tar"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const tarballPath = "/repos/tinted-theming/schemes/tarball/"

type tarEntry struct {
	name, body string
	typ        byte
}

func makeTarball(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.typ != 0 {
			hdr = &tar.Header{Name: e.name, Mode: 0o755, Typeflag: e.typ, Linkname: e.body}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.typ == 0 {
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

func named(name string) string {
	return strings.Replace(specYAML, "Tomorrow Night", name, 1)
}

// upstream is a fake tinted-theming/schemes tarball with two schemes that
// are not embedded, an override of an embedded slug and entries to skip.
func upstream(t *testing.T) []byte {
	t.Helper()
	return makeTarball(t,
		tarEntry{name: "tinted-theming-schemes-abc123/", typ: tar.TypeDir},
		tarEntry{name: "tinted-theming-schemes-abc123/LICENSE", body: "MIT License"},
		tarEntry{name: "tinted-theming-schemes-abc123/README.md", body: "readme"},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/zenburn.yaml", body: named("Zenburn")},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/monokai.yaml", body: named("Monokai")},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/tomorrow.yaml", body: named("Imposter")},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/broken.yaml", body: "palette: {}"},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/notes.txt", body: "skip"},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/nested/deep.yaml", body: named("Deep")},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/../../escape.yaml", body: named("Escape")},
		tarEntry{name: "tinted-theming-schemes-abc123/base16/link.yaml", body: "/etc/passwd", typ: tar.TypeSymlink},
		tarEntry{name: "tinted-theming-schemes-abc123/base24/dracula.yaml", body: named("Dracula 24")},
	)
}

// githubStub serves body at the API tarball path via a redirect, as GitHub
// does, and counts tarball downloads. A nil body fails every request.
type githubStub struct {
	*httptest.Server
	hits atomic.Int32
	req  atomic.Pointer[http.Request]
}

func newGitHub(t *testing.T, body []byte) *githubStub {
	t.Helper()
	g := &githubStub{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case body == nil:
			http.Error(w, "boom", http.StatusBadGateway)
		case strings.HasPrefix(r.URL.Path, tarballPath):
			g.req.Store(r)
			http.Redirect(w, r, "/codeload/"+strings.TrimPrefix(r.URL.Path, tarballPath), http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/codeload/"):
			g.hits.Add(1)
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.Close)
	return g
}

func slugs(schemes []Scheme) []string {
	out := make([]string, len(schemes))
	for i, s := range schemes {
		out[i] = s.Slug
	}
	return out
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

func quietLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestCurated(t *testing.T) {
	t.Parallel()
	got := Curated()
	if !slices.IsSorted(got) {
		t.Errorf("curated slugs not sorted: %v", got)
	}
	if embedded := slugs(Builtin()); !slices.Equal(got, embedded) {
		t.Errorf("curated %v\nembedded %v", got, embedded)
	}
	got[0] = "mutated"
	if Curated()[0] == "mutated" {
		t.Error("Curated() exposes shared slice")
	}
}

func TestCatalogDownload(t *testing.T) {
	gh := newGitHub(t, upstream(t))
	t.Setenv("GITHUB_TOKEN", "secret")
	dir := t.TempDir()
	c := &Catalog{BaseURL: gh.URL + "/", Ref: "main"}

	got, err := c.Download(t.Context(), dir, func(slug string) bool { return slug != "monokai" })
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"broken", "tomorrow", "zenburn"}; !slices.Equal(got, want) {
		t.Errorf("slugs = %v, want %v", got, want)
	}
	if files, want := listDir(t, dir), []string{"LICENSE", "broken.yaml", "tomorrow.yaml", "zenburn.yaml"}; !slices.Equal(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "zenburn.yaml")); string(data) != named("Zenburn") {
		t.Errorf("zenburn.yaml = %q", data)
	}

	req := gh.req.Load()
	if req.URL.Path != tarballPath+"main" {
		t.Errorf("path = %s", req.URL.Path)
	}
	for header, want := range map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
		"Authorization":        "Bearer secret",
	} {
		if got := req.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestCatalogDownloadNoToken(t *testing.T) {
	gh := newGitHub(t, upstream(t))
	t.Setenv("GITHUB_TOKEN", "")
	if _, err := (&Catalog{BaseURL: gh.URL}).Download(t.Context(), t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	if req := gh.req.Load(); req.Header.Get("Authorization") != "" || req.URL.Path != tarballPath+DefaultRef {
		t.Errorf("unexpected request %s with auth %q", req.URL.Path, req.Header.Get("Authorization"))
	}
}

func TestCatalogDownloadRejectedToken(t *testing.T) {
	body := upstream(t)
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_TOKEN", "expired")
	var logs bytes.Buffer
	c := &Catalog{BaseURL: srv.URL, Logger: quietLogger(&logs)}
	if _, err := c.Download(t.Context(), t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bearer expired", ""}; !slices.Equal(auths, want) {
		t.Errorf("Authorization headers = %q, want %q", auths, want)
	}
	if !strings.Contains(logs.String(), "GITHUB_TOKEN rejected") {
		t.Errorf("logs = %q, want rejected-token warning", logs.String())
	}
}

func TestCatalogDownloadErrors(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, tarEntry{name: "r/base16/a.yaml", body: "a"})
	status := func(code int) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	tests := []struct {
		name    string
		baseURL string
		dir     string
		wantErr string
	}{
		{name: "not found", baseURL: status(http.StatusNotFound), wantErr: "404"},
		{name: "unauthorized without token", baseURL: status(http.StatusUnauthorized), wantErr: "401"},
		{name: "rate limited", baseURL: status(http.StatusForbidden), wantErr: "GITHUB_TOKEN"},
		{name: "unreachable", baseURL: closed.URL, wantErr: "download schemes"},
		{name: "bad url", baseURL: "http://\x7f", wantErr: "download schemes"},
		{name: "not gzip", baseURL: newGitHub(t, []byte("plain text")).URL, wantErr: "gzip"},
		{name: "truncated", baseURL: newGitHub(t, tarball[:30]).URL, wantErr: "read schemes tarball"},
		{name: "no schemes", baseURL: newGitHub(t, makeTarball(t, tarEntry{name: "r/LICENSE", body: "MIT"})).URL, wantErr: "no base16 schemes"},
		{name: "no license", baseURL: newGitHub(t, tarball).URL, wantErr: "LICENSE"},
		{name: "unwritable dir", baseURL: newGitHub(t, tarball).URL, dir: filepath.Join(t.TempDir(), "missing"), wantErr: "create scheme file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := cmp.Or(tt.dir, t.TempDir())
			_, err := (&Catalog{BaseURL: tt.baseURL}).Download(t.Context(), dir, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCatalogAll(t *testing.T) {
	t.Parallel()
	gh := newGitHub(t, upstream(t))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var logs bytes.Buffer
	c := &Catalog{
		CacheDir: t.TempDir(),
		BaseURL:  gh.URL,
		Now:      func() time.Time { return now },
		Logger:   quietLogger(&logs),
	}

	all, err := c.All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := slices.Sorted(slices.Values(append(Curated(), "monokai", "zenburn")))
	if got := slugs(all); !slices.Equal(got, want) {
		t.Errorf("All() = %v\nwant %v", got, want)
	}
	if s, _ := find(all, "tomorrow"); s.Name != "Tomorrow" {
		t.Errorf("tomorrow = %q, embedded scheme should win", s.Name)
	}
	if s, _ := find(all, "zenburn"); s.Name != "Zenburn" || s.Variant != "dark" {
		t.Errorf("zenburn = %+v", s)
	}
	if !strings.Contains(logs.String(), "broken.yaml") {
		t.Errorf("logs = %q, want warning about broken.yaml", logs.String())
	}

	if got := listDir(t, c.CacheDir); !slices.Equal(got, []string{DefaultRef}) {
		t.Errorf("cache root = %v, want only %s", got, DefaultRef)
	}
	cached := listDir(t, filepath.Join(c.CacheDir, DefaultRef))
	if !slices.Contains(cached, stampFile) || !slices.Contains(cached, "zenburn.yaml") {
		t.Errorf("cache = %v", cached)
	}

	now = now.Add(DefaultMaxAge - time.Minute)
	if _, err := c.All(t.Context()); err != nil || gh.hits.Load() != 1 {
		t.Errorf("fresh cache: err %v, downloads %d, want 1", err, gh.hits.Load())
	}

	// A refresh replaces the cache in place and drops schemes gone upstream.
	stray := filepath.Join(c.CacheDir, DefaultRef, "stray.yaml")
	if err := os.WriteFile(stray, []byte(named("Stray")), 0o644); err != nil {
		t.Fatal(err)
	}
	next := newGitHub(t, makeTarball(t,
		tarEntry{name: "r/LICENSE", body: "MIT"},
		tarEntry{name: "r/base16/zenburn.yaml", body: named("Zenburn 2")},
	))
	c.BaseURL = next.URL
	now = now.Add(2 * time.Minute)
	all, err = c.All(t.Context())
	if err != nil || next.hits.Load() != 1 {
		t.Fatalf("stale cache: err %v, downloads %d, want 1", err, next.hits.Load())
	}
	if want := slices.Sorted(slices.Values(append(Curated(), "zenburn"))); !slices.Equal(slugs(all), want) {
		t.Errorf("after refresh All() = %v\nwant %v", slugs(all), want)
	}
	if s, _ := find(all, "zenburn"); s.Name != "Zenburn 2" {
		t.Errorf("zenburn = %q, want refreshed copy", s.Name)
	}
	if got := listDir(t, c.CacheDir); !slices.Equal(got, []string{DefaultRef}) {
		t.Errorf("refresh left %v behind", got)
	}
}

func TestCatalogAllStaleOffline(t *testing.T) {
	t.Parallel()
	gh := newGitHub(t, upstream(t))
	now := time.Now()
	var logs bytes.Buffer
	c := &Catalog{CacheDir: t.TempDir(), BaseURL: gh.URL, MaxAge: time.Hour, Now: func() time.Time { return now }, Logger: quietLogger(&logs)}
	if _, err := c.All(t.Context()); err != nil {
		t.Fatal(err)
	}

	c.BaseURL = newGitHub(t, nil).URL
	now = now.Add(2 * time.Hour)
	all, err := c.All(t.Context())
	if err != nil {
		t.Fatalf("stale cache offline: %v", err)
	}
	if !slices.Contains(slugs(all), "zenburn") {
		t.Errorf("stale cache not used: %v", slugs(all))
	}
	if !strings.Contains(logs.String(), "stale theme cache") || !strings.Contains(logs.String(), "502") {
		t.Errorf("logs = %q, want stale warning with cause", logs.String())
	}
}

func TestCatalogAllUnavailable(t *testing.T) {
	t.Parallel()
	offline := newGitHub(t, nil).URL
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		c       Catalog
		wantErr string
	}{
		{name: "offline without cache", c: Catalog{CacheDir: t.TempDir(), BaseURL: offline}, wantErr: "502"},
		{name: "cache dir is a file", c: Catalog{CacheDir: file, BaseURL: offline}, wantErr: "create theme cache"},
		{name: "invalid ref", c: Catalog{CacheDir: t.TempDir(), Ref: "..", BaseURL: offline}, wantErr: "invalid scheme ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			all, err := tt.c.All(t.Context())
			if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want ErrUnavailable with %q", err, tt.wantErr)
			}
			if !slices.Equal(slugs(all), Curated()) {
				t.Errorf("All() = %v, want embedded schemes", slugs(all))
			}
		})
	}
}

func TestCatalogDefaultCacheDir(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("cache dir comes from HOME and XDG_CACHE_HOME only on unix")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	gh := newGitHub(t, upstream(t))
	if _, err := (&Catalog{BaseURL: gh.URL}).All(t.Context()); err != nil {
		t.Fatal(err)
	}
	root, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "documango", "schemes", DefaultRef, "zenburn.yaml")); err != nil {
		t.Errorf("default cache not written: %v", err)
	}

	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if _, err := (&Catalog{BaseURL: gh.URL}).All(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable without a cache dir", err)
	}
}

func TestCatalogLoad(t *testing.T) {
	t.Parallel()
	gh := newGitHub(t, upstream(t))
	discard := slog.New(slog.DiscardHandler)
	online := &Catalog{CacheDir: t.TempDir(), BaseURL: gh.URL, Logger: discard}
	offline := &Catalog{CacheDir: t.TempDir(), BaseURL: newGitHub(t, nil).URL, Logger: discard}
	path := filepath.Join(t.TempDir(), "mine.yaml")
	if err := os.WriteFile(path, []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		c        *Catalog
		in       string
		wantSlug string
		wantName string
		wantErr  []string
		offline  bool
	}{
		{name: "embedded", c: offline, in: "Tomorrow-Night", wantSlug: "tomorrow-night", wantName: "Tomorrow Night"},
		{name: "file", c: offline, in: path, wantSlug: "mine", wantName: "Tomorrow Night"},
		{name: "missing file", c: offline, in: "missing.yml", wantErr: []string{"missing.yml"}},
		{name: "cached", c: online, in: "ZenBurn", wantSlug: "zenburn", wantName: "Zenburn"},
		{name: "unknown suggests cached", c: online, in: "zenbern", wantErr: []string{`unknown theme "zenbern"`, "did you mean zenburn"}},
		{name: "unknown offline", c: offline, in: "zenburn", wantErr: []string{`unknown theme "zenburn"`, "502"}, offline: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := tt.c.Load(context.Background(), tt.in)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Load(%q) succeeded", tt.in)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
				if errors.Is(err, ErrUnavailable) != tt.offline {
					t.Errorf("errors.Is(err, ErrUnavailable) = %v, want %v", !tt.offline, tt.offline)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.Slug != tt.wantSlug || s.Name != tt.wantName {
				t.Errorf("Load(%q) = %q %q, want %q %q", tt.in, s.Slug, s.Name, tt.wantSlug, tt.wantName)
			}
		})
	}
}

// TestCatalogGitHub downloads the real catalog. Run with DOCUMANGO_E2E=1.
func TestCatalogGitHub(t *testing.T) {
	if os.Getenv("DOCUMANGO_E2E") != "1" {
		t.Skip("set DOCUMANGO_E2E=1 to download schemes from GitHub")
	}
	c := &Catalog{CacheDir: t.TempDir()}
	for _, pass := range []string{"download", "cached"} {
		start := time.Now()
		all, err := c.All(t.Context())
		if err != nil {
			t.Fatalf("%s: %v", pass, err)
		}
		t.Logf("%s: %d schemes in %v", pass, len(all), time.Since(start))
		if len(all) < 200 {
			t.Errorf("%s: only %d schemes", pass, len(all))
		}
	}
}
