// Command documango turns a directory of Markdown into a docs site, served
// with live reload or built as static files. Run "documango --help" for usage.
package main

import (
	"context"
	"os"

	"github.com/desertthunder/documango/internal/cli"
)

// version is set at link time with -ldflags "-X main.version=v1.2.3".
var version string

func main() {
	os.Exit(cli.Execute(context.Background(), os.Args[1:], cli.Env{
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Environ: os.Environ(),
		Version: version,
	}))
}
