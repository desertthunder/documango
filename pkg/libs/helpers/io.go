// Reusable helper functions for file I/O
package helpers

import (
	"fmt"
	"os"
)

func OpenFileUnsafe(p string) string {
	f, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(f)
}

func OpenFileSafe(p string) (string, error) {
	f, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(f), nil
}

func CreateAndWriteFile(contents []byte, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	code, err := f.Write(contents)
	if err != nil {
		return fmt.Errorf("(%v) %v", code, err.Error())
	}
	return nil
}
