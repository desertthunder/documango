// package config implements methods that handle reading and writing
// from a config.toml file in the root of a project. This will be
// the root command. It passes the config file attrs into context.
package config

import (
	_ "embed"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/logs"
	"github.com/desertthunder/documango/internal/utils"
	"github.com/urfave/cli/v3"
)

const ConfKey string = "CONFIG"
const LoggerKey string = "LOGGER"

//go:embed config.toml
var DefaultConfigFile []byte

var logger = logs.CreateConsoleLogger("[documango 🥭]")

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
	BuildDir    string `toml:"build_dir"`
	Level       string `toml:"level"`
}

func BuildFlags(show bool) []cli.Flag {
	c := NewDefaultConfig()
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "content",
			Aliases:     []string{"c", "md"},
			Required:    false,
			DefaultText: c.Options.ContentDir,
			Value:       c.Options.ContentDir,
			Hidden:      show,
		},
		&cli.StringFlag{
			Name:        "templates",
			Aliases:     []string{"t", "html"},
			Required:    false,
			DefaultText: c.Options.TemplateDir,
			Value:       c.Options.TemplateDir,
			Hidden:      show,
		},
		&cli.StringFlag{
			Name:        "static",
			Aliases:     []string{"s", "assets"},
			Required:    false,
			DefaultText: fmt.Sprintf("static files directory, defaults to %v", c.Options.StaticDir),
			Value:       c.Options.StaticDir,
			Hidden:      show,
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
	c := Config{}
	toml.Unmarshal([]byte(DefaultConfigFile), &c)
	return c
}

// function ListThemes peeks in the theme directory and tells the user
// which options they have available.
// func ListThemes() {}

func OpenConfig(p string) *Config {
	c := NewDefaultConfig()
	f, _ := utils.OpenFileSafe(p)

	err := toml.Unmarshal([]byte(f), &c)
	if err != nil {
		logger.Fatalf("unable to parse config %v", err.Error())
	}

	return &c
}

func (d DevOptions) GetStaticPath() string {
	return fmt.Sprintf("./%v/assets", d.BuildDir)
}

func (c Config) UpdateLogLevel(l *log.Logger) {
	logs.SetLogLevel(l, c.Options.Level)
}
