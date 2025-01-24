package build

import (
	"fmt"
	"os"
	"testing"

	"github.com/desertthunder/documango/internal/logs"
	"github.com/desertthunder/documango/internal/utils"
)

func relPath(r, d string) string {
	return fmt.Sprintf("%v/%v", r, d)
}

func TestReadContentDirectory(t *testing.T) {
	BuildLogger = logs.CreateConsoleLogger("[test]")
	t.Run("creates a list of Views", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("unable to get working dir %v", err.Error())
		}

		root := utils.FindModuleRoot(wd, BuildLogger)
		views := readContentDirectory(relPath(root, "example"),
			relPath(root, "templates"))

		got := len(views)
		want := 2

		if got != want {
			t.Fatalf("got %v but want %v", got, want)
		}

		t.Run("each view allows caller to access its file contents",
			func(t *testing.T) {
				for _, v := range views {
					got := v.Content()
					if got == "" {
						t.Fatal("expected content but got none")
					} else {
						BuildLogger.Debugf("Markdown:\n%v", got)
					}
				}
			},
		)

	})
}
