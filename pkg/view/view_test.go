package view

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
)

func findModuleRoot(dir string) (roots string) {
	dir = filepath.Clean(dir)
	for {
		p := filepath.Join(dir, "go.mod")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			logger.Info(dir)
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

func ExampleParse() {
	logger.SetLevel(log.InfoLevel)

	wd, _ := os.Getwd()
	root := findModuleRoot(wd)
	f, err := os.Open(fmt.Sprintf("%v/examples/test.md", root))

	if err != nil {
		logger.Fatalf("unable to open file %v", err.Error())
	}

	m := loadMarkup(f)
	m.createAst()

	if logger.GetLevel() == log.DebugLevel {
		data, err := json.MarshalIndent(m.ast, "", "  ")

		if err != nil {
			logger.Fatalf("unable to marshal json: %v", err.Error())
		}

		fpath := fmt.Sprintf("%v/examples/test-ast.json", root)

		if err = os.WriteFile(fpath, data, 0644); err != nil {
			logger.Fatalf("unable to marshal json: %v", err.Error())
		}
	}

	fmt.Print(m.ast.DocType)
	// Output: document
}
