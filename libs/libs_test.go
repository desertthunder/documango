package libs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestLibsPackage(t *testing.T) {
	logger := CreateConsoleLogger("[libs test]")

	t.Run("GenerateLogID", func(t *testing.T) {
		id := GenerateLogID(logger)
		if len(id) < 8 {
			t.Errorf("invalid length of id %v", id)
		}

		_, err := strconv.Atoi(id)

		if err == nil {
			t.Errorf("id shouldn't be an integer but it is")
		}
	})

	t.Run("FindWDRoot finds the root directory of the project", func(t *testing.T) {
		root := FindWDRoot()

		if strings.Contains(root, "cmd") {
			t.Fatalf("incorrect root found")
		}

	})

	t.Run("FindModuleRoot finds the root directory of the provided function", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("unable to get cwd %v", err.Error())
		}

		root := FindModuleRoot(cwd)
		if strings.Contains(root, "cmd") {
			t.Fatalf("incorrect root found")
		}
	})

	t.Run("IsNotMarkdown returns false for files that aren't md files", func(t *testing.T) {
		root := FindWDRoot()
		dir := fmt.Sprintf("%v/example/docs", root)

		d, err := os.ReadDir(dir)

		if err != nil {
			t.Fatalf("unable to open dir %v", dir)
		}

		for _, f := range d {
			if strings.HasSuffix(f.Name(), ".md") != IsNotMarkdown(f.Name()) {
				continue
			} else {
				t.Errorf("%v should be marked as not markdown but it was", f)
				t.Fail()
			}
		}

	})

	t.Run("CreateErrorJSON", func(t *testing.T) {
		t.Skip()
	})
}
