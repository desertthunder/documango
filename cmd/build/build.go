package build

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/desertthunder/documango/pkg/libs/logs"
	"github.com/desertthunder/documango/pkg/view"
)

var logger = logs.CreateConsoleLogger("[build]")

type FilePath struct {
	FileP string
	Name  string
}

func createBuildDir(d string) (string, error) {
	err := os.MkdirAll(d, 0755)
	if err != nil {
		return "", fmt.Errorf("unable to create build & static assets dir at %v",
			err.Error(),
		)
	}
	return d, err
}

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
	defer logger.Debugf("wrote contents %v to file at %v", string(data), src_path)

	code, err := io.WriteString(f, string(data))
	if err != nil {
		return "", fmt.Errorf("unable to write file at %v with code %v %v",
			dest_path, code, err.Error(),
		)
	}
	return dest_path, nil
}

// CopyStaticFiles creates the build dir at d, the provided destination
// directory as well as the static files directory at {dest}/assets
func CopyStaticFiles(src, d string) ([]*FilePath, error) {
	dest, err := createBuildDir(d + "/assets")
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
			continue
		}

		path, err := CopyFile(fname, src, dest)
		paths = append(paths, &FilePath{path, fname})
		if err != nil {
			logger.Warnf("unable to copy %v from %v to %v", fname, src, dest)
			errs = append(errs, err)
		}
	}
	return paths, nil
}

func BuildHTML(v *view.View) (string, error) {
	path := strings.ToLower(v.Path)
	route := fmt.Sprintf("/%v", path)
	if path == "index" || path == "readme" {
		route = "/"
		path = "index"
	}

	f, err := os.Create(fmt.Sprintf("%v/%v.html", BuildDir, path))
	if err != nil {
		logger.Fatalf("unable to create file for route %v\n%v",
			route, err.Error(),
		)
	}

	code, err := f.Write([]byte(v.HTML))
	if err != nil {
		logger.Fatalf("unable to write file for route %v\n%v (code: %v)",
			route, err.Error(), code,
		)
	}
	return route, err
}
