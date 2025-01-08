package build

import (
	"context"
	"fmt"

	"github.com/desertthunder/documango/cmd/libs"
	"github.com/urfave/cli/v3"
)

const (
	DefaultContentDir  string = "examples"
	DefaultTemplateDir string = "templates"
	DefaultStaticDir   string = "static"
	BuildDir           string = "dist"
)

var BuildFlags []cli.Flag = []cli.Flag{
	&cli.StringFlag{
		Name:        "content",
		Aliases:     []string{"c", "md"},
		Required:    false,
		DefaultText: DefaultContentDir,
		Value:       DefaultContentDir,
	},
	&cli.StringFlag{
		Name:        "templates",
		Aliases:     []string{"t", "html"},
		Required:    false,
		DefaultText: DefaultTemplateDir,
		Value:       DefaultTemplateDir,
	},
	&cli.StringFlag{
		Name:     "static",
		Aliases:  []string{"s", "assets"},
		Required: false,
		DefaultText: fmt.Sprintf(
			"static files directory, defaults to %v",
			DefaultStaticDir,
		),
		Value: DefaultStaticDir,
	},
}

var BuildCommand = &cli.Command{
	Name:   "build",
	Usage:  fmt.Sprintf("build your site to dir %v", BuildDir),
	Flags:  BuildFlags,
	Action: Run,
}

// MergeFlags allows other commands to use the build commands directory
// paths to run while including their own flags by returning a new
// list of Flags
func MergeFlags(flag cli.Flag) []cli.Flag {
	return append(BuildFlags, flag)
}

func Run(ctx context.Context, c *cli.Command) error {
	dirs := []string{c.String("content"), c.String("templates"), c.String("static")}
	contentDir, templateDir, staticDir := dirs[0], dirs[1], dirs[2]
	views := NewViews(contentDir, templateDir)

	if _, err := CollectStatic(staticDir, BuildDir); err != nil {
		logger.Fatalf("unable to collect static files %v", err.Error())
	}

	defer logger.Infof("built site to %v ✅", BuildDir)

	for _, v := range views {
		if _, err := BuildHTMLFileContents(v); err != nil {
			logger.Fatalf("unable to build view %v %v", v.Path, err.Error())
		}
	}
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
