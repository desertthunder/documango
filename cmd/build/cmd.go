package build

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/cmd/libs"
	"github.com/urfave/cli/v3"
)

var logger *log.Logger

var BuildCommand = &cli.Command{
	Name:   "build",
	Usage:  fmt.Sprintf("build your site to dir %v", config.BuildDir),
	Flags:  config.BuildFlags(true),
	Action: Run,
}

func pause() {
	if logger.GetLevel() == log.DebugLevel {
		return
	}

	time.Sleep(time.Millisecond * 500)
}

func Run(ctx context.Context, c *cli.Command) error {
	logger = ctx.Value("LOGGER").(*log.Logger)
	conf := ctx.Value(config.ConfKey).(*config.Config)
	opts := conf.Options
	views := NewViews(opts.ContentDir, opts.TemplateDir)

	conf.UpdateLogLevel(logger)

	logger.Infof("building site %v", conf.Metadata.Name)

	pause()
	if _, err := CollectStatic(opts.StaticDir, config.BuildDir); err != nil {
		logger.Fatalf("unable to collect static files %v", err.Error())
	} else {
		logger.Info("collected static files ✅")
	}

	for _, v := range views {
		pause()
		if _, err := v.BuildHTMLFileContents(conf); err != nil {
			logger.Fatalf("unable to build view %v %v", v.Path, err.Error())
		}

		logger.Infof("built page %v.html (%v)", v.Path, v.name())
	}

	pause()

	logger.Infof("built site to %v ✅", config.BuildDir)

	return nil
}

func CollectStatic(s, b string) ([]*FilePath, error) {
	defer logger.Infof("copied static files from %v to %v", s, b)
	static_paths, err := CopyStaticFiles(s)

	if err != nil {
		logger.Warnf("collecting static files failed\n %v", err.Error())
	}

	theme := BuildTheme()
	err = libs.CreateAndWriteFile([]byte(theme), fmt.Sprintf("%v/assets/styles.css", b))

	if err != nil {
		logger.Fatalf("unable to generate theme %v", err.Error())
	}

	return static_paths, err
}
