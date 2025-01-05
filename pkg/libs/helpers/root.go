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
