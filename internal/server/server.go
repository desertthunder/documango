// Package server serves an in-memory built site for local development, rebuilds
// it when source files change, and notifies browsers over Server-Sent Events.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Files is a built site: slash-separated output paths ("index.html",
// "guide/install/index.html", "_documango/style.css") mapped to contents.
type Files map[string][]byte

// BuildFunc builds the whole site. It is called once at startup and after
// every batch of source changes.
type BuildFunc func() (Files, error)

// EventsPath is the Server-Sent Events endpoint, relative to the base path,
// that browsers subscribe to for live reload. See [Server.EventsURL].
const EventsPath = "/_documango/events"

// Options configures a [Server].
type Options struct {
	// Dir is the source directory watched recursively.
	Dir string
	// Ignore lists absolute or Dir-relative paths whose changes are ignored,
	// such as an output directory. Hidden files and directories are always ignored.
	Ignore []string
	// BasePath is the URL prefix the site is served under, e.g. "/" or "/docs/".
	BasePath string
	// Debounce is how long to wait for a burst of changes to settle before
	// rebuilding. Defaults to 100ms.
	Debounce time.Duration
	// Logger receives rebuild and watcher logs. Defaults to discarding them.
	Logger *slog.Logger
}

// Server is an http.Handler for a live-reloading development site.
type Server struct {
	build    BuildFunc
	dir      string
	ignore   []string
	base     string
	debounce time.Duration
	log      *slog.Logger

	// keepalive is the SSE comment interval; a field so tests need not wait.
	keepalive time.Duration

	buildMu sync.Mutex // serializes rebuilds
	mu      sync.RWMutex
	files   Files

	clientsMu sync.Mutex
	clients   map[chan event]struct{}

	ready    chan struct{} // closed once Watch has its watches in place
	done     chan struct{} // closed when Watch returns
	doneOnce sync.Once
}

type event struct{ name, data string }

// New returns a Server and performs the initial build, returning its error.
func New(build BuildFunc, opts Options) (*Server, error) {
	if build == nil {
		return nil, errors.New("server: nil build function")
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("server: resolve dir: %w", err)
	}
	s := &Server{
		build:     build,
		dir:       dir,
		base:      normalizeBase(opts.BasePath),
		debounce:  opts.Debounce,
		log:       opts.Logger,
		keepalive: 15 * time.Second,
		clients:   make(map[chan event]struct{}),
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
	}
	if s.debounce <= 0 {
		s.debounce = 100 * time.Millisecond
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	for _, p := range opts.Ignore {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		s.ignore = append(s.ignore, filepath.Clean(p))
	}
	if s.files, err = build(); err != nil {
		return nil, fmt.Errorf("server: initial build: %w", err)
	}
	return s, nil
}

// normalizeBase returns basePath with exactly one leading and trailing slash.
func normalizeBase(basePath string) string {
	if b := path.Clean("/" + basePath); b != "/" {
		return b + "/"
	}
	return "/"
}

// EventsURLFor returns the URL path of the live-reload event stream for a
// site served under basePath. It lets callers render pages before a Server
// exists; it always equals [Server.EventsURL] for the same base path.
func EventsURLFor(basePath string) string {
	return strings.TrimSuffix(normalizeBase(basePath), "/") + EventsPath
}

// EventsURL returns the URL path of the live-reload event stream.
func (s *Server) EventsURL() string {
	return EventsURLFor(s.base)
}

// Rebuild builds the site and, on success, swaps in the new files and
// broadcasts a "reload" event. On failure it keeps serving the last good
// build, logs the error, broadcasts an "error" event, and returns the error.
func (s *Server) Rebuild() error {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()

	start := time.Now()
	files, err := s.build()
	if err != nil {
		s.log.Error("build failed", "err", err)
		// JSON-encoding keeps multi-line messages on one SSE data line.
		msg, _ := json.Marshal(err.Error())
		s.broadcast(event{name: "error", data: string(msg)})
		return fmt.Errorf("server: rebuild: %w", err)
	}
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
	s.log.Info("rebuilt site", "duration", time.Since(start), "files", len(files))
	s.broadcast(event{name: "reload"})
	return nil
}

// ServeHTTP serves the current build under the base path, plus the
// live-reload event stream at [Server.EventsURL]. Each request is logged at
// debug level, except event streams, which log their connect and disconnect.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w}
	s.serve(sw, r)
	if sw.streamed {
		return
	}
	s.log.Debug("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
		"duration", time.Since(start), "bytes", sw.bytes)
}

func (s *Server) serve(w *statusWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	p := path.Clean("/" + r.URL.Path)
	if p == s.EventsURL() {
		s.serveEvents(w, r)
		return
	}
	if p != "/" && strings.HasSuffix(r.URL.Path, "/") {
		p += "/"
	}
	if p+"/" == s.base {
		redirect(w, r, s.base)
		return
	}
	name, ok := strings.CutPrefix(p, s.base)
	if !ok {
		s.notFound(w, r)
		return
	}

	s.mu.RLock()
	files := s.files
	s.mu.RUnlock()

	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	} else if _, isFile := files[name]; !isFile {
		if _, isDir := files[name+"/index.html"]; isDir {
			redirect(w, r, s.base+name+"/")
			return
		}
	}
	data, ok := files[name]
	if !ok {
		s.notFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func redirect(w http.ResponseWriter, r *http.Request, loc string) {
	if r.URL.RawQuery != "" {
		loc += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, loc, http.StatusMovedPermanently)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	page, ok := s.files["404.html"]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write(page)
	}
}

func (s *Server) serveEvents(w *statusWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Connection", "keep-alive")
	// Flushing sends the 200 header, or fails without writing anything.
	if err := w.FlushError(); err != nil {
		h.Del("Connection")
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodHead {
		return
	}

	w.streamed = true
	ch := s.subscribe()
	s.log.Debug("events client connected", "remote", r.RemoteAddr)
	defer func() {
		s.unsubscribe(ch)
		s.log.Debug("events client disconnected", "remote", r.RemoteAddr)
	}()

	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	if err := w.FlushError(); err != nil {
		return
	}

	ticker := time.NewTicker(s.keepalive)
	defer ticker.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			_, err = fmt.Fprint(w, ": keepalive\n\n")
		case ev, open := <-ch:
			if !open { // dropped as a slow client
				return
			}
			_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
		}
		if err == nil {
			err = w.FlushError()
		}
		if err != nil {
			return
		}
	}
}

// statusWriter records the status and body size of a response. It forwards
// flushes and supports [http.ResponseController] through Unwrap.
type statusWriter struct {
	http.ResponseWriter
	status   int
	bytes    int64
	streamed bool // an event stream, which logs its own lifetime
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// FlushError flushes the wrapped writer, returning [http.ErrNotSupported]
// if it cannot flush.
func (w *statusWriter) FlushError() error {
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
		return err
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return nil
}

func (w *statusWriter) Flush() { _ = w.FlushError() }

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) subscribe() chan event {
	ch := make(chan event, 8)
	s.clientsMu.Lock()
	s.clients[ch] = struct{}{}
	s.clientsMu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan event) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	if _, ok := s.clients[ch]; ok {
		delete(s.clients, ch)
		close(ch)
	}
}

// broadcast sends ev to every client without blocking; a client whose buffer
// is full is disconnected.
func (s *Server) broadcast(ev event) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- ev:
		default:
			delete(s.clients, ch)
			close(ch)
		}
	}
}

// Serve serves h on ln until ctx is done, then shuts down gracefully and
// returns nil. Serve closes ln.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// Request contexts derive from ctx so long-lived event streams end on shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return fmt.Errorf("server: serve: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: serve: %w", err)
	}
	return nil
}
