package build

import (
	"fmt"
	"io"
	"os"

	"github.com/desertthunder/documango/pkg/libs/logs"
)

var logger = logs.CreateConsoleLogger("[build]")

func createBuildDir(d string) (string, error) {
	err := os.MkdirAll(d, 0755)
	if err != nil {
		return "", fmt.Errorf("unable to create build & static assets dir at %v",
			err.Error(),
		)
	}
	return d, err
}

type FilePath struct {
	FileP string
	Name  string
}

// CopyStaticFiles creates the build dir at d, the provided destination
// directory.
func CopyStaticFiles(src, d string) ([]*FilePath, error) {
	dest, err := createBuildDir(d)
	paths := []*FilePath{}
	if err != nil {
		logger.Fatal(err.Error())
	}
	logger.Infof("created directory %v", dest)

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
			// No-op
		}

		path, err := copyFile(fname, src, dest)
		paths = append(paths, &FilePath{path, fname})
		if err != nil {
			logger.Warnf("unable to copy %v from %v to %v", fname, src, dest)
			errs = append(errs, err)
		}
	}

	return paths, nil
}

func copyFile(fname, src, dest string) (string, error) {
	src_path := fmt.Sprintf("%v/%v", src, fname)
	dest_path := fmt.Sprintf("%v/%v", dest, fname)
	data, err := os.ReadFile(src_path)
	if err != nil {
		return "", fmt.Errorf("unable to read file at %v %v",
			src_path, err.Error(),
		)
	}
	logger.Debugf("read file at %v", src_path)

	// File Syncing
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
	logger.Debugf("wrote contents %v to file at %v", string(data), src_path)

	return dest_path, nil
}
