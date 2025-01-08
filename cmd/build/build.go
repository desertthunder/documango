package build

import (
	"fmt"
	"os"

	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

type FilePath struct {
	FileP string
	Name  string
}

// CopyStaticFiles creates the build dir at d, the provided destination
// directory as well as the static files directory at {dest}/assets
func CopyStaticFiles(conf *config.Config) ([]*FilePath, error) {
	src := conf.Options.StaticDir
	dest, err := libs.CreateDir(conf.Options.BuildDir + "/assets")
	paths := []*FilePath{}
	if err != nil {
		logger.Fatal(err.Error())
	}

	logger.Debugf("created directory %v", dest)

	entries, err := os.ReadDir(src)
	if err != nil {
		return paths, fmt.Errorf("unable to read directory %v %v",
			src, err.Error(),
		)
	}

	errs := []error{}
	for _, entry := range entries {
		fname := entry.Name()
		if entry.IsDir() {
			continue
		}

		path, err := libs.CopyFile(fname, src, dest)
		paths = append(paths, &FilePath{path, fname})
		if err != nil {
			logger.Warnf("unable to copy %v from %v to %v", fname, src, dest)
			errs = append(errs, err)
		}
	}

	theme := BuildTheme()
	theme_path := fmt.Sprintf("%v/styles.css", dest)
	err = libs.CreateAndWriteFile([]byte(theme), theme_path)

	if err != nil {
		logger.Warnf("unable to write theme to %v/styles.css \n%v", dest, err.Error())
		return paths, nil
	} else {
		paths = append(paths, &FilePath{Name: "styles.css", FileP: theme_path})
	}

	return paths, nil
}

func CollectStatic(c *config.Config) ([]*FilePath, error) {
	b := c.Options.BuildDir
	defer logger.Infof("copied static files from %v to %v", c.Options.StaticDir, b)
	static_paths, err := CopyStaticFiles(c)

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
