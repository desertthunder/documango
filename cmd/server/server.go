// package server contains the implementation for
// an http server that watches for changes in the
// provided directory.
package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/desertthunder/documango/cmd/build"
	"github.com/desertthunder/documango/cmd/libs"
	"github.com/desertthunder/documango/cmd/libs/logs"
	"github.com/desertthunder/documango/cmd/view"
	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v3"
)

var (
	logger     = logs.CreateConsoleLogger("[server]")
	stopSignal = make(chan os.Signal, 1)
)

type middleware struct {
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

func (m middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	path := r.URL.String()

	logger.Infof("[%v]: %v", method, path)

	m.h.ServeHTTP(w, r)

	for k, v := range w.Header() {
		logger.Debugf("Header %v: %v", k, v)
	}
}

// function createMachine creates a state machine that stores
// the background process context and cancellation function to
// be used by the server and watchers
func createMachine() state {
	s := state{}
	s.ctx, s.canceller = context.WithCancel(context.Background())

	return s
}

// function createLocks creates mutex locks for the server
// instance to ensure that there are no race conditions
// between document loading and server lifecycle
func (s *server) createLocks() {
	logger.Debug("creating locks")

	s.locks.documentLoader = &sync.RWMutex{}
	s.locks.serverStarter = &sync.RWMutex{}
}

// function addLogger adds logging middleware that wraps the
// mux instance
func (s *server) addLogger() {
	logger.Debug("adding logger")

	s.handler = middleware{h: s.handler}
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
	s.setup()
	return &s
}

// function loadViewLayer loads the markup in the content dir
// and then stores them in the server instance.
//
// Right now the contents of the file are stored in the struct
// but this could prove to be less than performant and not scalable.
func (s *server) loadViewLayer() {
	logger.Debug("loading views")

	s.locks.documentLoader.Lock()
	defer s.locks.documentLoader.Unlock()

	s.views = view.NewViews(s.contentDir, s.templateDir)
	s.staticPaths, _ = build.CopyStaticFiles(s.staticDir)

	build.CollectStatic(s.staticDir, build.BuildDir)
}

func (s *server) setup() {
	s.createLocks()
	s.loadViewLayer()
	s.staticPaths, _ = build.CopyStaticFiles(s.staticDir)
	s.addRoutes()
	s.addLogger()
}

func (s *server) reloadHandler(srv *http.Server) {
	s.loadViewLayer()
	s.addRoutes()
	s.addLogger()

	srv.Handler = s.handler
}

// function addRoutes parses a directory of template files and
// executes them based on html files found in the templates
// directory (defaults to /templates)
func (s *server) addRoutes() {
	logger.Debug("registering routes")

	static_fpath := fmt.Sprintf("./%v/assets", build.BuildDir)

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(static_fpath))))
	logger.Infof("Serving static files from %v at /assets/", static_fpath)

	for _, view := range s.views {
		route, err := build.BuildHTMLFileContents(view)
		if err != nil {
			logger.Fatalf("unable to build file for route %v \n%v", route, err.Error())
		}

		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			if code, err := w.Write(view.HTML()); err != nil {
				data := libs.CreateErrorJSON(http.StatusInternalServerError, err)

				w.WriteHeader(http.StatusInternalServerError)
				w.Write(data)

				logger.Errorf("unable to execute template with code %v: %v",
					err.Error(), code,
				)
			}
		})

		logger.Infof("Registered Route: %v", route)
	}

	s.handler = mux
}

// function watchFiles instantiates a filesystem watcher that
// responds to the context in the application.
func (s *server) watchFiles(ctx context.Context, reload chan struct{}) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Errorf("unable to create watcher: %v", err.Error())
	}

	defer watcher.Close()

	// TODO: we need to create this dir if it doesn't exist
	if err = watcher.Add(s.contentDir); err != nil {
		logger.Fatalf("unable to read content dir %v", s.contentDir)
	}

	// TODO: we need to create this dir
	// TODO: should this be watched if the user chooses to use the preconfigured
	// templates?
	if err = watcher.Add(s.templateDir); err != nil {
		logger.Fatalf("unable to read template dir %v", s.templateDir)
	}

	// TODO: same as templateDir
	if err = watcher.Add(s.staticDir); err != nil {
		logger.Fatalf("unable to read static dir %v", s.staticDir)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("stopping watcher...")
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

// function address is a getter for the address of the server
func (s server) address() string {
	return fmt.Sprintf(":%v", s.port)
}

func (s server) listen(ctx context.Context, reload chan struct{}) {
	srv := &http.Server{Addr: s.address(), Handler: s.handler}

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
			fmt.Print("\033[H\033[2J")
			logger.Infof("reloading documents...")
			s.reloadHandler(srv)
		}
	}()

	if err := srv.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			logger.Info("server closed")
			os.Exit(0)
		} else {
			logger.Fatalf("something went wrong %v", err.Error())
		}
	}
}

// function Run is an ActionFunc for the cli library. It creates a filesystem
// watcher for the provided directory and a server that handles requests to the
// provided address. When a change is detected in the filesystem, the server is
// locked and gracefully shutsdown.
func Run(ctx context.Context, c *cli.Command) error {
	dirs := []string{
		c.String("content"),
		c.String("templates"),
		c.String("static"),
	}
	s := createServer(c.Int("port"), dirs...)
	machine := createMachine()
	reload := make(chan struct{}, 1)

	defer machine.canceller()
	go s.watchFiles(machine.ctx, reload)
	go func() {
		signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)
		<-stopSignal
		machine.canceller()
	}()

	s.listen(machine.ctx, reload)

	return nil
}
