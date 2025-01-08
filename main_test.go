package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

func TestMain(t *testing.T) {
	root := libs.FindWDRoot()
	base_path := fmt.Sprintf("%v/example", root)

	t.Run("setContext updates the context with a config and logger", func(t *testing.T) {
		ctx := context.Background()
		cmd := rootCommand
		sb := strings.Builder{}
		cmd.Writer = &sb

		args := os.Args[0:1]
		args = append(args, "--file")
		args = append(args, fmt.Sprintf("%v/%v", base_path, "config.toml"))

		err := cmd.Run(ctx, args)

		if err != nil {
			t.Fatalf("unable to set flag to root command \n%v", err.Error())
		}

		ctx, err = setContext(ctx, cmd)

		if err != nil {
			t.Logf("unable to set context \n%v", err.Error())
			t.Fail()
		}

		logger = ctx.Value(config.LoggerKey).(*log.Logger)

		if logger == nil {
			t.Log("logger should be defined but it is not")
			t.Fail()
		}

		conf := ctx.Value(config.ConfKey).(*config.Config)
		if conf == nil {
			t.Log("conf should be defined but it is not")
			t.Fail()
		}

		output := sb.String()

		for _, c := range []string{"documango", "OPTIONS", "static site", "help"} {
			t.Run("output of no subcommand should have "+c, func(t *testing.T) {
				if !strings.Contains(output, c) {
					t.Logf("%v should contain %s but does not", output[1:10]+"..."+output[20:], c)
				}
			})
		}
	})
}
