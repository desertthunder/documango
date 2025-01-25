package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/build"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/utils"
)

func setupConf() (string, string, *config.Config) {
	root := utils.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)
	conf := config.OpenConfig(fmt.Sprintf("%v/%v", base_path, "config.toml"))
	return root, base_path, conf
}

func mutateConf(conf *config.Config) {
	root := utils.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)

	conf.Options.BuildDir = fmt.Sprintf("%v/%v", base_path, conf.Options.BuildDir)
	conf.Options.TemplateDir = fmt.Sprintf("%v/%v", base_path, conf.Options.TemplateDir)
	conf.Options.ContentDir = fmt.Sprintf("%v/%v", base_path, conf.Options.ContentDir)
	conf.Options.StaticDir = fmt.Sprintf("%v/%v", base_path, conf.Options.StaticDir)
}

func TestServer(t *testing.T) {
	wg := sync.WaitGroup{}

	sb := strings.Builder{}
	ServerLogger = log.Default()
	ServerLogger.SetOutput(&sb)
	build.BuildLogger = ServerLogger
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
			t.Fatal("there should be a js file in the test dir but there is not")
		}

		s.loadViewLayer()

		_, err = os.ReadFile(fmt.Sprintf("%v/assets/%v", s.config.Options.BuildDir, jsFile.Name()))

		if err != nil {
			t.Fatalf("unable to find js file %v in %v/assets: %v", jsFile.Name(), s.config.Options.BuildDir, err.Error())
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
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.listen(machine.ctx, reload)
			}()

			req, _ := http.NewRequest(http.MethodGet, p, nil)

			_, err := c.Do(req)
			if err != nil {
				t.Fatalf("the server should have handled this %v", err.Error())
			}

			time.Sleep(3 * time.Second)
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

	t.Run("Run Command", func(t *testing.T) {
		wg.Wait()
		sb := strings.Builder{}
		ServerLogger = log.Default()
		ServerLogger.SetOutput(&sb)
		_, _, conf := setupConf()
		mutateConf(conf)
		ctx := context.TODO()
		ctx = context.WithValue(ctx, config.ConfKey, conf)
		ctx = context.WithValue(ctx, config.LoggerKey, ServerLogger)
		ctx, cancelFunc := context.WithCancel(ctx)
		var err error

		wg.Add(1)
		go func() {
			<-ctx.Done()
			defer wg.Done()
			err = ServerCommand.Run(ctx, []string{})
		}()

		time.Sleep(2 * time.Second)

		cancelFunc()

		if err != nil {
			t.Errorf("execution failed %v", err.Error())
		}
	})

	t.Run("watchFiles", func(t *testing.T) {
		sb.Reset()
		ServerLogger = log.Default()
		ServerLogger.SetOutput(&sb)

		_, _, conf := setupConf()

		mutateConf(conf)

		ctx := context.TODO()
		ctx = context.WithValue(ctx, config.LoggerKey, ServerLogger)
		ctx, cancelFunc := context.WithCancel(ctx)

		wg.Add(1)
		s := createServer(conf)
		s.createLocks()
		s.addRoutes()
		s.addLoggingMiddleware()

		go func() {
			<-ctx.Done()
			defer wg.Done()
			s.watchFiles(ctx, make(chan struct{}))
		}()
		tmp_path := fmt.Sprintf("%v/test.md", conf.Options.ContentDir)

		time.Sleep(2 * time.Second)

		// Modify file in docs dir
		f, err := os.Create(tmp_path)
		if err != nil {
			t.Fatalf("unable to create file %v", err.Error())
		}

		_, err = f.Write([]byte("# Test"))
		if err != nil {
			os.Remove(tmp_path)
			t.Fatalf("unable to write to file %v", err.Error())
		}

		f.Close()
		time.Sleep(2 * time.Second)

		cancelFunc()
		os.Remove(tmp_path)

		out := sb.String()

		if !strings.Contains(out, "reload") {
			t.Errorf("watchFiles should have logged a reload event %v", out)
		}
	})
}
