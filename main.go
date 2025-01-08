// package main is the application entry point for the
// Documango CLI.
//
// Commands:
//
//	documango run		 - starts the server
//
// In Progress:
//
//	documango new		 - creates a documentation directory
//
// Future:
//
//	documango new [type] - create a docs dir and frontmatter schema
//	documango build		 - builds a directory of pages for your files
//	documango deploy 	 - deploy to gh pages, neocities, cloudflare
package main

import (
	"context"
	"os"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/build"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/cmd/server"
	"github.com/desertthunder/documango/libs"
	"github.com/urfave/cli/v3"
)

var logger = libs.CreateConsoleLogger("[documango 🥭]")

var rootCommand = &cli.Command{
	Name:        "documango",
	Authors:     []any{"Owais (github.com/desertthunder)"},
	Version:     "0.2.0",
	Description: `a cli to quickly generate a static site from a folder of markdown files`,
	Usage:       "generate a static site from a collection of markdown files",
	Flags: config.MergeFlags(
		&cli.StringFlag{
			Name:        "file",
			Aliases:     []string{"f"},
			Usage:       "path to config file",
			Value:       "config.toml",
			DefaultText: "default text",
		}, false),
	Commands: []*cli.Command{server.ServerCommand, build.ThemeCommand, build.BuildCommand},
	Before:   setContext,
}

func setContext(parent context.Context, c *cli.Command) (context.Context, error) {
	conf := config.OpenConfig(c.String("file"))
	ctx := context.WithValue(parent, config.ConfKey, conf)
	ctx = context.WithValue(ctx, config.LoggerKey, logger)
	logger.Debugf("Set context %v", libs.ToJSONString(conf))
	return ctx, nil
}

func main() {
	if err := rootCommand.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("something went wrong %v", err.Error())
	}
}
