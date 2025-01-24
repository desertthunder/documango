package server

import (
	"strings"

	"github.com/desertthunder/documango/internal/config"
	"github.com/urfave/cli/v3"
)

var ServerCommand = &cli.Command{
	Name:      "server",
	Aliases:   []string{"start", "serve"},
	Usage:     "starts the server",
	UsageText: "starts the development server at the provided port",
	Description: strings.Join(
		[]string{
			"instantiates a filesystem watcher and serves ",
			"a website with pages for each markdown file.",
		},
		"\n",
	),
	ArgsUsage: "[config]",
	Flags: config.MergeFlags(
		&cli.IntFlag{
			Name:     "port",
			Aliases:  []string{"p", "addr"},
			Required: false,
		}, true),
	Action: Run,
}
