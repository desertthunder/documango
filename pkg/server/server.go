// package server contains the implementation for
// an http server that watches for changes in the
// provided directory.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/desertthunder/documango/pkg/build"
	"github.com/desertthunder/documango/pkg/libs/logs"
	"github.com/desertthunder/documango/pkg/view"
	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v3"
)

var logger = logs.CreateConsoleLogger("Server 🌎")

func GenerateLogID() (string, error) {
	var id [8]byte
	_, err := rand.Read(id[:])

	if err != nil {
		logger.Errorf("error generating random ID: %v", err)
		return "", err
	}

	encoded := hex.EncodeToString(id[:])

	return encoded, nil
}

type loggingMiddleware struct {
	h http.Handler
}

type locks struct {
	documentLoader *sync.RWMutex
	serverStarter  *sync.RWMutex
}

type state struct {
	ctx       context.Context
	canceller context.CancelFunc
}

func (l loggingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, _ := GenerateLogID()
	method := r.Method
	path := r.URL.String()

	logger.Infof("[%v] %v: %v", id, method, path)

	l.h.ServeHTTP(w, r)
}

// function createMachine creates a state machine that stores
// the background process context and cancellation function to
// be used by the server and watchers
func createMachine() state {
	defer logger.Print("created state machine")

	s := state{}
	s.ctx, s.canceller = context.WithCancel(context.Background())
	return s
}

type document struct {
	dir      string
	path     string
	filename string
	contents string
}

// function createDocument creates a reference to a
// markdown file and stores it in a document struct
func createDocument(dir string, entry fs.DirEntry) (*document, error) {
	doc := document{
		dir:      dir,
		filename: entry.Name(),
		path:     fmt.Sprintf("%v/%v", dir, entry.Name()),
	}

	file, err := os.ReadFile(doc.path)

	if err != nil {
		return &doc, err
	}

	doc.contents = string(file)

	return &doc, nil
}

type server struct {
	port        int64
	contentDir  string
	templateDir string
	staticDir   string
	views       []*view.View
	staticPaths []*build.FilePath
	watcher     fsnotify.Watcher
	locks       locks
	handler     http.Handler
}

// function createLocks creates mutex locks for the server
// instance to ensure that there are no race conditions
// between document loading and server lifecycle
func (s *server) createLocks() {
	logger.Print("creating locks")

	s.locks.documentLoader = &sync.RWMutex{}
	s.locks.serverStarter = &sync.RWMutex{}
}

// function addLogger adds logging middleware that wraps the
// mux instance
func (s *server) addLogger() {
	logger.Print("adding logger")

	s.handler = loggingMiddleware{h: s.handler}
}

// function createServer instantiates a server instance
// with the given port/addr and a filesystem directory to
// watch. It also instantiates a new mux instance
func createServer(p int64, dirs ...string) *server {
	if len(dirs) != 3 {
		logger.Fatalf("invalid configuration. should only be 3 dirs")
	}
	s := server{port: p}
	s.contentDir, s.templateDir, s.staticDir = dirs[0], dirs[1], dirs[2]

	return &s
}

// function loadDocuments loads the documents in the docs dir
// and then stores them in the server instance.
//
// Right now the contents of the file are stored in the struct
// but this could prove to be less than performant and not scalable.
func (s *server) loadDocuments() {
	logger.Print("loading documents")

	s.locks.documentLoader.Lock()
	defer s.locks.documentLoader.Unlock()
	s.views = view.NewViews(s.contentDir, s.templateDir)
}

func (s *server) reloadHandler(srv *http.Server) {
	s.loadDocuments()
	s.collectStatic()
	s.addRoutes()
	s.addLogger()

	srv.Handler = s.handler
}

type errorData struct {
	Status int    `json:"statusCode"`
	Err    string `json:"ErrorMessage"`
}

func createErrorJSON(s int, e error) []byte {
	errData := errorData{s, e.Error()}
	data, _ := json.Marshal(errData)
	return data
}

// function addRoutes parses a directory of template files
// and executes them based on html files found in the
// templates directory (defaults to /templates)
//
// TODO: template dir (configurable?)
func (s *server) addRoutes() {
	logger.Print("registering routes")

	mux := http.NewServeMux()
	for _, doc := range s.views {
		path := strings.ToLower(doc.Path)

		route := fmt.Sprintf("/%v", path)
		if path == "index" || path == "readme" {
			route = "/"
		}

		logger.Infof("Registered Route: %v", route)

		// TODO: encapsulate in `build`
		// Build the file in the build dir
		f, err := os.Create(fmt.Sprintf("%v/%v.html", buildDir, path))
		if err != nil {
			logger.Fatalf("unable to create file for route %v\n%v",
				route, err.Error(),
			)
		}

		code, err := f.Write([]byte(doc.HTML))
		if err != nil {
			logger.Fatalf("unable to write file for route %v\n%v (code: %v)",
				route, err.Error(), code,
			)
		}

		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			code, err := w.Write([]byte(doc.HTML))
			if err != nil {
				logger.Errorf("unable to execute template with code %v: %v",
					err.Error(), code,
				)

				data := createErrorJSON(http.StatusInternalServerError, err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(data)
			}
		})
	}

	// TODO: use http.FileServer
	for _, sp := range s.staticPaths {
		r := "/" + strings.ToLower(sp.Name)
		defer logger.Infof("registered static route %v", r)
		mux.HandleFunc(r, func(w http.ResponseWriter, r *http.Request) {
			f, err := os.ReadFile("./" + sp.FileP)
			if err != nil {
				logger.Errorf("unable to read static file %v: %v",
					sp.FileP, err.Error(),
				)
			}

			if strings.HasSuffix(sp.Name, "css") {
				w.Header().Set("Content-Type", "text/css")
				w.Header().Set("Vary", "Accept-Encoding")
			}

			code, err := w.Write(f)
			if err != nil {
				logger.Errorf("file not %v not found (code: %v):\n %v",
					sp.FileP, code, err.Error(),
				)

				data := createErrorJSON(http.StatusNotFound, err)
				w.WriteHeader(http.StatusNotFound)
				w.Write(data)
			}
		})
	}

	s.handler = mux
}

// function watchDocuments instantiates a filesystem watcher that
// responds to the context in the application.
func (s *server) watchDocuments(ctx context.Context, reload chan struct{}) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Errorf("unable to create watcher: %v", err.Error())
	}

	defer watcher.Close()

	if err = watcher.Add(s.contentDir); err != nil {
		logger.Fatalf("unable to read content dir %v", s.contentDir)
	}

	if err = watcher.Add(s.templateDir); err != nil {
		logger.Fatalf("unable to read template dir %v", s.templateDir)
	}

	if err = watcher.Add(s.staticDir); err != nil {
		logger.Fatalf("unable to read static dir %v", s.staticDir)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Infof("stopping watcher...")
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			logger.Debugf("Event: %v | Operation: %v ", event.Name, event.Op.String())
			restart := false
			switch event.Op {
			case fsnotify.Create:
				logger.Debugf("created file %v", event.Name)
				restart = true
				break
			case fsnotify.Remove:
				logger.Debugf("removed file %v", event.Name)
				restart = true
				break
			case fsnotify.Chmod:
			case fsnotify.Write:
				logger.Debugf("modified file %v", event.Name)
				restart = true
				break
			default:
				logger.Warnf("unsupported operation %v", event.Op.String())
			}

			if restart {
				select {
				case reload <- struct{}{}:
				default: // no-op
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}

			if err != nil {
				logger.Errorf("something went wrong: %v", err.Error())
				return
			}
		}
	}
}

var stopSignal = make(chan os.Signal, 1)

// function address is a getter for the address of the server
func (s server) address() string {
	return fmt.Sprintf(":%v", s.port)
}

func (s server) listen(ctx context.Context, reload chan struct{}) {
	srv := &http.Server{Addr: s.address(), Handler: s.handler}

	// graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			if err == http.ErrServerClosed {
				logger.Info("closing server...")
			} else {
				logger.Errorf("something went wrong %v", err.Error())
			}
		}
	}()

	go func() {
		for range reload {
			logger.Infof("reloading documents...")
			s.reloadHandler(srv)
		}
	}()

	if err := srv.ListenAndServe(); err != nil {
		logger.Fatalf("something went wrong %v", err.Error())
	}

}

func (s *server) collectStatic() {
	var err error
	s.staticPaths, err = build.CopyStaticFiles(s.staticDir, buildDir)
	if err != nil {
		logger.Warnf("collecting static files failed\n %v", err.Error())
	}

	defer logger.Infof("copied static files from %v to %v",
		s.staticDir, buildDir,
	)

}

func (s *server) setup() {
	s.createLocks()
	s.loadDocuments()
	s.collectStatic()
	s.addRoutes()
	s.addLogger()
}

// function Run creates filesystem watcher for the provided
// directory and a server that handles requests to the provided
// address. When a change is detected in the filesystem,
// the server is locked and gracefully shutsdown.
//
// Is an ActionFunc for the cli library
func Run(ctx context.Context, c *cli.Command) error {
	dirs := []string{
		c.String("content"),
		c.String("templates"),
		c.String("static"),
	}
	s := createServer(c.Int("port"), dirs...)
	machine := createMachine()
	defer machine.canceller()

	s.setup()

	reload := make(chan struct{}, 1)
	go s.watchDocuments(machine.ctx, reload)
	go func() {
		signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)
		<-stopSignal
		machine.canceller()
	}()

	s.listen(machine.ctx, reload)

	return nil
}
