package build

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/logs"
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
	views := NewViews(conf.Options.ContentDir, conf.Options.TemplateDir)
	lvl := BuildLogger.GetLevel()

	conf.UpdateLogLevel(BuildLogger)

	BuildLogger.Infof("building site %v", conf.Metadata.Name)

	logs.Pause(lvl)

	if _, err := CollectStatic(conf); err != nil {
		BuildLogger.Fatalf("unable to collect static files %v", err.Error())
	} else {
		BuildLogger.Info("collected static files ✅")
	}

	for _, v := range views {
		logs.Pause(lvl)

		if _, err := v.BuildHTMLFileContents(conf); err != nil {
			BuildLogger.Fatalf("unable to build view %v %v", v.Path, err.Error())
		}

		BuildLogger.Infof("built page %v.html (%v)", v.Path, v.name())
	}

	logs.Pause(lvl)

	BuildLogger.Infof("built site to %v ✅", conf.Options.BuildDir)

	return nil
}
