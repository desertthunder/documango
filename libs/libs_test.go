package libs

import (
	"fmt"
	"net/http"
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
			}
		}

	})

	t.Run("CreateErrorJSON", func(t *testing.T) {
		data := CreateErrorJSON(http.StatusNotFound, fmt.Errorf("not found"))
		if !strings.Contains(string(data), "404") {
			t.Error("should have status code")
		}

		if !strings.Contains(string(data), "not found") {
			t.Error("should message")
		}
	})

	root := FindWDRoot()
	t.Run("open file unsafe", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Error("this should not panic")
			}
		}()
		_ = OpenFileUnsafe(fmt.Sprintf("%v/README.md", root))

	})

	t.Run("open file safe", func(t *testing.T) {

		f, err := OpenFileSafe(fmt.Sprintf("%v/README.md", root))

		if err != nil {
			t.Errorf("failed to open file %v", err.Error())
		}

		if len(f) < 1 {
			t.Error("file opened should have content")
		}
	})

	t.Run("create dir", func(t *testing.T) {
		d, err := CreateDir(fmt.Sprintf("%v/test", root))

		if err != nil {
			t.Errorf("couldn't create dir %v", err.Error())
		} else {
			err = os.Remove(d)

			if err != nil {
				t.Logf("clean up failed %v", err.Error())
			}
		}

	})
}
