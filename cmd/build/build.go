package build

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

//go:embed assets/theme.js
var ScriptFile string

type FilePath struct {
	FileP string
	Name  string
}

type Builder struct {
	Config *config.Config
	Logger *log.Logger
}

var failBuildAndExit func(msg string) = func(msg string) {
	BuildLogger.Fatal(msg)
}

func createStaticBuildDir(c *config.Config) string {
	dest := libs.CreateDir(c.Options.BuildDir + "/assets")
	BuildLogger.Debugf("created directory %v", dest)
	return dest
}

// CopyStaticFiles creates the build dir at d, the provided destination
// directory as well as the static files directory at {dest}/assets
func CopyStaticFiles(c *config.Config) ([]*FilePath, error) {
	paths := []*FilePath{}
	src := c.Options.StaticDir
	dest := createStaticBuildDir(c)
	entries, err := os.ReadDir(src)
	if err != nil {
		return paths, fmt.Errorf("unable to read directory %v %v", src, err.Error())
	}

	for _, entry := range entries {
		fname := entry.Name()
		if entry.IsDir() {
			continue
		}

		path, _ := libs.CopyFile(fname, src, dest)
		paths = append(paths, &FilePath{path, fname})
	}

	theme := BuildTheme()
	theme_path := fmt.Sprintf("%v/styles.css", dest)
	err = libs.CreateAndWriteFile([]byte(theme), theme_path)

	if err != nil {
		BuildLogger.Warnf("unable to write theme to %v/styles.css \n%v", dest, err.Error())
		return paths, nil
	} else {
		paths = append(paths, &FilePath{Name: "styles.css", FileP: theme_path})
	}

	return paths, nil
}

func CollectStatic(c *config.Config) ([]*FilePath, error) {
	b := c.Options.BuildDir
	defer BuildLogger.Infof("copied static files from %v to %v", c.Options.StaticDir, b)
	static_paths, err := CopyStaticFiles(c)

	if err != nil {
		BuildLogger.Warnf("collecting static files failed: %v", err.Error())

		_ = CopyJS(c)
	}

	theme := BuildTheme()
	// The failure case here is when the file exists but that is handled by CopyFile
	libs.CreateAndWriteFile([]byte(theme), fmt.Sprintf("%v/assets/styles.css", b))

	return static_paths, err
}

// When using the default template, {views}/base, we want to bundle assets/theme.js
// to ensure that the user can access the basic light/dark toggler.
//
// TODO: this should be configurable
func CopyJS(conf *config.Config) error {
	fs, err := os.Stat(conf.Options.TemplateDir)

	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fpath := fmt.Sprintf("%v/assets/theme.js", conf.Options.BuildDir)
			f, err := os.Create(fpath)

			if err != nil {
				BuildLogger.Errorf("sww %v", err.Error())
				return err
			}

			if _, err = f.Write([]byte(ScriptFile)); err != nil {
				BuildLogger.Errorf("sww %v", err.Error())
				return err
			}

			BuildLogger.Info("copied theme.js to /dist/")
			return nil
		} else {
			BuildLogger.Errorf("sww %v", err.Error())
			return err
		}
	}

	if fs.IsDir() {
		BuildLogger.Info("template directory present, using custom theme")
		return nil
	}

	return nil
}
