/*
References:

	https://theapache64.github.io/posts/caveman-debugging-using-live-templates/
	https://dusted.codes/creating-a-pretty-console-logger-using-gos-slog-package
*/
package debug

import (
	"context"

	"github.com/urfave/cli/v3"
)

var Logger = NewLogger(DebugConf())

var DebugCmd = &cli.Command{
	Name:      "debug",
	Aliases:   []string{"dbg"},
	Usage:     "caveman debugging",
	UsageText: "debug - print the current function/feature you're working on",
	Description: `a "caveman debugger"/sandbox to look at helpers or other lib
	functions you're working on`,
	Action: func(ctx context.Context, cmd *cli.Command) error {
		Logger.Debug("hello world!")
		return nil
	},
}
