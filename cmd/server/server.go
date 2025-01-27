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

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/build"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/logs"
	"github.com/desertthunder/documango/internal/view"
	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v3"
)

var (
	ServerLogger *log.Logger
	stopSignal   = make(chan os.Signal, 1)
)

type locks struct {
	documentLoader *sync.RWMutex
	serverStarter  *sync.RWMutex
}

type state struct {
	ctx       context.Context
	canceller context.CancelFunc
}

type server struct {
	port        int32
	contentDir  string
	templateDir string
	staticDir   string
	staticRoot  string
	config      *config.Config
	views       []*view.View
	staticPaths []*build.FilePath
	watcher     fsnotify.Watcher
	locks       locks
	handler     http.Handler
	server      *http.Server
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
	ServerLogger.Debug("creating locks")

	s.locks.documentLoader = &sync.RWMutex{}
	s.locks.serverStarter = &sync.RWMutex{}
}

// function addLoggingMiddleware adds logging middleware that wraps the mux instance
func (s *server) addLoggingMiddleware() {
	ServerLogger.Debug("adding logger")

	s.handler = logs.Middleware{Handler: s.handler, MLogger: ServerLogger}
}

// function createServer instantiates a server instance
// with the given port/addr and a filesystem directory to
// watch. It also instantiates a new mux instance
func createServer(config *config.Config) server {
	s := server{
		config:      config,
		port:        config.Options.Port,
		contentDir:  config.Options.ContentDir,
		staticDir:   config.Options.StaticDir,
		templateDir: config.Options.TemplateDir,
		staticRoot:  config.Options.GetStaticPath(),
	}

	return s
}

// function loadViewLayer loads the markup in the content dir
// and then stores them in the server instance.
//
// Right now the contents of the file are stored in the struct
// but this could prove to be less than performant and not scalable.
//
// TODO: handle error value
func (s *server) loadViewLayer() {
	var err error
	ServerLogger.Debug("loading views")

	s.locks.documentLoader.Lock()
	defer s.locks.documentLoader.Unlock()

	s.views, err = view.NewViews(s.config.Options.ContentDir, s.config.Options.TemplateDir)
	if err != nil && len(s.views) > 0 {
		ServerLogger.Warn(err.Error())
	}

	s.staticPaths, _ = build.CopyStaticFiles(s.config)

	build.CollectStatic(s.config)
}

func (s *server) reloadHandler() {
	s.loadViewLayer()
	s.addRoutes()
	s.addLoggingMiddleware()

	s.server.Handler = s.handler
}

// function addRoutes parses a directory of template files and
// executes them based on html files found in the templates
// directory (defaults to /templates)
func (s *server) addRoutes() error {
	ServerLogger.Debug("registering routes")

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(s.staticRoot))))
	ServerLogger.Infof("Serving static files from %v at /assets/", s.staticRoot)

	for _, v := range s.views {
		if route, err := v.BuildHTMLFileContents(s.config); err != nil {
			return fmt.Errorf("unable to build file for route %v %w", route, err)
		} else {
			mux.HandleFunc(route, v.Handler(ServerLogger))
			ServerLogger.Infof("Registered Route: %v", route)
		}
	}

	s.handler = mux

	return nil
}

// function watchFiles instantiates a filesystem watcher that
// responds to the context in the application.
func (s *server) watchFiles(ctx context.Context, reload chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		ServerLogger.Errorf("unable to create watcher: %v", err.Error())
	}

	defer watcher.Close()

	if err = watcher.Add(s.contentDir); err != nil {
		return fmt.Errorf("unable to read content dir %v %w", s.contentDir, err)
	}

	if err = watcher.Add(s.templateDir); err != nil {
		ServerLogger.Warnf("unable to read template dir %v", s.templateDir)
	}

	if err = watcher.Add(s.staticDir); err != nil {
		ServerLogger.Warnf("unable to read static dir %v", s.staticDir)
	}

	for {
		select {
		case <-ctx.Done():
			ServerLogger.Debug("stopping watcher...")
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			ServerLogger.Debugf("Event: %v | Operation: %v ", event.Name, event.Op.String())
			restart := false
			switch event.Op {
			case fsnotify.Create:
				ServerLogger.Debugf("created file %v", event.Name)
				restart = true
				break
			case fsnotify.Remove:
				ServerLogger.Debugf("removed file %v", event.Name)
				restart = true
				break
			case fsnotify.Chmod:
			case fsnotify.Write:
				ServerLogger.Debugf("modified file %v", event.Name)
				restart = true
				break
			default:
				ServerLogger.Warnf("unsupported operation %v", event.Op.String())
			}

			if restart {
				select {
				case reload <- struct{}{}:
				default: // no-op
				}
			}
		case err, _ := <-watcher.Errors:
			return err
		}
	}
}

// function address is a getter for the address of the server
func (s server) address() string {
	return fmt.Sprintf(":%v", s.port)
}

func (s *server) listen(ctx context.Context, reload chan struct{}) error {
	s.server = &http.Server{Addr: s.address(), Handler: s.handler}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			if err == http.ErrServerClosed {
				ServerLogger.Info("closing server...")
			} else {
				ServerLogger.Errorf("something went wrong %v", err.Error())
			}
		}
	}()

	go func() {
		for range reload {
			fmt.Print("\033[H\033[2J")
			ServerLogger.Infof("reloading documents...")

			time.Sleep(500 * time.Millisecond)

			s.reloadHandler()
		}
	}()

	if err := s.server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			ServerLogger.Info("server closed")
		}

		return err
	}

	return nil
}

// function Run is an ActionFunc for the cli library. It creates a filesystem
// watcher for the provided directory and a server that handles requests to the
// provided address. When a change is detected in the filesystem, the server is
// locked and gracefully shutsdown.
func Run(ctx context.Context, c *cli.Command) error {
	ServerLogger = ctx.Value(config.LoggerKey).(*log.Logger)
	conf := ctx.Value(config.ConfKey).(*config.Config)

	logs.SetLogLevel(ServerLogger, conf.Options.Level)

	s := createServer(conf)
	s.createLocks()
	s.loadViewLayer()
	s.staticPaths, _ = build.CopyStaticFiles(s.config)
	s.addRoutes()
	s.addLoggingMiddleware()

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
