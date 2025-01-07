package view

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/desertthunder/documango/cmd/libs/helpers"
)

func relPath(r, d string) string {
	return fmt.Sprintf("%v/%v", r, d)
}

func TestReadContentDirectory(t *testing.T) {
	t.Run("creates a list of Views", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("unable to get working dir %v", err.Error())
		}

		root := helpers.FindModuleRoot(wd, logger)
		views := readContentDirectory(relPath(root, "examples"),
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
						logger.Debugf("Markdown:\n%v", got)
					}
				}
			},
		)

		t.Run("each view allows caller to create an HTML string",
			func(t *testing.T) {
				for _, v := range views {
					v.Build()
					got := v.HTMLContent()

					if got == "" {
						t.Fatal("nothing rendered")
					} else {
						logger.Debugf("HTML:\n%v", got)
					}
				}
			},
		)

		t.Run("each view allows caller to create a viewable HTML file",
			func(t *testing.T) {
				for _, v := range views {
					b := strings.Builder{}

					v.Render(&b)
					got := b.String()

					if got == "" {
						t.Fatal("nothing rendered")
					} else {
						logger.Infof("HTML:\n%v", got)
					}
				}
			},
		)
	})
}
