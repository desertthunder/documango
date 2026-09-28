package cli

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
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

type result struct {
	code           int
	stdout, stderr string
}

// run executes the CLI in-process with the given environment.
func run(t *testing.T, environ []string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, Env{
		Stdout:  &stdout,
		Stderr:  &stderr,
		Environ: environ,
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
		{[]string{"help", "build"}, []string{"Examples:", "--out", "--clean", "--base-path", "--title"}},
		{[]string{"serve", "-h"}, []string{"Examples:", "--port", "--host", "--light-theme"}},
		{[]string{"themes", "--help"}, []string{"Examples:", "--variant"}},
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
	r := run(t, nil, "build", dir, "--out", out)
	if r.code != 0 || !strings.HasPrefix(r.stderr, "Built 1 page to ") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	home, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !bytes.Contains(home, []byte("Documentation")) {
		t.Error("default title missing")
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
		if r.code != 1 || !strings.Contains(r.stderr, "cannot build into") {
			t.Errorf("-o %s: exit %d, stderr %q", out, r.code, r.stderr)
		}
	}
	if !exists(filepath.Join(dir, "index.md")) {
		t.Fatal("docs were deleted")
	}
}

func TestBuildClean(t *testing.T) {
	dir := docsDir(t)
	tmp := t.TempDir()

	t.Run("refuses user data", func(t *testing.T) {
		out := filepath.Join(tmp, "mine")
		writeFile(t, filepath.Join(out, "notes.txt"), "keep me")
		r := run(t, nil, "build", dir, "-o", out, "--clean")
		if r.code != 1 || !strings.Contains(r.stderr, "refusing to clean") || !strings.Contains(r.stderr, "Hint: ") {
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
	if !strings.Contains(r.stdout, "tomorrow-night\tdark\tTomorrow Night\n") {
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
		if f := strings.Split(l, "\t"); len(f) != 3 || f[1] != "light" {
			t.Errorf("unexpected line %q", l)
		}
	}

	tty := []string{"TTY_FORCE=1", "TERM=xterm-256color"}
	r = run(t, tty, "themes", "--variant", "dark")
	if r.code != 0 || !strings.Contains(r.stdout, "\x1b[") || !strings.Contains(r.stdout, "Tomorrow Night") || strings.Contains(r.stdout, "\t") {
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
	stderr = &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- Execute(ctx, args, Env{Stdout: io.Discard, Stderr: stderr, Version: "1.2.3"})
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	var stderr syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- Execute(ctx, []string{"serve", docsDir(t), "--port", strings.TrimPrefix(addr, "127.0.0.1:"), "--quiet"},
			Env{Stdout: io.Discard, Stderr: &stderr})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never came up: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("exit %d", code)
	}
	if s := stderr.String(); s != "" {
		t.Errorf("quiet serve printed %q", s)
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
