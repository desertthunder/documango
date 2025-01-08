package build

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
	"github.com/urfave/cli/v3"
)

var logger *log.Logger = libs.CreateConsoleLogger("[build]")

var BuildCommand = &cli.Command{
	Name:   "build",
	Usage:  "build your site to your configured directory (defaults to dist)",
	Flags:  config.BuildFlags(true),
	Action: Run,
}

func Run(ctx context.Context, c *cli.Command) error {
	logger = ctx.Value("LOGGER").(*log.Logger)
	lvl := logger.GetLevel()
	conf := ctx.Value(config.ConfKey).(*config.Config)
	views := NewViews(conf.Options.ContentDir, conf.Options.TemplateDir)

	conf.UpdateLogLevel(logger)

	logger.Infof("building site %v", conf.Metadata.Name)

	libs.Pause(lvl)

	if _, err := CollectStatic(conf); err != nil {
		logger.Fatalf("unable to collect static files %v", err.Error())
	} else {
		logger.Info("collected static files ✅")
	}

	for _, v := range views {
		libs.Pause(lvl)

		if _, err := v.BuildHTMLFileContents(conf); err != nil {
			logger.Fatalf("unable to build view %v %v", v.Path, err.Error())
		}

		logger.Infof("built page %v.html (%v)", v.Path, v.name())
	}

	libs.Pause(lvl)

	logger.Infof("built site to %v ✅", conf.Options.BuildDir)

	return nil
}
