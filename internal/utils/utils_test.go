package utils

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/desertthunder/documango/internal/logs"
)

type TestValue struct {
	Some  string `json:"some"`
	Value string `json:"value"`
}

func TestLibsPackage(t *testing.T) {
	logger := logs.CreateConsoleLogger("[libs test]")

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
		d := CreateDir(fmt.Sprintf("%v/test", root))

		err := os.Remove(d)
		if err != nil {
			t.Logf("clean up failed %v", err.Error())
		}
	})

	t.Run("create & write file", func(t *testing.T) {
		err := CreateAndWriteFile([]byte("{}"), "tmp.json")

		if err != nil {
			t.Errorf("failed to create file %v", err.Error())
		}

		f, _ := os.ReadFile("tmp.json")

		if string(f) != "{}" {
			t.Error("wrong contents written")
		}

		os.Remove("tmp.json")
	})

	t.Run("ToJSONString", func(t *testing.T) {
		want := TestValue{"test", "value"}

		got := ToJSONString(want)

		if !strings.Contains(got, "test") {
			t.Errorf("invalid serialization %v", got)
		}
	})

	t.Run("CreateDir", func(t *testing.T) {
		d := CreateDir("temp")

		i, err := os.Stat(d)

		if err != nil {
			t.Error(err.Error())
		}

		if i.IsDir() != true {
			t.Errorf("%v should be a dir but it is not", d)
		}

		os.Remove("temp")
	})

	t.Run("CopyFile", func(t *testing.T) {
		err := CreateAndWriteFile([]byte("{}"), "tmp.json")

		if err != nil {
			t.Errorf("failed to create file %v", err.Error())
		}

		_, err = CopyFile("tmp.json", ".", ".")
		if err != nil {
			t.Errorf("failed to copy file %v", err.Error())
		}

		os.Remove("tmp.json")
		os.Remove("tmp.json")
	})

	t.Run("Error states for lib functions", func(t *testing.T) {
		t.Run("OpenFileUnsafe", func(t *testing.T) {
			content := OpenFileUnsafe("non-existent-file.md")

			if len(content) > 0 {
				t.Error("should have failed to open file")
			}
		})

		t.Run("OpenFileSafe", func(t *testing.T) {
			_, err := OpenFileSafe("non-existent-file.md")

			if err == nil {
				t.Error("should have failed to open file")
			}
		})

		t.Run("CreateAndWriteFile", func(t *testing.T) {
			err := CreateAndWriteFile([]byte("{}"), "/tmp/")

			if err == nil {
				t.Error("should have failed to write file")
			}
		})
	})
}
