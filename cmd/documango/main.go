// Command documango turns a folder of Markdown files into a documentation
// website. It previews the site in your browser with live reload, or builds
// it as static files you can host anywhere.
//
// Install it with:
//
//	go install github.com/desertthunder/documango/cmd/documango@latest
//
// Prebuilt binaries for macOS, Linux, and Windows are attached to each
// GitHub release.
//
// # Usage
//
//	documango [dir] [flags]
//	documango [command]
//
// With no command, documango previews dir (the current folder by default),
// the same as "documango serve". The commands are:
//
//	serve    Preview the docs with live reload
//	build    Build a static site, into _site by default
//	init     Create a starter docs folder
//	themes   List the colour themes
//
// For example, to preview the docs folder and then build it for a site
// served from https://example.com/project/:
//
//	documango docs
//	documango build docs --base-path /project/
//
// Run "documango --help" or "documango [command] --help" for every flag.
// The full documentation is at https://desertthunder.github.io/documango/.
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
