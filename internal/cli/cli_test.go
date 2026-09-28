package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/desertthunder/documango/internal/theme"
)

// syncBuffer is a bytes.Buffer safe for a writer goroutine and a reading test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestMain lets the test binary stand in for pagefind: run with --site, it
// writes a small bundle listing the pages it was given.
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "--site" {
		os.Exit(fakePagefind(os.Args[2]))
	}
	os.Exit(m.Run())
}

func fakePagefind(site string) int {
	var pages []string
	_ = filepath.WalkDir(site, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(site, p)
			pages = append(pages, filepath.ToSlash(rel))
		}
		return nil
	})
	out := filepath.Join(site, "pagefind")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return 2
	}
	entry, _ := json.Marshal(map[string][]string{"pages": pages})
	if os.WriteFile(filepath.Join(out, "pagefind-entry.json"), entry, 0o644) != nil ||
		os.WriteFile(filepath.Join(out, "pagefind.js"), []byte("export {}"), 0o644) != nil {
		return 2
	}
	return 0
}

// testEnv returns an environment that keeps the CLI offline: a cache folder
// holding a fresh copy of the theme list with one downloaded theme,
// remote-dusk, and a stand-in pagefind. On Unix that is a shell script, since
// starting the test binary costs over a second per build on macOS.
func testEnv(t *testing.T) []string {
	t.Helper()
	cache := t.TempDir()
	schemes := filepath.Join(cache, "schemes", theme.DefaultRef)
	writeFile(t, filepath.Join(schemes, ".fetched"), time.Now().UTC().Format(time.RFC3339Nano))
	writeFile(t, filepath.Join(schemes, "remote-dusk.yaml"), "system: base16\nname: Remote Dusk\nvariant: dark\npalette:\n"+paletteYAML())
	bin := fakePagefindBin(t)
	if runtime.GOOS != "windows" {
		bin = filepath.Join(cache, "pagefind.sh")
		writeFile(t, bin, "#!/bin/sh\nmkdir -p \"$2/pagefind\" && printf 'export {}' > \"$2/pagefind/pagefind.js\"\n")
		if err := os.Chmod(bin, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return []string{"DOCUMANGO_CACHE_DIR=" + cache, "DOCUMANGO_PAGEFIND=" + bin}
}

// fakePagefindBin returns the test binary, which acts as pagefind (see
// TestMain) and records the pages it indexed.
func fakePagefindBin(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

type result struct {
	code           int
	stdout, stderr string
}

// run executes the CLI in-process with testEnv followed by environ.
func run(t *testing.T, environ []string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, Env{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Environ: append(testEnv(t), environ...),
		Version: "1.2.3",
	})
	return result{code, stdout.String(), stderr.String()}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// docsDir returns a temporary docs directory with a home page and one guide.
func docsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.md"), "# Hello Docs\n\nWelcome.\n")
	writeFile(t, filepath.Join(dir, "guide", "install.md"), "# Install\n\nRun it.\n")
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"--help"}, []string{"Usage:", "Examples:", "documango build", "serve", "themes", "--dark-theme", "--port", "--quiet", "--no-color"}},
		{[]string{"help", "build"}, []string{"Examples:", "-o, --output", "--clean", "--base-path", "--title"}},
		{[]string{"serve", "-h"}, []string{"Examples:", "--port", "--host", "--open", "--light-theme"}},
		{[]string{"themes", "--help"}, []string{"Examples:", "--variant", "--offline", "pick"}},
		{[]string{"themes", "pick", "--help"}, []string{"Examples:", "--multi", "--variant", "--offline", "themes pick --variant dark"}},
	} {
		r := run(t, nil, tc.args...)
		if r.code != 0 {
			t.Errorf("%v: exit %d, stderr %q", tc.args, r.code, r.stderr)
		}
		for _, w := range tc.want {
			if !strings.Contains(r.stdout, w) {
				t.Errorf("%v: help lacks %q:\n%s", tc.args, w, r.stdout)
			}
		}
		if r.stderr != "" {
			t.Errorf("%v: stderr = %q, want empty", tc.args, r.stderr)
		}
	}
}

func TestVersion(t *testing.T) {
	r := run(t, nil, "--version")
	if r.code != 0 || r.stdout != "documango 1.2.3\n" {
		t.Errorf("--version = %d %q, want 0 %q", r.code, r.stdout, "documango 1.2.3\n")
	}
	if got := versionString(""); got == "" {
		t.Error("versionString(\"\") is empty, want a fallback")
	}
	if got := versionString("v9"); got != "v9" {
		t.Errorf("versionString(v9) = %q", got)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		msg, cmd string
	}{
		{[]string{"--bogus"}, "unknown flag: --bogus", "documango --help"},
		{[]string{"build", "--bogus"}, "unknown flag: --bogus", "documango build --help"},
		{[]string{"build", "a", "b"}, "accepts at most 1 arg", "documango build --help"},
		{[]string{"serve", "--port", "abc"}, "invalid argument", "documango serve --help"},
		{[]string{"themes", "extra"}, "unknown command", "documango themes --help"},
		{[]string{"themes", "--variant", "blue"}, `invalid --variant "blue"`, "documango themes --help"},
		{[]string{"-q", "-v", "themes"}, "--quiet and --verbose", "documango themes --help"},
	} {
		r := run(t, nil, tc.args...)
		if r.code != 2 {
			t.Errorf("%v: exit %d, want 2 (stderr %q)", tc.args, r.code, r.stderr)
		}
		if !strings.HasPrefix(r.stderr, "Error: ") || !strings.Contains(r.stderr, tc.msg) {
			t.Errorf("%v: stderr %q lacks %q", tc.args, r.stderr, tc.msg)
		}
		if !strings.Contains(r.stderr, "Run '"+tc.cmd+"' for usage.") {
			t.Errorf("%v: stderr %q lacks usage hint for %q", tc.args, r.stderr, tc.cmd)
		}
		if r.stdout != "" {
			t.Errorf("%v: stdout = %q, want empty", tc.args, r.stdout)
		}
	}
}

func TestDirErrors(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "file.md")
	writeFile(t, file, "# x")
	empty := filepath.Join(tmp, "empty")
	writeFile(t, filepath.Join(empty, "notes.txt"), "not markdown")

	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"serve", filepath.Join(tmp, "nope")}, []string{"does not exist"}},
		{[]string{"build", file}, []string{"is not a directory"}},
		{[]string{"build", empty, "-o", filepath.Join(tmp, "out")}, []string{"no Markdown files found in " + empty, "Hint: "}},
		{[]string{"biuld"}, []string{`"biuld" does not exist`, `did you mean "build"?`}},
		{[]string{"serv"}, []string{`did you mean "serve"?`}},
	} {
		r := run(t, nil, tc.args...)
		if r.code != 1 {
			t.Errorf("%v: exit %d, want 1 (stderr %q)", tc.args, r.code, r.stderr)
		}
		for _, w := range tc.want {
			if !strings.Contains(r.stderr, w) {
				t.Errorf("%v: stderr %q lacks %q", tc.args, r.stderr, w)
			}
		}
	}
	if exists(filepath.Join(tmp, "out")) {
		t.Error("failed build created the output directory")
	}
}

func TestBuild(t *testing.T) {
	dir := docsDir(t)
	writeFile(t, filepath.Join(dir, "img", "logo.png"), "png")
	out := filepath.Join(t.TempDir(), "site")

	r := run(t, nil, "build", dir, "-o", out, "--title", "My Docs", "--base-path", "/docs/", "--light-theme", "github")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	for _, f := range []string{"index.html", "guide/install/index.html", "404.html", "img/logo.png", "_documango/style.css", "_documango/app.js", "_documango/search.json"} {
		if !exists(filepath.Join(out, f)) {
			t.Errorf("missing %s", f)
		}
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, w := range []string{"My Docs", "/docs/_documango/style.css", `content="documango 1.2.3"`} {
		if !bytes.Contains(home, []byte(w)) {
			t.Errorf("index.html lacks %q", w)
		}
	}
	if bytes.Contains(home, []byte("_documango/events")) {
		t.Error("static build enables live reload")
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
	if !regexp.MustCompile(`^Built 2 pages to .+site in \S+\n$`).MatchString(r.stderr) {
		t.Errorf("summary = %q", r.stderr)
	}

	if r := run(t, nil, "build", dir, "-o", out, "-q"); r.code != 0 || r.stderr != "" {
		t.Errorf("quiet build: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestBuildSinglePage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README.md"), "Just text.\n")
	out := filepath.Join(t.TempDir(), "out")
	r := run(t, nil, "build", dir, "--output", out)
	if r.code != 0 || !strings.HasPrefix(r.stderr, "Built 1 page to ") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !bytes.Contains(home, []byte("Documentation")) {
		t.Error("default title missing")
	}
}

func TestBuildOutFlag(t *testing.T) {
	dir := docsDir(t)
	out := filepath.Join(t.TempDir(), "site")
	r := run(t, nil, "build", dir, "--out", out)
	if r.code != 0 || !exists(filepath.Join(out, "index.html")) {
		t.Fatalf("--out: exit %d, stderr %q", r.code, r.stderr)
	}
	if !strings.Contains(r.stderr, "WARN --out is deprecated; use --output\n") || strings.Count(r.stderr, "deprecated") != 1 {
		t.Errorf("--out notice missing or repeated: %q", r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
	if r := run(t, nil, "help", "build"); strings.Contains(r.stdout, "--out ") {
		t.Errorf("help lists the deprecated --out:\n%s", r.stdout)
	}

	r = run(t, nil, "build", dir, "--out", out, "-o", out)
	if r.code != 2 || !strings.Contains(r.stderr, "--out and --output cannot be used together") ||
		!strings.Contains(r.stderr, "Run 'documango build --help' for usage.") {
		t.Errorf("--out with -o: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestBuildOutputInsideDocs(t *testing.T) {
	dir := docsDir(t)
	out := filepath.Join(dir, "public")
	writeFile(t, filepath.Join(out, "stray.md"), "# Stray\n")

	for range 2 {
		if r := run(t, nil, "build", dir, "-o", out); r.code != 0 {
			t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
		}
	}
	if exists(filepath.Join(out, "public")) || exists(filepath.Join(out, "stray", "index.html")) {
		t.Error("output directory was included in the site")
	}

	// The default _site output is skipped by its leading underscore.
	t.Chdir(dir)
	for range 2 {
		if r := run(t, nil, "build"); r.code != 0 {
			t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
		}
	}
	if !exists(filepath.Join(dir, "_site", "index.html")) || exists(filepath.Join(dir, "_site", "_site")) {
		t.Error("default output directory built incorrectly")
	}
}

func TestBuildRefusesDocsDirOrAncestor(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "docs")
	writeFile(t, filepath.Join(dir, "index.md"), "# Home\n")
	for _, out := range []string{dir, parent} {
		r := run(t, nil, "build", dir, "-o", out, "--clean")
		if r.code != 1 || !strings.Contains(r.stderr, "cannot build into") || !strings.Contains(r.stderr, "Hint: Use --output") {
			t.Errorf("-o %s: exit %d, stderr %q", out, r.code, r.stderr)
		}
	}
	if !exists(filepath.Join(dir, "index.md")) {
		t.Fatal("docs were deleted")
	}
}

func TestBuildSymlinkedDocs(t *testing.T) {
	docs := docsDir(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(docs, link); err != nil {
		t.Fatal(err)
	}

	r := run(t, nil, "build", link, "-o", docs)
	if r.code != 1 || !strings.Contains(r.stderr, "cannot build into") || !strings.Contains(r.stderr, "Hint: Use --output") {
		t.Errorf("-o docs via symlink: exit %d, stderr %q", r.code, r.stderr)
	}
	if exists(filepath.Join(docs, "index.html")) {
		t.Error("built into the docs folder")
	}

	out := filepath.Join(docs, "public")
	writeFile(t, filepath.Join(out, "stray.md"), "# Stray\n")
	for range 2 {
		if r := run(t, nil, "build", link, "-o", out); r.code != 0 {
			t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
		}
	}
	if exists(filepath.Join(out, "public")) || exists(filepath.Join(out, "stray", "index.html")) {
		t.Error("output directory was included in the site")
	}
}

func TestBuildRemovesStaleFiles(t *testing.T) {
	dir := docsDir(t)
	out := filepath.Join(t.TempDir(), "site")
	if r := run(t, nil, "build", dir, "-o", out); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	writeFile(t, filepath.Join(out, "notes.txt"), "keep me")
	writeFile(t, filepath.Join(out, "guide", "keep.txt"), "keep me")
	if err := os.Rename(filepath.Join(dir, "guide", "install.md"), filepath.Join(dir, "guide", "setup.md")); err != nil {
		t.Fatal(err)
	}
	if r := run(t, nil, "build", dir, "-o", out); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if exists(filepath.Join(out, "guide", "install")) {
		t.Error("renamed page's old output remains")
	}
	for _, f := range []string{"guide/setup/index.html", "notes.txt", "guide/keep.txt"} {
		if !exists(filepath.Join(out, f)) {
			t.Errorf("missing %s", f)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(out, "_documango", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := json.Unmarshal(manifest, &paths); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if !slices.IsSorted(paths) || !slices.Contains(paths, "_documango/manifest.json") || !slices.Contains(paths, "guide/setup/index.html") {
		t.Errorf("manifest = %q", paths)
	}
	if slices.Contains(paths, "notes.txt") {
		t.Error("manifest lists a user file")
	}
}

func TestBuildIgnoresBadManifest(t *testing.T) {
	dir := docsDir(t)
	tmp := t.TempDir()
	out := filepath.Join(tmp, "site")
	outside := filepath.Join(tmp, "outside")
	writeFile(t, filepath.Join(tmp, "victim.txt"), "keep me")
	writeFile(t, filepath.Join(outside, "victim.txt"), "keep me")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(out, "link")); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(out, "_documango", "manifest.json")

	for _, body := range []string{
		`["../victim.txt", "/etc/passwd", "link/victim.txt", "link", "."]`,
		`not json`,
	} {
		writeFile(t, manifest, body)
		if r := run(t, nil, "build", dir, "-o", out); r.code != 0 {
			t.Fatalf("%s: exit %d, stderr %q", body, r.code, r.stderr)
		}
		for _, f := range []string{filepath.Join(tmp, "victim.txt"), filepath.Join(outside, "victim.txt"), filepath.Join(out, "link")} {
			if !exists(f) {
				t.Errorf("%s: deleted %s", body, f)
			}
		}
	}
}

func TestBuildClean(t *testing.T) {
	dir := docsDir(t)
	tmp := t.TempDir()

	t.Run("refuses user data", func(t *testing.T) {
		out := filepath.Join(tmp, "mine")
		writeFile(t, filepath.Join(out, "notes.txt"), "keep me")
		r := run(t, nil, "build", dir, "-o", out, "--clean")
		if r.code != 1 || !strings.Contains(r.stderr, "refusing to clean") || !strings.Contains(r.stderr, "Hint: Use --output") {
			t.Errorf("exit %d, stderr %q", r.code, r.stderr)
		}
		if !exists(filepath.Join(out, "notes.txt")) {
			t.Error("user file deleted")
		}
	})
	t.Run("refuses a file", func(t *testing.T) {
		out := filepath.Join(tmp, "file")
		writeFile(t, out, "x")
		if r := run(t, nil, "build", dir, "-o", out, "--clean"); r.code != 1 || !strings.Contains(r.stderr, "is not a directory") {
			t.Errorf("exit %d, stderr %q", r.code, r.stderr)
		}
	})
	t.Run("previous build", func(t *testing.T) {
		out := filepath.Join(tmp, "prev")
		if r := run(t, nil, "build", dir, "-o", out); r.code != 0 {
			t.Fatal(r.stderr)
		}
		writeFile(t, filepath.Join(out, "stale", "index.html"), "old")
		if r := run(t, nil, "build", dir, "-o", out, "--clean"); r.code != 0 {
			t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
		}
		if exists(filepath.Join(out, "stale")) || !exists(filepath.Join(out, "index.html")) {
			t.Error("clean did not replace the previous build")
		}
	})
	t.Run("empty or missing", func(t *testing.T) {
		empty := filepath.Join(tmp, "empty")
		if err := os.Mkdir(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, out := range []string{empty, filepath.Join(tmp, "missing")} {
			if r := run(t, nil, "build", dir, "-o", out, "--clean"); r.code != 0 || !exists(filepath.Join(out, "index.html")) {
				t.Errorf("%s: exit %d, stderr %q", out, r.code, r.stderr)
			}
		}
	})
}

func TestBuildThemeErrors(t *testing.T) {
	dir := docsDir(t)
	out := filepath.Join(t.TempDir(), "out")
	r := run(t, nil, "build", dir, "-o", out, "--dark-theme", "tomorow-night")
	if r.code != 1 || !strings.Contains(r.stderr, `dark theme: unknown theme "tomorow-night"; did you mean tomorrow-night`) {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	if !strings.Contains(r.stderr, "documango themes") {
		t.Errorf("stderr %q lacks themes hint", r.stderr)
	}
	r = run(t, nil, "build", dir, "-o", out, "--light-theme", filepath.Join(dir, "missing.yaml"))
	if r.code != 1 || !strings.Contains(r.stderr, "light theme:") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}

	scheme := filepath.Join(t.TempDir(), "mine.yaml")
	writeFile(t, scheme, "system: base16\nname: Mine\nvariant: dark\npalette:\n"+paletteYAML())
	if r := run(t, nil, "build", dir, "-o", out, "--dark-theme", scheme); r.code != 0 {
		t.Errorf("custom theme: exit %d, stderr %q", r.code, r.stderr)
	}
}

func paletteYAML() string {
	var b strings.Builder
	for _, k := range []string{"00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "0A", "0B", "0C", "0D", "0E", "0F"} {
		b.WriteString("  base" + k + ": \"#1d1f21\"\n")
	}
	return b.String()
}

func TestThemes(t *testing.T) {
	r := run(t, nil, "themes")
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "tomorrow-night\tdark\tTomorrow Night\tbuiltin\n") ||
		!strings.Contains(r.stdout, "remote-dusk\tdark\tRemote Dusk\tdownload\n") {
		t.Errorf("plain listing lacks tomorrow-night:\n%.300s", r.stdout)
	}
	if strings.Contains(r.stdout, "\x1b[") {
		t.Error("plain listing contains ANSI escapes")
	}

	r = run(t, nil, "themes", "--variant", "light")
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if r.code != 0 || len(lines) < 10 {
		t.Fatalf("exit %d, %d lines", r.code, len(lines))
	}
	for _, l := range lines {
		if f := strings.Split(l, "\t"); len(f) != 4 || f[1] != "light" {
			t.Errorf("unexpected line %q", l)
		}
	}

	tty := []string{"TTY_FORCE=1", "TERM=xterm-256color"}
	r = run(t, tty, "themes", "--variant", "dark")
	if r.code != 0 || !strings.Contains(r.stdout, "\x1b[") || !strings.Contains(r.stdout, "Tomorrow Night") || strings.Contains(r.stdout, "\t") ||
		!strings.Contains(r.stdout, "built-in") || !strings.Contains(r.stdout, "Remote Dusk") {
		t.Errorf("terminal listing: exit %d:\n%.300s", r.code, r.stdout)
	}
	r = run(t, append(tty, "NO_COLOR=1"), "themes")
	if strings.Contains(r.stdout, "\x1b[") || !strings.Contains(r.stdout, "tomorrow-night") || strings.Contains(r.stdout, "\t") {
		t.Errorf("NO_COLOR terminal listing:\n%.300s", r.stdout)
	}
}

func TestColor(t *testing.T) {
	tty := []string{"TTY_FORCE=1", "TERM=xterm-256color"}
	for _, tc := range []struct {
		name    string
		environ []string
		args    []string
		color   bool
	}{
		{"terminal", tty, nil, true},
		{"not a terminal", []string{"TERM=xterm-256color"}, nil, false},
		{"NO_COLOR", append(tty, "NO_COLOR=1"), nil, false},
		{"NO_COLOR any value", append(tty, "NO_COLOR=yes"), nil, false},
		{"empty NO_COLOR", append(tty, "NO_COLOR="), nil, true},
		{"TERM=dumb", []string{"TTY_FORCE=1", "TERM=dumb"}, nil, false},
		{"--no-color", tty, []string{"--no-color"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, tc.environ, append(tc.args, "serve", filepath.Join(t.TempDir(), "nope"))...)
			if r.code != 1 || !strings.Contains(r.stderr, "Error:") {
				t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
			}
			if got := strings.Contains(r.stderr, "\x1b["); got != tc.color {
				t.Errorf("ANSI escapes = %v, want %v: %q", got, tc.color, r.stderr)
			}
		})
	}
}

func TestLogHandler(t *testing.T) {
	var buf bytes.Buffer
	h := &logHandler{w: &colorprofile.Writer{Forward: &buf, Profile: colorprofile.NoTTY}, level: slog.LevelDebug, mu: &sync.Mutex{}}
	log := slog.New(h).With("site", "docs").WithGroup("build")
	log.Debug("source changed", "paths", []string{"a.md"})
	log.Error("build failed", "err", "boom")
	slog.New(h.WithGroup("")).Info("plain")

	got := buf.String()
	for _, w := range []string{"DEBU source changed site=docs build.paths=[a.md]\n", "ERRO build failed site=docs build.err=boom\n", "INFO plain\n"} {
		if !strings.Contains(got, w) {
			t.Errorf("log output lacks %q:\n%s", w, got)
		}
	}
	if !regexp.MustCompile(`^\d\d:\d\d:\d\d `).MatchString(got) {
		t.Errorf("log line lacks time: %q", got)
	}
	if (&logHandler{level: slog.LevelWarn}).Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Info enabled at Warn level")
	}
}

// startServe runs the CLI with args until the returned stop function is
// called, and returns the address it printed.
func startServe(t *testing.T, args ...string) (url string, stderr *syncBuffer, stop func() int) {
	t.Helper()
	return startServeEnv(t, testEnv(t), args...)
}

func startServeEnv(t *testing.T, environ []string, args ...string) (url string, stderr *syncBuffer, stop func() int) {
	t.Helper()
	stderr = &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- Execute(ctx, args, Env{Stdout: io.Discard, Stderr: stderr, Environ: environ, Version: "1.2.3"})
	}()
	stop = func() int {
		cancel()
		select {
		case code := <-done:
			return code
		case <-time.After(5 * time.Second):
			t.Fatal("serve did not stop after cancel")
			return -1
		}
	}
	re := regexp.MustCompile(`at (http://\S+)\n`)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := re.FindStringSubmatch(stderr.String()); m != nil {
			return m[1], stderr, stop
		}
		select {
		case code := <-done:
			t.Fatalf("serve exited early with %d: %s", code, stderr.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
	stop()
	t.Fatalf("serve never printed its address: %q", stderr.String())
	return "", nil, nil
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestServe(t *testing.T) {
	dir := docsDir(t)
	url, stderr, stop := startServe(t, dir, "--port", "0", "--base-path", "docs", "-v")

	if !strings.Contains(stderr.String(), "Serving Hello Docs at http://127.0.0.1:") || !strings.HasSuffix(url, "/docs/") {
		t.Errorf("banner = %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Press Ctrl+C to stop") {
		t.Errorf("banner lacks stop hint: %q", stderr.String())
	}
	resp, body := get(t, url+"guide/install/")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Install") || !strings.Contains(body, "/docs/_documango/events") {
		t.Errorf("GET page: %d\n%.300s", resp.StatusCode, body)
	}

	// Editing a page triggers a logged rebuild.
	writeFile(t, filepath.Join(dir, "guide", "install.md"), "# Install v2\n")
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(stderr.String(), "rebuilt site") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := stderr.String(); !strings.Contains(s, "rebuilt site") || !strings.Contains(s, "source changed") {
		t.Errorf("no rebuild logs: %q", s)
	}
	if _, body := get(t, url+"guide/install/"); !strings.Contains(body, "Install v2") {
		t.Error("page not rebuilt")
	}

	if code := stop(); code != 0 {
		t.Errorf("exit %d after cancel, stderr %q", code, stderr.String())
	}
}

func TestServeEvents(t *testing.T) {
	url, _, stop := startServe(t, "serve", docsDir(t), "-p", "0")
	defer stop()
	resp, err := http.Get(url + "_documango/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if line, _ := bufio.NewReader(resp.Body).ReadString('\n'); line != "retry: 1000\n" {
		t.Errorf("first line = %q", line)
	}
}

func TestServeQuiet(t *testing.T) {
	url, stderr, stop := startServe(t, "serve", docsDir(t), "--port", "0", "--quiet")
	if resp, _ := get(t, url); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s: %d", url, resp.StatusCode)
	}
	if code := stop(); code != 0 {
		t.Errorf("exit %d", code)
	}
	if s := stderr.String(); !regexp.MustCompile(`^Serving Hello Docs at http://127\.0\.0\.1:\d+/\n$`).MatchString(s) {
		t.Errorf("quiet serve printed %q, want only the address line", s)
	}
}

func TestServeErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	dir := docsDir(t)
	r := run(t, nil, "serve", dir, "--port", strings.TrimPrefix(ln.Addr().String(), "127.0.0.1:"))
	if r.code != 1 || !strings.Contains(r.stderr, "already in use") || !strings.Contains(r.stderr, "use --port to pick another port") {
		t.Errorf("port %d in use: exit %d, stderr %q", port, r.code, r.stderr)
	}
	r = run(t, nil, "serve", dir, "--port", "0", "--host", "256.0.0.1")
	if r.code != 1 || !strings.HasPrefix(r.stderr, "Error: ") {
		t.Errorf("bad host: exit %d, stderr %q", r.code, r.stderr)
	}
	r = run(t, nil, dir, "--port", "0", "--dark-theme", "nope-nope")
	if r.code != 1 || !strings.Contains(r.stderr, `unknown theme "nope-nope"`) {
		t.Errorf("bad theme: exit %d, stderr %q", r.code, r.stderr)
	}
	empty := t.TempDir()
	r = run(t, nil, "serve", empty)
	if r.code != 1 || !strings.Contains(r.stderr, "no Markdown files found") {
		t.Errorf("empty dir: exit %d, stderr %q", r.code, r.stderr)
	}
}

// fakeOpener replaces the browser opener for one test and returns the URLs
// it is asked to open.
func fakeOpener(t *testing.T, err error) <-chan string {
	t.Helper()
	urls := make(chan string, 1)
	orig := openBrowser
	openBrowser = func(url string) error {
		urls <- url
		return err
	}
	t.Cleanup(func() { openBrowser = orig })
	return urls
}

func TestServeOpen(t *testing.T) {
	for _, tc := range []struct {
		name, host, opened string
	}{
		{"serve", "127.0.0.1", "127.0.0.1"},
		{"root", "0.0.0.0", "127.0.0.1"},
		{"root", "::", "localhost"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			if tc.host == "::" {
				ln, err := net.Listen("tcp", "[::]:0")
				if err != nil {
					t.Skip("no IPv6:", err)
				}
				ln.Close()
			}
			urls := fakeOpener(t, nil)
			args := []string{docsDir(t), "-p", "0", "--host", tc.host, "--open", "--base-path", "docs"}
			if tc.name == "serve" {
				args = append([]string{"serve"}, args...)
			}
			url, _, stop := startServe(t, args...)
			defer stop()
			select {
			case got := <-urls:
				port := url[strings.LastIndex(url, ":")+1:]
				if want := "http://" + net.JoinHostPort(tc.opened, strings.TrimSuffix(port, "/docs/")) + "/docs/"; got != want {
					t.Errorf("opened %q, want %q (printed %q)", got, want, url)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("browser not opened")
			}
		})
	}
}

func TestServeOpenFails(t *testing.T) {
	urls := fakeOpener(t, errors.New("boom"))
	url, stderr, stop := startServe(t, "serve", docsDir(t), "-p", "0", "--open", "-q")
	<-urls
	if resp, _ := get(t, url); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s: %d", url, resp.StatusCode)
	}
	if code := stop(); code != 0 {
		t.Errorf("exit %d", code)
	}
	if s := stderr.String(); strings.Count(s, "could not open a browser: boom") != 1 {
		t.Errorf("stderr %q lacks one open warning", s)
	}
}

func TestServeNoOpen(t *testing.T) {
	urls := fakeOpener(t, nil)
	_, _, stop := startServe(t, "serve", docsDir(t), "-p", "0")
	stop()
	select {
	case u := <-urls:
		t.Errorf("opened %q without --open", u)
	default:
	}
}

func TestStartBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the opener")
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if err := startBrowser("http://x/"); err == nil {
		t.Error("startBrowser with no opener on PATH: nil error")
	}

	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	got := filepath.Join(bin, "url")
	writeFile(t, filepath.Join(bin, name), "#!/bin/sh\nprintf %s \"$1\" > "+got+"\n")
	if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := startBrowser("http://x/"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(got); string(b) == "http://x/" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("opener never ran with the URL")
}

func TestThemeLists(t *testing.T) {
	dir := docsDir(t)
	out := filepath.Join(t.TempDir(), "out")
	mine := filepath.Join(t.TempDir(), "mine.yaml")
	writeFile(t, mine, "system: base16\nname: Mine\nvariant: dark\npalette:\n"+paletteYAML())

	r := run(t, nil, "build", dir, "-o", out, "--dark-theme", " tomorrow-night, remote-dusk ,"+mine, "--light-theme", "tomorrow,github")
	if r.code != 0 || strings.Contains(r.stderr, "WARN") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	css, _ := os.ReadFile(filepath.Join(out, "_documango", "style.css"))
	for _, slug := range []string{"remote-dusk", "mine", "github"} {
		if !bytes.Contains(css, []byte(slug)) {
			t.Errorf("style.css lacks scheme %s", slug)
		}
	}

	r = run(t, nil, "build", dir, "-o", out, "--dark-theme", "tomorrow-night,github")
	if r.code != 0 || !strings.Contains(r.stderr, "WARN github is a light theme but is listed in --dark-theme") {
		t.Errorf("variant mismatch: exit %d, stderr %q", r.code, r.stderr)
	}

	for _, tc := range []struct{ flag, value, msg string }{
		{"--dark-theme", "nord,,dracula", `--dark-theme has an empty entry in "nord,,dracula"`},
		{"--light-theme", " ", "--light-theme has an empty entry"},
		{"--dark-theme", "nord, Nord", `--dark-theme lists "Nord" more than once`},
		{"--search", "google", `invalid --search "google": use pagefind or builtin`},
	} {
		r := run(t, nil, "build", dir, "-o", out, tc.flag, tc.value)
		if r.code != 2 || !strings.Contains(r.stderr, tc.msg) || !strings.Contains(r.stderr, "documango build --help") {
			t.Errorf("%s %q: exit %d, stderr %q", tc.flag, tc.value, r.code, r.stderr)
		}
	}

	// Without a usable theme cache, unknown names still get suggestions.
	blocked := filepath.Join(t.TempDir(), "file")
	writeFile(t, blocked, "")
	r = run(t, []string{"DOCUMANGO_CACHE_DIR=" + blocked}, "build", dir, "-o", out, "--light-theme", "tomorow")
	if r.code != 1 || !strings.Contains(r.stderr, `light theme: unknown theme "tomorow"; did you mean tomorrow`) ||
		!strings.Contains(r.stderr, "catalog unavailable") || !strings.Contains(r.stderr, "Hint: Run 'documango themes'") {
		t.Errorf("unknown theme offline: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestThemesCatalog(t *testing.T) {
	r := run(t, nil, "themes", "--offline")
	if r.code != 0 || r.stderr != "" || strings.Contains(r.stdout, "remote-dusk") || !strings.Contains(r.stdout, "\tbuiltin\n") {
		t.Errorf("--offline: exit %d, stderr %q\n%.200s", r.code, r.stderr, r.stdout)
	}
	offline := r.stdout

	blocked := filepath.Join(t.TempDir(), "file")
	writeFile(t, blocked, "")
	r = run(t, []string{"DOCUMANGO_CACHE_DIR=" + blocked}, "themes")
	if r.code != 0 || r.stdout != offline || !strings.Contains(r.stderr, "WARN showing the built-in themes only") {
		t.Errorf("unavailable: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestPickNeedsTerminal(t *testing.T) {
	r := run(t, nil, "themes", "pick", "--variant", "dark")
	if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "Error: themes pick needs an interactive terminal") ||
		!strings.Contains(r.stderr, "Hint: Run 'documango themes'") {
		t.Errorf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	r = run(t, nil, "themes", "pick", "--variant", "blue")
	if r.code != 2 || !strings.Contains(r.stderr, `invalid --variant "blue"`) {
		t.Errorf("bad variant: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestBuildSearch(t *testing.T) {
	dir := docsDir(t)
	writeFile(t, filepath.Join(dir, "old-build", "index.html"), "<h1>Hello Docs</h1>")
	out := filepath.Join(t.TempDir(), "out")
	r := run(t, []string{"DOCUMANGO_PAGEFIND=" + fakePagefindBin(t)}, "build", dir, "-o", out)
	if r.code != 0 || !regexp.MustCompile(`^Built 2 pages to \S+ in \S+\n$`).MatchString(r.stderr) {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	entry, err := os.ReadFile(filepath.Join(out, "pagefind", "pagefind-entry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Pages []string }
	if err := json.Unmarshal(entry, &got); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got.Pages)
	if want := []string{"guide/install/index.html", "index.html"}; !slices.Equal(got.Pages, want) {
		t.Errorf("indexed %v, want %v", got.Pages, want)
	}

	// Switching to the built-in search drops the old bundle.
	r = run(t, []string{"DOCUMANGO_PAGEFIND=" + filepath.Join(t.TempDir(), "missing")}, "build", dir, "-o", out, "--search", "builtin")
	if r.code != 0 || strings.Contains(r.stderr, "WARN") || strings.Contains(r.stderr, "built-in search") || exists(filepath.Join(out, "pagefind")) {
		t.Errorf("builtin: exit %d, stderr %q", r.code, r.stderr)
	}

	notExec := filepath.Join(t.TempDir(), "pagefind")
	writeFile(t, notExec, "")
	for _, bin := range []string{filepath.Join(t.TempDir(), "missing"), notExec} {
		r = run(t, []string{"DOCUMANGO_PAGEFIND=" + bin}, "build", dir, "-o", out, "-q")
		if r.code != 0 || strings.Count(r.stderr, "WARN pagefind search is unavailable; using the built-in search") != 1 {
			t.Errorf("%s: exit %d, stderr %q", bin, r.code, r.stderr)
		}
		r = run(t, []string{"DOCUMANGO_PAGEFIND=" + bin}, "build", dir, "-o", out)
		if !regexp.MustCompile(`Built 2 pages to \S+ with the built-in search in \S+\n$`).MatchString(r.stderr) {
			t.Errorf("%s: summary %q", bin, r.stderr)
		}
	}
}

func TestServeSearch(t *testing.T) {
	dir := docsDir(t)
	url, _, stop := startServe(t, dir, "--port", "0")
	if resp, body := get(t, url+"pagefind/pagefind.js"); resp.StatusCode != http.StatusOK || body != "export {}" {
		t.Errorf("GET pagefind.js: %d %q", resp.StatusCode, body)
	}
	stop()

	environ := append(testEnv(t), "DOCUMANGO_PAGEFIND="+filepath.Join(t.TempDir(), "missing"))
	url, stderr, stop := startServeEnv(t, environ, dir, "--port", "0")
	defer stop()
	writeFile(t, filepath.Join(dir, "index.md"), "# Hello again\n")
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(stderr.String(), "rebuilt site") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := stderr.String(); !strings.Contains(s, "rebuilt site") || strings.Count(s, "pagefind search is unavailable") != 1 {
		t.Errorf("want one pagefind warning across rebuilds: %q", s)
	}
	if resp, _ := get(t, url+"pagefind/pagefind.js"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET pagefind.js after fallback: %d", resp.StatusCode)
	}
}

// configDocs returns docsDir plus a favicon, a logo and the config file name
// with body.
func configDocs(t *testing.T, name, body string) string {
	t.Helper()
	dir := docsDir(t)
	writeFile(t, filepath.Join(dir, "favicon.svg"), "<svg/>")
	writeFile(t, filepath.Join(dir, "img", "logo.png"), "png")
	writeFile(t, filepath.Join(dir, name), body)
	return dir
}

const configTOML = `title = "Acme Docs"
description = "Docs for Acme."
url = "https://acme.dev/docs/"
author = "Acme Inc."
language = "de"
favicon = "favicon.svg"
logo = "img/logo.png"

[theme]
dark = ["remote-dusk", "github-dark"]
light = "github"
search = "builtin"

[[links]]
title = "GitHub"
url = "https://github.com/acme/acme"
`

func TestBuildConfig(t *testing.T) {
	dir := configDocs(t, "documango.toml", configTOML)
	out := filepath.Join(t.TempDir(), "site")
	if r := run(t, nil, "build", dir, "-o", out); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, w := range []string{
		"<title>Acme Docs</title>", `<html lang="de"`, `<meta name="description" content="Docs for Acme.">`,
		`<meta name="author" content="Acme Inc.">`, `<link rel="canonical" href="https://acme.dev/docs/">`,
		`<link rel="icon" href="/docs/favicon.svg"`, `src="/docs/img/logo.png"`, "/docs/_documango/style.css",
		`href="https://github.com/acme/acme" rel="noopener">GitHub</a>`, `<option value="github-dark">`,
	} {
		if !bytes.Contains(home, []byte(w)) {
			t.Errorf("index.html lacks %q", w)
		}
	}
	css, _ := os.ReadFile(filepath.Join(out, "_documango", "style.css"))
	if github, _ := theme.Lookup("github"); !bytes.Contains(css, []byte(github.CSSVars())) {
		t.Error("light theme from the config not applied")
	}
	if exists(filepath.Join(out, "documango.toml")) || exists(filepath.Join(out, "pagefind")) {
		t.Error("config file copied or pagefind used")
	}
}

func TestBuildFlagsOverrideConfig(t *testing.T) {
	dir := configDocs(t, "documango.yaml", "title: Acme Docs\nbase_path: /docs/\ntheme:\n  search: builtin\n  light: [github]\n")
	out := filepath.Join(t.TempDir(), "site")
	r := run(t, nil, "build", dir, "-o", out, "--title", "Flag Title", "--base-path", "/", "--search", "pagefind", "--light-theme", "tomorrow")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, w := range []string{"<title>Flag Title</title>", `href="/_documango/style.css"`, `<html lang="en"`} {
		if !bytes.Contains(home, []byte(w)) {
			t.Errorf("index.html lacks %q", w)
		}
	}
	css, _ := os.ReadFile(filepath.Join(out, "_documango", "style.css"))
	if tomorrow, _ := theme.Lookup("tomorrow"); !bytes.Contains(css, []byte(tomorrow.CSSVars())) {
		t.Error("--light-theme did not override the config")
	}
	if !exists(filepath.Join(out, "pagefind", "pagefind.js")) || exists(filepath.Join(out, "documango.yaml")) {
		t.Error("--search pagefind ignored or config copied")
	}
}

func TestBuildExplicitConfig(t *testing.T) {
	dir := configDocs(t, "documango.toml", `title = "Implicit"`)
	writeFile(t, filepath.Join(dir, "conf", "site.yml"), "title: Inside\n")
	outside := filepath.Join(t.TempDir(), "site.toml")
	writeFile(t, outside, `title = "Outside"`)
	out := filepath.Join(t.TempDir(), "site")

	for _, tc := range []struct{ config, title string }{
		{filepath.Join(dir, "conf", "site.yml"), "Inside"},
		{outside, "Outside"},
	} {
		if r := run(t, nil, "build", dir, "-o", out, "--config", tc.config); r.code != 0 {
			t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
		}
		home, _ := os.ReadFile(filepath.Join(out, "index.html"))
		if !bytes.Contains(home, []byte("<title>"+tc.title+"</title>")) {
			t.Errorf("--config %s: title not applied", tc.config)
		}
		if exists(filepath.Join(out, "conf", "site.yml")) == (tc.title == "Inside") || exists(filepath.Join(out, "documango.toml")) {
			t.Errorf("--config %s: config file copied into the site", tc.config)
		}
	}
}

func TestConfigErrors(t *testing.T) {
	dir := docsDir(t)
	r := run(t, nil, "build", dir, "--config", filepath.Join(dir, "missing.toml"), "-o", filepath.Join(t.TempDir(), "o"))
	if r.code != 1 || !strings.Contains(r.stderr, "missing.toml does not exist") {
		t.Errorf("missing --config: exit %d, stderr %q", r.code, r.stderr)
	}

	writeFile(t, filepath.Join(dir, "documango.toml"), `colour = "red"`)
	r = run(t, nil, "build", dir, "-o", filepath.Join(t.TempDir(), "o"))
	if r.code != 1 || !strings.Contains(r.stderr, `documango.toml: unknown key "colour"`) {
		t.Errorf("unknown key: exit %d, stderr %q", r.code, r.stderr)
	}

	writeFile(t, filepath.Join(dir, "documango.toml"), "[theme]\ndark = \"nope-nope\"")
	r = run(t, nil, dir, "--port", "0")
	if r.code != 1 || !strings.Contains(r.stderr, `unknown theme "nope-nope"`) {
		t.Errorf("bad theme in config: exit %d, stderr %q", r.code, r.stderr)
	}

	writeFile(t, filepath.Join(dir, "documango.yaml"), "")
	r = run(t, nil, "serve", dir, "--port", "0")
	if r.code != 1 || !strings.Contains(r.stderr, "keep only one") {
		t.Errorf("two config files: exit %d, stderr %q", r.code, r.stderr)
	}
}

// waitFor polls url until its body contains want.
func waitFor(t *testing.T, url, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, body := get(t, url); strings.Contains(body, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never contained %q", url, want)
}

func TestServeReloadsConfig(t *testing.T) {
	dir := configDocs(t, "documango.toml", "title = \"First\"\nbase_path = \"/docs/\"")
	url, stderr, stop := startServe(t, dir, "--port", "0")
	defer stop()
	if !strings.HasSuffix(url, "/docs/") {
		t.Fatalf("base path from config not used: %s", url)
	}
	if resp, _ := get(t, url+"documango.toml"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("config file served: %d", resp.StatusCode)
	}

	writeFile(t, filepath.Join(dir, "documango.toml"), "title = \"Second\"\nbase_path = \"/docs/\"\n[[links]]\ntitle = \"Blog\"\nurl = \"/blog/\"")
	waitFor(t, url, ">Blog</a>")

	// A broken config fails the rebuild but keeps the last good site.
	writeFile(t, filepath.Join(dir, "documango.toml"), `colour = "red"`)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(stderr.String(), "build failed") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := stderr.String(); !strings.Contains(s, `unknown key \"colour\"`) {
		t.Errorf("broken config not reported: %q", s)
	}
	if _, body := get(t, url); !strings.Contains(body, "<title>Second</title>") {
		t.Error("last good build not kept")
	}

	// The base path is fixed while serving.
	writeFile(t, filepath.Join(dir, "documango.toml"), "title = \"Third\"\nbase_path = \"/other/\"")
	waitFor(t, url, "<title>Third</title>")
	if !strings.Contains(stderr.String(), "restart") {
		t.Errorf("base path change not reported: %q", stderr.String())
	}
}
