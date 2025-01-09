package server

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

func setupConf() (string, string, *config.Config) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)
	conf := config.OpenConfig(fmt.Sprintf("%v/%v", base_path, "config.toml"))
	return root, base_path, conf
}

func mutateConf(conf *config.Config) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)

	conf.Options.BuildDir = fmt.Sprintf("%v/%v", base_path, conf.Options.BuildDir)
	conf.Options.TemplateDir = fmt.Sprintf("%v/%v", base_path, conf.Options.TemplateDir)
	conf.Options.ContentDir = fmt.Sprintf("%v/%v", base_path, conf.Options.ContentDir)
	conf.Options.StaticDir = fmt.Sprintf("%v/%v", base_path, conf.Options.StaticDir)
}

func TestServer(t *testing.T) {
	logger = libs.CreateConsoleLogger("[server test]")
	_, _, conf := setupConf()

	t.Run("createMachine creates a state machine that stores a cancellable context", func(t *testing.T) {
		s := createMachine()

		if s.ctx.Err() != nil {
			t.Fail()
		}
	})

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

	var s server
	t.Run("adds locks to the server", func(t *testing.T) {
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

	t.Run("listen opens a connection to the server address", func(t *testing.T) {
		t.Skip()
		t.Run("mutating a file causes a reload signal to dispatch", func(t *testing.T) {})
		t.Run("os.Kill closes process and shuts down the server", func(t *testing.T) {})
	})
}
