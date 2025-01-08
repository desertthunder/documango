// package config implements methods that handle reading and writing
// from a config.toml file in the root of a project. This will be
// the root command. It passes the config file attrs into context.
//
// TODO: this could be converted to an init &/or check command
package config

import (
	"encoding/json"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/libs"
	"github.com/urfave/cli/v3"
)

const ConfKey string = "CONFIG"
const LoggerKey string = "LOGGER"

var logger = libs.CreateConsoleLogger("[documango 🥭]")

type Config struct {
	Metadata Meta       `toml:"meta"`
	Theme    Theme      `toml:"theme"`
	Options  DevOptions `toml:"dev"`
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

type DevOptions struct {
	Port        int32  `toml:"port"`
	StaticDir   string `toml:"static_dir"`
	TemplateDir string `toml:"template_dir"`
	ContentDir  string `toml:"content_dir"`
	Level       string `toml:"level"`
}

const (
	DefaultContentDir  string = "examples"
	DefaultTemplateDir string = "templates"
	DefaultStaticDir   string = "static"
	BuildDir           string = "dist"
)

func BuildFlags(show bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "content",
			Aliases:  []string{"c", "md"},
			Required: false,

			DefaultText: DefaultContentDir,
			Value:       DefaultContentDir,
			Hidden:      show,
		},
		&cli.StringFlag{
			Name:        "templates",
			Aliases:     []string{"t", "html"},
			Required:    false,
			DefaultText: DefaultTemplateDir,
			Value:       DefaultTemplateDir,
			Hidden:      show,
		},
		&cli.StringFlag{
			Name:     "static",
			Aliases:  []string{"s", "assets"},
			Required: false,
			DefaultText: fmt.Sprintf(
				"static files directory, defaults to %v",
				DefaultStaticDir,
			),
			Value:  DefaultStaticDir,
			Hidden: show,
		},
	}
}

// MergeFlags allows other commands to use the build commands directory
// paths to run while including their own flags by returning a new
// list of Flags
func MergeFlags(flag cli.Flag, show bool) []cli.Flag {
	return append(BuildFlags(show), flag)
}

func NewDefaultConfig() Config {
	return Config{Options: DevOptions{
		Port:        4242,
		StaticDir:   DefaultStaticDir,
		ContentDir:  DefaultContentDir,
		TemplateDir: DefaultTemplateDir,
		Level:       log.InfoLevel.String(),
	}}
}

// function ListThemes peeks in the theme directory and tells the user
// which options they have available.
func ListThemes() {}

func OpenConfig() *Config {
	c := NewDefaultConfig()
	f, err := libs.OpenFileSafe("config.toml")

	if err != nil {
		logger.Fatalf("unable to open config %v", err.Error())
	}

	err = toml.Unmarshal([]byte(f), &c)
	if err != nil {
		logger.Fatalf("unable to parse config %v", err.Error())
	}

	return &c
}

// function render config renders the config file to std out
func RenderConfig() error {
	c := OpenConfig()
	data, err := json.MarshalIndent(c, "", "  ")

	if err != nil {
		return err
	}

	logger.Infof("Config: \n%v", string(data))

	return nil
}

func (d DevOptions) GetStaticPath() string {
	return fmt.Sprintf("./%v/assets", BuildDir)
}

func (c Config) UpdateLogLevel(l *log.Logger) {
	libs.SetLogLevel(l, c.Options.Level)
}
