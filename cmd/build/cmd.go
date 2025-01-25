package build

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/logs"
	"github.com/desertthunder/documango/internal/view"
	"github.com/urfave/cli/v3"
)

var BuildLogger *log.Logger = logs.CreateConsoleLogger("[build]")

var BuildCommand = &cli.Command{
	Name:   "build",
	Usage:  "build your site to your configured directory (defaults to dist)",
	Flags:  config.BuildFlags(true),
	Action: Run,
}

func Run(ctx context.Context, c *cli.Command) error {
	BuildLogger = ctx.Value(config.LoggerKey).(*log.Logger)
	conf := ctx.Value(config.ConfKey).(*config.Config)
	views, err := view.NewViews(conf.Options.ContentDir, conf.Options.TemplateDir)
	if err != nil && len(views) > 0 {
		BuildLogger.Warn(err.Error())
	}

	level := BuildLogger.GetLevel()

	conf.UpdateLogLevel(BuildLogger)

	BuildLogger.Infof("building site %v", conf.Metadata.Name)

	logs.Pause(level)

	if _, err := CollectStatic(conf); err != nil {
		return fmt.Errorf("unable to collect static files %w", err)
	} else {
		BuildLogger.Info("collected static files ✅")
	}

	for _, v := range views {
		logs.Pause(level)

		if _, err := v.BuildHTMLFileContents(conf); err != nil {
			return fmt.Errorf("unable to build view %v %w", v.Path, err)
		}

		BuildLogger.Infof("built page %v.html (%v)", v.Path, v.Name())
	}

	logs.Pause(level)

	BuildLogger.Infof("built site to %v ✅", conf.Options.BuildDir)

	return nil
}
