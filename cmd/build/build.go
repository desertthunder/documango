package build

import (
	"fmt"
	"io"
	"os"

	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

type FilePath struct {
	FileP string
	Name  string
}

// TODO: CreateDir helper?
func createBuildDir(d string) (string, error) {
	err := os.MkdirAll(d, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("unable to create build & static assets dir at %v",
			err.Error(),
		)
	}
	return d, err
}

// TODO: Move to helpers/libs
func CopyFile(fname, src, dest string) (string, error) {
	src_path := fmt.Sprintf("%v/%v", src, fname)
	dest_path := fmt.Sprintf("%v/%v", dest, fname)
	data, err := os.ReadFile(src_path)
	if err != nil {
		return "", fmt.Errorf("unable to read file at %v %v",
			src_path, err.Error(),
		)
	}
	logger.Debugf("read file at %v", src_path)

	_ = os.Remove(dest_path)
	f, err := os.Create(dest_path)
	if err != nil {
		return "", fmt.Errorf("unable to create file at %v %v",
			src_path, err.Error(),
		)
	}

	logger.Debugf("created file at %v", dest_path)

	code, err := io.WriteString(f, string(data))
	if err != nil {
		return "", fmt.Errorf("unable to write file at %v with code %v %v",
			dest_path, code, err.Error(),
		)
	}

	logger.Debugf("wrote contents to file at %v", src_path)
	return dest_path, nil
}

// CopyStaticFiles creates the build dir at d, the provided destination
// directory as well as the static files directory at {dest}/assets
func CopyStaticFiles(conf *config.Config) ([]*FilePath, error) {
	src := conf.Options.StaticDir
	dest, err := createBuildDir(conf.Options.BuildDir + "/assets")
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

		path, err := CopyFile(fname, src, dest)
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
