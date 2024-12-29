package server

import (
	"fmt"

	"github.com/urfave/cli/v3"
)

var defaultPort int64 = 4242
var defaultDir string = "examples"

var ServerCommand = &cli.Command{
	Name:      "run",
	Authors:   []any{"Owais"},
	Aliases:   []string{"r", "s", "serve"},
	Usage:     "starts the server",
	UsageText: "starts the development server at the provided port",
	Description: `instantiates a filesystem watcher and serves a
	website with pages for each markdown file`,
	ArgsUsage: "[config]",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "port",
			Aliases:     []string{"p", "addr"},
			Required:    false,
			DefaultText: fmt.Sprintf("defaults to port %v", defaultPort),
			Value:       defaultPort,
		},
		&cli.StringFlag{
			Name:        "directory",
			Aliases:     []string{"dir", "path", "d"},
			Required:    false,
			DefaultText: fmt.Sprintf("defaults to /%v", defaultDir),
			Value:       defaultDir,
		},
	},
	Action: Run,
}
