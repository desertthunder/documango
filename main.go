// package main is the application entry point for the
// Documango CLI.
//
// Commands:
//
//	documango run		 - starts the server
//
// In Progress:
//
//	documango new		 - creates a documentation directory
//
// Future:
//
//	documango new [type] - create a docs dir and frontmatter schema
//	documango build		 - builds a directory of pages for your files
//	documango deploy 	 - deploy to gh pages, neocities, cloudflare
package main

import (
	"context"
	"os"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
)

func main() {
	if err := config.ConfCommand.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("something went wrong %v", err.Error())
	}
}
