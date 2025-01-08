package syntax

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/libs"
)

func ExampleParse() {
	logger.SetLevel(log.InfoLevel)

	wd, _ := os.Getwd()
	root := libs.FindModuleRoot(wd, logger)
	f, err := os.Open(fmt.Sprintf("%v/example/docs/about.md", root))

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

		fpath := fmt.Sprintf("%v/example/test-ast.json", root)

		if err = os.WriteFile(fpath, data, 0644); err != nil {
			logger.Fatalf("unable to marshal json: %v", err.Error())
		}
	}

	fmt.Print(m.ast.DocType)
	// Output: document
}
