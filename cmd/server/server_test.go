package server

import (
	"testing"

	"github.com/desertthunder/documango/cmd/libs"
)

func TestServer(t *testing.T) {
	t.Skip()
	logger = libs.CreateConsoleLogger("[server test]")

	t.Run("createMachine creates a state machine that stores a cancellable context",
		func(t *testing.T) {},
	)

	t.Run("adds locks to the server", func(t *testing.T) {})

	t.Run("loading view layer adds list of static file paths to server instance",
		func(t *testing.T) {},
	)

	t.Run("listen opens a connection to the server address",
		func(t *testing.T) {
			t.Run("mutating a file causes a reload signal to dispatch", func(t *testing.T) {})

			t.Run("os.Kill closes process and shuts down the server", func(t *testing.T) {})
		},
	)
}
