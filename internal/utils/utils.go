// package utils contains helper functions and utilities used application wide.
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// type ErrorData is the serialized error response
type ErrorData struct {
	Status int    `json:"statusCode"`
	Err    string `json:"ErrorMessage"`
}

// function GenerateLogID generates a random 8 digit identifier for logs.
func GenerateLogID(logger *log.Logger) string {
	var id [8]byte
	rand.Read(id[:])

	encoded := hex.EncodeToString(id[:])

	return encoded
}

func CreateErrorJSON(status int, msg error) []byte {
	errData := ErrorData{status, msg.Error()}
	data, _ := json.Marshal(errData)
	return data
}

func IsNotMarkdown(n string) bool {
	p := strings.Split(n, ".")
	return p[len(p)-1] != "md"
}

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

func FindModuleRoot(dir string, l ...*log.Logger) string {
	var logger *log.Logger
	if l == nil {
		logger = log.Default()
	} else {
		logger = l[len(l)-1]
	}
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

func FindWDRoot(l ...*log.Logger) string {
	var logger *log.Logger
	if l == nil {
		logger = log.Default()
	} else {
		logger = l[len(l)-1]
	}

	wd, _ := os.Getwd()

	return FindModuleRoot(wd, logger)
}

func ToJSONString(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

func CreateDir(d string) string {
	os.MkdirAll(d, os.ModePerm)
	return d
}

func CopyFile(fname, src, dest string) (string, error) {
	src_path := fmt.Sprintf("%v/%v", src, fname)
	dest_path := fmt.Sprintf("%v/%v", dest, fname)
	data, err := os.ReadFile(src_path)
	if err != nil {
		return "", fmt.Errorf("unable to read file at %v %v",
			src_path, err.Error(),
		)
	}

	_ = os.Remove(dest_path)
	f, err := os.Create(dest_path)
	if err != nil {
		return "", fmt.Errorf("unable to create file at %v %v",
			src_path, err.Error(),
		)
	}

	code, err := io.WriteString(f, string(data))
	if err != nil {
		return "", fmt.Errorf("unable to write file at %v with code %v %v",
			dest_path, code, err.Error(),
		)
	}

	return dest_path, nil
}
