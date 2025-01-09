package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

func setupConf() (string, string, *config.Config) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)
	conf := config.OpenConfig(fmt.Sprintf("%v/%v", base_path, "config.toml"))
	return root, base_path, conf
}

var wg = sync.WaitGroup{}

func mutateConf(conf *config.Config) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)

	conf.Options.BuildDir = fmt.Sprintf("%v/%v", base_path, conf.Options.BuildDir)
	conf.Options.TemplateDir = fmt.Sprintf("%v/%v", base_path, conf.Options.TemplateDir)
	conf.Options.ContentDir = fmt.Sprintf("%v/%v", base_path, conf.Options.ContentDir)
	conf.Options.StaticDir = fmt.Sprintf("%v/%v", base_path, conf.Options.StaticDir)
}

func TestServer(t *testing.T) {
	wg.Add(1)

	sb := strings.Builder{}
	ServerLogger = log.Default()
	ServerLogger.SetOutput(&sb)
	_, _, conf := setupConf()

	var machine state

	t.Run("createMachine creates a state machine that stores a cancellable context", func(t *testing.T) {
		machine = createMachine()

		if machine.ctx.Err() != nil {
			t.Fatal()
		}
	})

	machine.ctx = context.TODO()

	t.Run("createServer creates a server with a conf", func(t *testing.T) {
		s := createServer(conf)

		if s.contentDir != conf.Options.ContentDir {
			t.Fail()
		}

		if s.templateDir != conf.Options.TemplateDir {
			t.Fail()
		}

		if s.staticDir != conf.Options.StaticDir {
			t.Fail()
		}
	})

	t.Run("adds locks to the server", func(t *testing.T) {
		s := createServer(conf)
		if s.locks.documentLoader != nil || s.locks.serverStarter != nil {
			t.Error("neither lock should be defined at this point")
		}

		s.createLocks()

		if s.locks.documentLoader == nil || s.locks.serverStarter == nil {
			t.Error("both locks should be defined at this point")
		}
	})

	// At this point we should be mutating the conf so that the static file paths
	mutateConf(conf)

	t.Run("loading view layer adds list of static file paths to server instance", func(t *testing.T) {
		s := createServer(conf)
		s.createLocks()
		entries, err := os.ReadDir(s.staticDir)

		if err != nil {
			t.Errorf("unable to read static dir %v", err.Error())
		}

		var jsFile os.DirEntry

		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".js") {
				jsFile = entry
				break
			}
		}

		if jsFile == nil {
			t.Error("there should be a js file in the test dir but there is not")
		}

		s.loadViewLayer()

		_, err = os.ReadFile(fmt.Sprintf("%v/assets/%v", s.config.Options.BuildDir, jsFile.Name()))

		if err != nil {
			t.Errorf("unable to find js file %v in %v/assets: %v", jsFile.Name(), s.config.Options.BuildDir, err.Error())
		}
	})

	t.Run("addLoggingMiddleware adds a logger", func(t *testing.T) {
		s := createServer(conf)
		s.createLocks()
		s.addRoutes()
		s.addLoggingMiddleware()
		p := fmt.Sprintf("http://localhost:%v", s.port)
		c := &http.Client{}

		t.Run("listen opens a connection to the server address", func(t *testing.T) {
			reload := make(chan struct{}, 1)

			go func() {
				defer wg.Done()
				s.listen(machine.ctx, reload)
			}()

			req, _ := http.NewRequest(http.MethodGet, p, nil)

			_, err := c.Do(req)
			if err != nil {
				t.Fatalf("the server should have handled this %v", err.Error())
			}

			s.server.Shutdown(machine.ctx)
		})

		req, _ := http.NewRequest(http.MethodGet, p, nil)
		if _, err := c.Do(req); err == nil {
			t.Error("the server should be closed and this request should fail")
		}

		out := sb.String()
		if out == "" {
			t.Errorf("string builder should have captured some output %v %v", out, s.config.Options.Port)
		}

		if !strings.Contains(out, "Header") {
			t.Errorf("output from logger: %v should have request headers", out)
		}
	})
}
