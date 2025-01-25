package build

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/internal/config"
)

func TestBuildCommand(t *testing.T) {
	wg := sync.WaitGroup{}
	t.Run("Build", func(t *testing.T) {
		sb := strings.Builder{}
		BuildLogger = log.Default()
		BuildLogger.SetOutput(&sb)
		_, _, c := setupConf()

		mutateConf(c, "build")

		ctx := context.TODO()
		ctx = context.WithValue(ctx, config.LoggerKey, BuildLogger)
		ctx, cancelFunc := context.WithCancel(ctx)

		wg.Add(1)
		var err error
		go func() {
			<-ctx.Done()
			defer wg.Done()
			err = BuildCommand.Run(ctx, []string{})

			if err != nil {
				t.Log(sb.String())
				t.Errorf("execution failed %v", err.Error())
			}
			cancelFunc()
		}()

	})
}
