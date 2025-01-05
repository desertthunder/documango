package server

import (
	"fmt"
	"strings"

	// "github.com/desertthunder/documango/pkg/libs/debug"
	"github.com/urfave/cli/v3"
)

const (
	defaultPort        int64  = 4242
	defaultContentDir  string = "examples"
	defaultTemplateDir string = "templates"
	defaultStaticDir   string = "static"
	buildDir           string = "dist"
)

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
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "port",
			Aliases:     []string{"p", "addr"},
			Required:    false,
			DefaultText: fmt.Sprintf("%v", defaultPort),
			Value:       defaultPort,
		},
		&cli.StringFlag{
			Name:        "content",
			Aliases:     []string{"c", "md"},
			Required:    false,
			DefaultText: defaultContentDir,
			Value:       defaultContentDir,
		},
		&cli.StringFlag{
			Name:        "templates",
			Aliases:     []string{"t", "html"},
			Required:    false,
			DefaultText: defaultTemplateDir,
			Value:       defaultTemplateDir,
		},
		&cli.StringFlag{
			Name:     "static",
			Aliases:  []string{"s", "assets"},
			Required: false,
			DefaultText: fmt.Sprintf(
				"static files directory, defaults to %v",
				defaultStaticDir,
			),
			Value: defaultStaticDir,
		},
	},
	// Commands: []*cli.Command{debug.DebugCmd},
	Action: Run,
}
