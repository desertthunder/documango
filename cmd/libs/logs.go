package libs

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

var layout = fmt.Sprintf("06/01/02 at %v", time.Kitchen)

type LogConfig struct {
	Prefix string
	// flag to determine whether or not to create a file logger
	File bool
	// sink for file logger
	Name string
	// flag to determine whether or not to create console logger
	Console bool
}

// CreateFileLogger creates a log directory in the current working
// directory and stores a log file or looks for a pre-existing file.
func CreateFileLogger(conf LogConfig) *log.Logger {
	w, _ := os.Create(conf.Name + ".log")
	return log.NewWithOptions(w, log.Options{
		Prefix:          conf.Prefix,
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      layout,
	})
}

// CreateConsoleLogger creates a stdout logger with the provided
// configuration options.
func CreateConsoleLogger(prefix string) *log.Logger {
	return log.NewWithOptions(os.Stderr, log.Options{
		Prefix:          prefix,
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      layout,
	})
}

func CreateLoggers(conf LogConfig) *log.Logger {
	var w io.Writer

	if conf.File {
		w, _ = os.Create(conf.Name + ".log")
	}

	if conf.Console && conf.File {
		w = io.MultiWriter(os.Stderr, w)
	} else if conf.Console {
		w = os.Stderr
	}

	return log.NewWithOptions(w, log.Options{
		Prefix:          conf.Prefix,
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      layout,
	})
}
