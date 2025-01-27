package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
	"github.com/desertthunder/documango/internal/utils"
)

func TestMain(t *testing.T) {
	root := utils.FindWDRoot()
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
			t.Errorf("unable to set context \n%v", err.Error())
		}

		logger = ctx.Value(config.LoggerKey).(*log.Logger)

		if logger == nil {
			t.Error("logger should be defined but it is not")
		}

		conf := ctx.Value(config.ConfKey).(*config.Config)
		if conf == nil {
			t.Error("conf should be defined but it is not")
		}

		output := sb.String()

		for _, c := range []string{"documango", "OPTIONS", "static site", "help"} {
			t.Run("output of no subcommand should have "+c, func(t *testing.T) {
				if !strings.Contains(output, c) {
					t.Errorf("%v should contain %s but does not", output[1:10]+"..."+output[20:], c)
				}
			})
		}
	})
}
