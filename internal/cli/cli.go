// Package cli implements the documango command line: serving a directory of
// Markdown with live reload, building it into a static site, and listing
// themes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"
)

// Env is the process environment a command runs in.
type Env struct {
	// Stdin is read by interactive commands, which need it to be a terminal.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// Environ holds "KEY=value" pairs, as returned by os.Environ.
	Environ []string
	// Version is the release version, usually set at link time. Empty falls
	// back to the module version from the build info, else "dev".
	Version string
}

// Execute runs the command line args (without the program name) and returns
// the process exit code: 0 on success, 1 for runtime errors, and 2 for usage
// errors.
func Execute(ctx context.Context, args []string, env Env) int {
	a := &app{env: env, version: versionString(env.Version)}
	root := a.newRootCmd()
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}
	// Flag parsing may have stopped before --no-color was read.
	a.noColor = a.noColor || slices.Contains(args, "--no-color")
	stderr, _ := a.writer(env.Stderr)

	if errors.Is(err, errCancelled) {
		if !a.quiet {
			fmt.Fprintln(stderr, "Cancelled")
		}
		return 1
	}
	code, hint := 1, ""
	var he *hintError
	if errors.As(err, &he) {
		hint = he.hint
	}
	var ue *usageError
	if errors.As(err, &ue) {
		code = 2
		if cmd == nil {
			cmd = root
		}
		hint = fmt.Sprintf("Run '%s --help' for usage.", cmd.CommandPath())
	}
	fmt.Fprintln(stderr, errorStyle.Render("Error:"), err)
	if hint != "" {
		fmt.Fprintln(stderr, hintStyle.Render("Hint:"), hint)
	}
	return code
}

// versionString returns v, else the main module version, else "dev".
func versionString(v string) string {
	if v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// errCancelled reports that the user quit an interactive command.
var errCancelled = errors.New("cancelled")

// usageError marks a mistake in how a command was invoked (exit code 2).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// hintError adds a suggestion printed after the error message.
type hintError struct {
	err  error
	hint string
}

func (e *hintError) Error() string { return e.err.Error() }
func (e *hintError) Unwrap() error { return e.err }

func withHint(err error, hint string) error { return &hintError{err, hint} }

// usageArgs wraps a cobra argument validator so its errors are usage errors.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return &usageError{err}
		}
		return nil
	}
}

// app holds the state shared by all commands of one invocation.
type app struct {
	env     Env
	version string

	quiet, verbose, noColor bool

	// Set by setup once flags are parsed.
	stdout, stderr *colorprofile.Writer
	stdoutTTY      bool
}

// setup validates the global flags and prepares the output streams.
func (a *app) setup(*cobra.Command, []string) error {
	if a.quiet && a.verbose {
		return &usageError{errors.New("--quiet and --verbose cannot be used together")}
	}
	a.stdout, a.stdoutTTY = a.writer(a.env.Stdout)
	a.stderr, _ = a.writer(a.env.Stderr)
	return nil
}

// writer wraps w so styled output is downsampled to what w supports, and
// reports whether w is an interactive terminal. Colour is removed entirely
// when w is not a terminal, TERM is dumb, NO_COLOR is set, or --no-color is
// given.
func (a *app) writer(w io.Writer) (*colorprofile.Writer, bool) {
	p := colorprofile.Detect(w, a.env.Environ)
	tty := p > colorprofile.NoTTY
	if a.noColor || a.getenv("NO_COLOR") != "" {
		p = colorprofile.NoTTY
	}
	return &colorprofile.Writer{Forward: w, Profile: p}, tty
}

func (a *app) getenv(key string) string {
	for _, kv := range slices.Backward(a.env.Environ) {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// logger returns a logger writing styled lines to stderr at the level the
// global flags select.
func (a *app) logger() *slog.Logger {
	level := slog.LevelInfo
	switch {
	case a.quiet:
		level = slog.LevelWarn
	case a.verbose:
		level = slog.LevelDebug
	}
	return slog.New(&logHandler{w: a.stderr, level: level, mu: &sync.Mutex{}})
}

var (
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red)
	hintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Green)
	accentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	boldStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
)
