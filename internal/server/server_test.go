package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func staticBuild(files Files) BuildFunc {
	return func() (Files, error) { return files, nil }
}

// switchBuild returns a build whose result the test can change, and counts calls.
type switchBuild struct {
	mu    sync.Mutex
	files Files
	err   error
	calls atomic.Int64
}

func (b *switchBuild) build() (Files, error) {
	b.calls.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.files, b.err
}

func (b *switchBuild) set(files Files, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files, b.err = files, err
}

func newTestServer(t *testing.T, build BuildFunc, opts Options) *Server {
	t.Helper()
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	s, err := New(build, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNew(t *testing.T) {
	t.Run("initial build error", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := New(func() (Files, error) { return nil, boom }, Options{Dir: t.TempDir()})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want wrapping %v", err, boom)
		}
	})
	t.Run("nil build", func(t *testing.T) {
		if _, err := New(nil, Options{Dir: t.TempDir()}); err == nil {
			t.Fatal("want error for nil build")
		}
	})
	t.Run("base path normalization", func(t *testing.T) {
		for in, want := range map[string]string{
			"":          "/",
			"/":         "/",
			"docs":      "/docs/",
			"/docs":     "/docs/",
			"/docs/":    "/docs/",
			"//a//b/":   "/a/b/",
			"/a/../b/":  "/b/",
			"/docs/v1/": "/docs/v1/",
		} {
			s := newTestServer(t, staticBuild(Files{}), Options{BasePath: in})
			if s.base != want {
				t.Errorf("BasePath %q normalized to %q, want %q", in, s.base, want)
			}
		}
	})
	t.Run("events url", func(t *testing.T) {
		s := newTestServer(t, staticBuild(Files{}), Options{BasePath: "/docs"})
		if got, want := s.EventsURL(), "/docs/_documango/events"; got != want {
			t.Errorf("EventsURL = %q, want %q", got, want)
		}
		s = newTestServer(t, staticBuild(Files{}), Options{})
		if got := s.EventsURL(); got != EventsPath {
			t.Errorf("EventsURL = %q, want %q", got, EventsPath)
		}
	})
	t.Run("events url for base path", func(t *testing.T) {
		for in, want := range map[string]string{
			"":       EventsPath,
			"/":      EventsPath,
			"docs":   "/docs" + EventsPath,
			"/docs/": "/docs" + EventsPath,
		} {
			if got := EventsURLFor(in); got != want {
				t.Errorf("EventsURLFor(%q) = %q, want %q", in, got, want)
			}
			s := newTestServer(t, staticBuild(Files{}), Options{BasePath: in})
			if got := s.EventsURL(); got != want {
				t.Errorf("EventsURL with base %q = %q, want %q", in, got, want)
			}
		}
	})
}

func TestServeHTTP(t *testing.T) {
	files := Files{
		"index.html":               []byte("<p>home</p>"),
		"guide/index.html":         []byte("<p>guide</p>"),
		"guide/install/index.html": []byte("<p>install</p>"),
		"_documango/style.css":     []byte("body{}"),
		"img/logo.png":             []byte("\x89PNG\r\n\x1a\n...."),
		"data.unknownext":          []byte("plain words"),
	}
	withNotFound := Files{"index.html": []byte("home"), "404.html": []byte("<h1>missing</h1>")}

	tests := []struct {
		name       string
		files      Files
		method     string
		target     string
		wantStatus int
		wantBody   string
		wantType   string
		wantLoc    string
	}{
		{name: "root index", target: "/docs/", wantStatus: 200, wantBody: "<p>home</p>", wantType: "text/html; charset=utf-8"},
		{name: "subdir index", target: "/docs/guide/", wantStatus: 200, wantBody: "<p>guide</p>"},
		{name: "nested index", target: "/docs/guide/install/", wantStatus: 200, wantBody: "<p>install</p>"},
		{name: "explicit index", target: "/docs/guide/index.html", wantStatus: 200, wantBody: "<p>guide</p>"},
		{name: "css", target: "/docs/_documango/style.css", wantStatus: 200, wantBody: "body{}", wantType: "text/css; charset=utf-8"},
		{name: "png", target: "/docs/img/logo.png", wantStatus: 200, wantType: "image/png"},
		{name: "sniffed type", target: "/docs/data.unknownext", wantStatus: 200, wantType: "text/plain; charset=utf-8"},
		{name: "dir redirect", target: "/docs/guide", wantStatus: 301, wantLoc: "/docs/guide/"},
		{name: "dir redirect keeps query", target: "/docs/guide/install?x=1&y=2", wantStatus: 301, wantLoc: "/docs/guide/install/?x=1&y=2"},
		{name: "base redirect", target: "/docs?q=1", wantStatus: 301, wantLoc: "/docs/?q=1"},
		{name: "dot dot cleaned", target: "/docs/img/../guide/", wantStatus: 200, wantBody: "<p>guide</p>"},
		{name: "double slash cleaned", target: "/docs//img//logo.png", wantStatus: 200, wantType: "image/png"},
		{name: "traversal stays in base", target: "/docs/../../index.html", wantStatus: 404},
		{name: "outside base", target: "/other/index.html", wantStatus: 404, wantBody: "404 page not found\n"},
		{name: "missing plain", target: "/docs/nope.html", wantStatus: 404, wantBody: "404 page not found\n"},
		{name: "missing dir without index", target: "/docs/img/", wantStatus: 404},
		{name: "missing with 404 page", files: withNotFound, target: "/docs/nope", wantStatus: 404, wantBody: "<h1>missing</h1>", wantType: "text/html; charset=utf-8"},
		{name: "head", method: http.MethodHead, target: "/docs/guide/", wantStatus: 200, wantBody: ""},
		{name: "post not allowed", method: http.MethodPost, target: "/docs/", wantStatus: 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := files
			if tt.files != nil {
				fs = tt.files
			}
			s := newTestServer(t, staticBuild(fs), Options{BasePath: "/docs/"})
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(method, tt.target, nil))
			res := rec.Result()
			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if got := res.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if tt.wantBody != "" || method == http.MethodHead {
				if got := rec.Body.String(); got != tt.wantBody {
					t.Errorf("body = %q, want %q", got, tt.wantBody)
				}
			}
			if tt.wantType != "" {
				if got := res.Header.Get("Content-Type"); got != tt.wantType {
					t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
				}
			}
			if tt.wantLoc != "" {
				if got := res.Header.Get("Location"); got != tt.wantLoc {
					t.Errorf("Location = %q, want %q", got, tt.wantLoc)
				}
			}
			if tt.wantStatus == 405 {
				if got := res.Header.Get("Allow"); got != "GET, HEAD" {
					t.Errorf("Allow = %q", got)
				}
			}
		})
	}
}

func TestServeHTTPHeadContentLength(t *testing.T) {
	s := newTestServer(t, staticBuild(Files{"index.html": []byte("12345")}), Options{})
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/", nil))
	if got := rec.Header().Get("Content-Length"); got != "5" {
		t.Errorf("Content-Length = %q, want 5", got)
	}
}

func TestServeHTTPRange(t *testing.T) {
	s := newTestServer(t, staticBuild(Files{"a.txt": []byte("0123456789")}), Options{})
	req := httptest.NewRequest(http.MethodGet, "/a.txt", nil)
	req.Header.Set("Range", "bytes=2-4")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "234" {
		t.Errorf("got %d %q, want 206 \"234\"", rec.Code, rec.Body.String())
	}
}

func TestRebuild(t *testing.T) {
	b := &switchBuild{files: Files{"index.html": []byte("v1")}}
	s := newTestServer(t, b.build, Options{})
	ch := s.subscribe()
	defer s.unsubscribe(ch)

	get := func() string {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		return rec.Body.String()
	}

	b.set(Files{"index.html": []byte("v2")}, nil)
	if err := s.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if got := get(); got != "v2" {
		t.Errorf("after rebuild body = %q, want v2", got)
	}
	if ev := <-ch; ev.name != "reload" {
		t.Errorf("event = %+v, want reload", ev)
	}

	b.set(nil, errors.New("bad front matter\nat line 3"))
	if err := s.Rebuild(); err == nil {
		t.Fatal("Rebuild: want error")
	}
	if got := get(); got != "v2" {
		t.Errorf("after failed rebuild body = %q, want last good v2", got)
	}
	ev := <-ch
	if ev.name != "error" || strings.Contains(ev.data, "\n") {
		t.Fatalf("event = %+v, want single-line error", ev)
	}
	var msg string
	if err := json.Unmarshal([]byte(ev.data), &msg); err != nil || msg != "bad front matter\nat line 3" {
		t.Errorf("error data decodes to %q (%v)", msg, err)
	}

	b.set(Files{"index.html": []byte("v3")}, nil)
	if err := s.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if ev := <-ch; ev.name != "reload" {
		t.Errorf("event after recovery = %+v, want reload", ev)
	}
}

func TestSlowClientDropped(t *testing.T) {
	s := newTestServer(t, staticBuild(Files{}), Options{})
	slow := s.subscribe()
	for range cap(slow) + 1 {
		s.broadcast(event{name: "reload"})
	}
	// Drain what was buffered; the channel must then be closed.
	for range slow {
	}
	s.unsubscribe(slow) // must not panic on an already-dropped client
}

// readEvents streams SSE lines from resp into a channel until the body closes.
func readEvents(resp *http.Response) <-chan string {
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines
}

func waitLine(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("stream closed before %q", want)
			}
			if line == want {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %q", want)
		}
	}
}

func openEvents(t *testing.T, ctx context.Context, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("events response %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	return resp
}

func TestEventsStream(t *testing.T) {
	b := &switchBuild{files: Files{"index.html": []byte("v1")}}
	s := newTestServer(t, b.build, Options{BasePath: "/docs/"})
	s.keepalive = 10 * time.Millisecond
	ts := httptest.NewServer(s)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lines := readEvents(openEvents(t, ctx, ts.URL+s.EventsURL()))
	waitLine(t, lines, "retry: 1000")
	waitLine(t, lines, ": keepalive")

	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	waitLine(t, lines, "event: reload")

	b.set(nil, errors.New("line one\nline two"))
	_ = s.Rebuild()
	waitLine(t, lines, "event: error")
	waitLine(t, lines, `data: "line one\nline two"`)
}

func TestEventsHandlerExitsOnRequestCancel(t *testing.T) {
	s := newTestServer(t, staticBuild(Files{}), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, EventsPath, nil))
	}()
	waitClients(t, s, 1)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("events handler did not exit after request cancel")
	}
	waitClients(t, s, 0)
}

func TestEventsNoFlusher(t *testing.T) {
	s := newTestServer(t, staticBuild(Files{}), Options{})
	w := struct{ http.ResponseWriter }{httptest.NewRecorder()}
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, EventsPath, nil))
	if code := w.ResponseWriter.(*httptest.ResponseRecorder).Code; code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}

func waitClients(t *testing.T, s *Server, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.clientsMu.Lock()
		got := len(s.clients)
		s.clientsMu.Unlock()
		if got == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("clients never reached %d", n)
}

func TestIsIgnored(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(t, staticBuild(Files{}), Options{Dir: dir, Ignore: []string{"_site", filepath.Join(dir, "abs-out")}})
	for rel, want := range map[string]bool{
		"docs/a.md":        false,
		"a.md":             false,
		".git":             true,
		".git/HEAD":        true,
		"sub/.hidden/x.md": true,
		".a.md.swp":        true,
		"_site":            true,
		"_site/index.html": true,
		"_sitemap.md":      false,
		"abs-out/x":        true,
	} {
		if got := s.isIgnored(filepath.Join(dir, rel)); got != want {
			t.Errorf("isIgnored(%q) = %v, want %v", rel, got, want)
		}
	}
	if !s.isIgnored(filepath.Dir(dir)) {
		t.Error("paths outside Dir must be ignored")
	}
	if s.isIgnored(dir) {
		t.Error("Dir itself must not be ignored")
	}
}

// startWatch runs Watch in the background and waits until watches are in place.
// The returned stop func cancels it and asserts Watch returned nil.
func startWatch(t *testing.T, s *Server) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- s.Watch(ctx) }()
	select {
	case <-s.ready:
	case err := <-errc:
		cancel()
		t.Fatalf("Watch: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("watcher never became ready")
	}
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-errc:
				if err != nil {
					t.Errorf("Watch returned %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("Watch did not return after cancel")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func waitCalls(t *testing.T, b *switchBuild, n int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b.calls.Load() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("build calls = %d, want >= %d", b.calls.Load(), n)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatchRebuildsAndDebounces(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "guide"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &switchBuild{files: Files{}}
	s := newTestServer(t, b.build, Options{Dir: dir, Debounce: 100 * time.Millisecond})
	ch := s.subscribe()
	defer s.unsubscribe(ch)
	startWatch(t, s)

	// A burst across the tree collapses into one rebuild.
	for i := range 5 {
		writeFile(t, filepath.Join(dir, "guide", "p"+string(rune('a'+i))+".md"), "x")
		writeFile(t, filepath.Join(dir, "index.md"), "x")
	}
	select {
	case ev := <-ch:
		if ev.name != "reload" {
			t.Fatalf("event = %+v, want reload", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reload after changes")
	}
	// Wait past another debounce window to confirm no second rebuild followed.
	time.Sleep(300 * time.Millisecond)
	if got := b.calls.Load(); got != 2 {
		t.Errorf("build calls = %d, want 2 (initial + one debounced)", got)
	}
}

func TestWatchNewDirectory(t *testing.T) {
	dir := t.TempDir()
	b := &switchBuild{files: Files{}}
	s := newTestServer(t, b.build, Options{Dir: dir, Debounce: 20 * time.Millisecond})
	startWatch(t, s)

	sub := filepath.Join(dir, "new", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	waitCalls(t, b, 2)
	// Give the watcher time to register the new directories, then edit inside.
	deadline := time.Now().Add(5 * time.Second)
	before := b.calls.Load()
	for b.calls.Load() == before {
		if time.Now().After(deadline) {
			t.Fatal("changes inside a newly created directory were not seen")
		}
		writeFile(t, filepath.Join(sub, "page.md"), time.Now().String())
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWatchIgnoresPaths(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"_site", ".git"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b := &switchBuild{files: Files{}}
	s := newTestServer(t, b.build, Options{Dir: dir, Ignore: []string{"_site"}, Debounce: 20 * time.Millisecond})
	startWatch(t, s)

	writeFile(t, filepath.Join(dir, "_site", "index.html"), "x")
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "x")
	writeFile(t, filepath.Join(dir, ".page.md.swp"), "x")
	time.Sleep(200 * time.Millisecond)
	if got := b.calls.Load(); got != 1 {
		t.Errorf("build calls = %d after ignored writes, want 1", got)
	}
}

func TestWatchKeepsServingOnError(t *testing.T) {
	dir := t.TempDir()
	b := &switchBuild{files: Files{"index.html": []byte("good")}}
	s := newTestServer(t, b.build, Options{Dir: dir, Debounce: 20 * time.Millisecond})
	ch := s.subscribe()
	defer s.unsubscribe(ch)
	startWatch(t, s)

	b.set(nil, errors.New("broken"))
	writeFile(t, filepath.Join(dir, "a.md"), "x")
	select {
	case ev := <-ch:
		if ev.name != "error" || ev.data != `"broken"` {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no error event")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Body.String() != "good" {
		t.Errorf("body = %q, want last good build", rec.Body.String())
	}
}

func TestWatchStopsEventHandlers(t *testing.T) {
	s := newTestServer(t, staticBuild(Files{}), Options{})
	ts := httptest.NewServer(s)
	defer ts.Close()
	stop := startWatch(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp := openEvents(t, ctx, ts.URL+s.EventsURL())
	stop()
	// The handler returns, so the body reaches EOF without the client cancelling.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatalf("reading events body: %v", err)
	}
}

func TestWatchMissingDir(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(t, staticBuild(Files{}), Options{Dir: dir})
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Watch(ctx); err == nil {
		t.Fatal("Watch on missing dir: want error")
	}
}

func TestServeShutsDownWithOpenStream(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, staticBuild(Files{"index.html": []byte("hi")}), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, ln, s) }()

	resp, err := http.Get("http://" + ln.Addr().String() + s.EventsURL())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	waitLine(t, readEvents(resp), "retry: 1000")

	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("Serve = %v, want nil on shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

func TestServe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, staticBuild(Files{"index.html": []byte("hi")}), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, ln, s) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hi" {
		t.Errorf("body = %q, want %q", body, "hi")
	}

	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("Serve = %v, want nil on shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
