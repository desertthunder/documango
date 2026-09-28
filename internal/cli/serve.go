package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	"github.com/desertthunder/documango/internal/server"
)

const serveExample = `  # Preview the docs in the current folder at http://127.0.0.1:3000
  documango serve

  # Preview the docs folder on port 4000
  documango serve docs --port 4000

  # Share the preview with other devices on your network
  documango serve docs --host 0.0.0.0`

// serveOptions are the flags of serve and the root command.
type serveOptions struct {
	site siteOptions
	port int
	host string
}

func (o *serveOptions) addFlags(fs *pflag.FlagSet) {
	o.site.addFlags(fs)
	fs.IntVarP(&o.port, "port", "p", 3000, "port to listen on")
	fs.StringVar(&o.host, "host", "127.0.0.1", "address to listen on; use 0.0.0.0 to allow other devices")
}

func (a *app) newServeCmd() *cobra.Command {
	opts := &serveOptions{}
	cmd := &cobra.Command{
		Use:   "serve [dir]",
		Short: "Preview the docs with live reload",
		Long: `Serve the docs in dir (the current folder by default) on a local web server.
Pages rebuild and your browser reloads whenever a file changes.
Press Ctrl+C to stop.`,
		Example: serveExample,
		Args:    usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.serve(cmd.Context(), dirArg(args), opts, nil)
		},
	}
	opts.addFlags(cmd.Flags())
	return cmd
}

// serve builds the docs in dir and serves them with live reload until ctx is
// done or the process is interrupted. root is set when the root command runs
// serve, so a mistyped command name can be suggested.
func (a *app) serve(ctx context.Context, dir string, o *serveOptions, root *cobra.Command) error {
	if err := checkDir(dir, root); err != nil {
		return err
	}
	b, err := o.site.builder(dir, a.version)
	if err != nil {
		return err
	}
	b.render.LiveReload = server.EventsURLFor(b.render.BasePath)

	// Build once here so errors surface as they are and the title is known;
	// the server reuses this result as its initial build.
	s, files, err := b.build()
	if err != nil {
		return err
	}
	srv, err := server.New(func() (server.Files, error) {
		if files != nil {
			initial := files
			files = nil
			return initial, nil
		}
		_, f, err := b.build()
		return f, err
	}, server.Options{Dir: dir, BasePath: b.render.BasePath, Logger: a.logger()})
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(o.host, strconv.Itoa(o.port))
	ln, err := net.Listen("tcp", addr)
	if errors.Is(err, syscall.EADDRINUSE) {
		return withHint(fmt.Errorf("port %d is already in use", o.port), "use --port to pick another port")
	}
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !a.quiet {
		host := o.host
		if host == "" {
			host = "localhost"
		}
		url := "http://" + net.JoinHostPort(host, strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)) + b.render.BasePath
		fmt.Fprintf(a.stderr, "Serving %s at %s\n", boldStyle.Render(s.Title), accentStyle.Render(url))
		fmt.Fprintln(a.stderr, dimStyle.Render("Press Ctrl+C to stop"))
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return srv.Watch(ctx) })
	g.Go(func() error { return server.Serve(ctx, ln, srv) })
	return g.Wait()
}
