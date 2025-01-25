/*
package build creates in-memory HTML documents for use by
the server & build commands.

In its simplest form, our View type contains a reference
to the contents of a markdown file and contains implementations
for methods that create a document using one of the following:

 1. a template in its frontmatter
 2. a template with the same name as the file (sans extensions)
 3. the base template

Then executes (renders) the template by placing it in some stream,
be it file, stdout or stderr.
*/
package build

import (
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/theme"
	"github.com/desertthunder/documango/internal/utils"
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
	dest := utils.CreateDir(c.Options.BuildDir + "/assets")
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
		if os.IsNotExist(err) {
			CopyJS(c)
		}

		return paths, fmt.Errorf("unable to read directory %v %w", src, err)
	}

	for _, entry := range entries {
		fname := entry.Name()
		if entry.IsDir() {
			continue
		}

		path, _ := utils.CopyFile(fname, src, dest)
		paths = append(paths, &FilePath{path, fname})
	}

	theme, err := theme.BuildTheme()
	if err != nil && theme == "" {
		return paths, err
	} else if err != nil {
		BuildLogger.Warn(err.Error())
	}

	theme_path := fmt.Sprintf("%v/styles.css", dest)
	err = utils.CreateAndWriteFile([]byte(theme), theme_path)

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
		if os.IsNotExist(errors.Unwrap(err)) {
			BuildLogger.Warnf("collecting static files failed: %v", err.Error())

			CopyJS(c)
		} else {
			return nil, err
		}
	}

	theme, err := theme.BuildTheme()
	if err != nil && theme == "" {
		return static_paths, err
	} else if err != nil {
		BuildLogger.Warn(err.Error())
	}

	// The failure case here is when the file exists but that is handled by CopyFile
	utils.CreateAndWriteFile([]byte(theme), fmt.Sprintf("%v/assets/styles.css", b))

	return static_paths, nil
}

// When using the default template, {views}/base, we want to bundle assets/theme.js
// to ensure that the user can access the basic light/dark toggler.
//
// TODO: this should be configurable
func CopyJS(conf *config.Config) error {
	fs, err := os.Stat(conf.Options.TemplateDir)

	if err != nil {
		if os.IsNotExist(err) {
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
