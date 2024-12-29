// package main is the application entry point for the
// Documango CLI.
//
// Commands:
//
//	documango new
//	documango run
//	documango create
package main

import (
	"context"
	"os"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/pkg/server"
)

func main() {
	if err := server.ServerCommand.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("something went wrong %v", err.Error())
	}
}
