// package config implements methods that handle reading and writing
// from a config.toml file in the root of a project. This will be
// the root command.
package config

import (
	"context"
	"encoding/json"

	"github.com/BurntSushi/toml"
	"github.com/desertthunder/documango/cmd/build"
	"github.com/desertthunder/documango/cmd/libs/helpers"
	"github.com/desertthunder/documango/cmd/libs/logs"
	"github.com/desertthunder/documango/cmd/server"
	"github.com/desertthunder/documango/cmd/view"
	"github.com/urfave/cli/v3"
)

type Config struct {
	Meta  Meta  `toml:"meta"`
	Theme Theme `toml:"theme"`
}

type Meta struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Keywords    []string `toml:"keywords"`
	URL         string   `toml:"URL"`
}

type Theme struct {
	Light string `toml:"light"`
	Dark  string `toml:"dark"`
}

var logger = logs.CreateConsoleLogger("[documango 🥭]")

var ConfCommand = &cli.Command{
	Name:        "documango",
	Version:     "0.1.0",
	Description: `a cli to quickly generate a static site from a folder of markdown files`,
	Usage:       "generate a static site from a collection of markdown files",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Aliases:     []string{"f"},
			Usage:       "path to config file",
			Value:       "config.toml",
			DefaultText: "default text",
		},
	},
	Commands: []*cli.Command{server.ServerCommand, view.ThemeCommand, build.BuildCommand},
	Action: func(ctx context.Context, c *cli.Command) error {
		err := RenderConfig()
		if err != nil {
			logger.Fatalf("something went wrong %v", err.Error())
		}
		return nil
	},
}

// function ListThemes peeks in the theme directory and tells the user
// which options they have available.
func ListThemes() {}

// function render config renders the config file to std out
func RenderConfig() error {
	f, err := helpers.OpenFileSafe("config.toml")

	if err != nil {
		logger.Fatalf("unable to open config %v", err.Error())
	}

	logger.Infof("raw config \n%v", f)
	c := Config{}
	err = toml.Unmarshal([]byte(f), &c)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	logger.Infof("Config: \n%v", string(data))

	return nil
}
