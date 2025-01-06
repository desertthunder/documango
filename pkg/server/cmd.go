package server

import (
	"fmt"
	"strings"

	// "github.com/desertthunder/documango/pkg/libs/debug"
	"github.com/desertthunder/documango/pkg/build"
	"github.com/desertthunder/documango/pkg/view"
	"github.com/urfave/cli/v3"
)

const defaultPort int64 = 4242

var ServerCommand = &cli.Command{
	Name:      "run",
	Authors:   []any{"Owais"},
	Aliases:   []string{"r", "s", "serve"},
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
	Commands: []*cli.Command{build.BuildCommand, view.ThemeCommand},
	Action:   Run,
}
