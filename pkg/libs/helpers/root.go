package helpers

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
)

func FindModuleRoot(dir string, logger *log.Logger) (roots string) {
	dir = filepath.Clean(dir)
	for {
		p := filepath.Join(dir, "go.mod")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			if logger != nil {
				logger.Info(dir)
			}
			return dir
		} else if err != nil {
			d := filepath.Dir(dir)
			dir = d
		} else {
			break
		}

	}

	return ""
}

func FindWDRoot(l ...*log.Logger) (roots string) {
	var logger *log.Logger
	if l == nil {
		logger = log.Default()
	} else {
		logger = l[len(l)-1]
	}

	wd, _ := os.Getwd()

	return FindModuleRoot(wd, logger)
}
