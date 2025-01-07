package server

import (
	"fmt"
	"strings"

	// "github.com/desertthunder/documango/cmd/libs/debug"
	"github.com/desertthunder/documango/cmd/build"
	"github.com/urfave/cli/v3"
)

const defaultPort int64 = 4242

var ServerCommand = &cli.Command{
	Name:      "server",
	Authors:   []any{"Owais (github.com/desertthunder)"},
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
	Flags: build.MergeFlags(&cli.IntFlag{
		Name:        "port",
		Aliases:     []string{"p", "addr"},
		Required:    false,
		DefaultText: fmt.Sprintf("%v", defaultPort),
		Value:       defaultPort,
	}),
	Action: Run,
}
