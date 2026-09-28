package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"
)

const (
	sitePage    = "/guide/markdown/"
	siteHost    = "docs.example.com"
	freezeWidth = 1480
	freezeHint  = "install it with 'brew install charmbracelet/tap/freeze' or 'go install github.com/charmbracelet/freeze@latest'"
)

// shot is one image written by the screenshots subcommand. CLI shots set
// command; site shots set path.
type shot struct {
	name    string
	command string

	path          string
	dark          bool
	width, height int
	mobile        bool
	search        string
}

var shots = []shot{
	{name: "cli-help", command: "documango --help"},
	{name: "cli-build", command: "documango build docs --search builtin"},
	{name: "cli-themes", command: "documango themes --offline --variant dark"},
	{name: "site-dark", path: sitePage, dark: true, width: 1280, height: 800},
	{name: "site-light", path: sitePage, width: 1280, height: 800},
	{name: "site-mobile", path: sitePage, dark: true, width: 390, height: 844, mobile: true},
	{name: "site-search", path: sitePage, dark: true, width: 1280, height: 800, search: "theme"},
}

type screenshotOptions struct {
	out, docs string
	shots     []shot
}

func runScreenshots(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, ok := parseScreenshots(args, stderr)
	if !ok {
		return 2
	}
	if err := takeScreenshots(ctx, opts, stdout); err != nil {
		fmt.Fprintf(stderr, "screenshots: %v\n", err)
		return 1
	}
	return 0
}

func parseScreenshots(args []string, stderr io.Writer) (screenshotOptions, bool) {
	fs := flag.NewFlagSet("screenshots", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", ".github/assets", "directory to write PNG files into")
	docs := fs.String("docs", "docs", "docs folder to build and serve")
	only := fs.String("only", "", "comma-separated shot names to take (default all)")
	if !parse(fs, args, stderr) {
		return screenshotOptions{}, false
	}
	selected, err := selectShots(*only)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n%s", err, usage)
		return screenshotOptions{}, false
	}
	return screenshotOptions{out: *out, docs: *docs, shots: selected}, true
}

// selectShots returns the shots named in the comma-separated list only, in
// their usual order, or every shot when only is empty.
func selectShots(only string) ([]shot, error) {
	if only == "" {
		return shots, nil
	}
	names := strings.Split(only, ",")
	for _, n := range names {
		if !slices.ContainsFunc(shots, func(s shot) bool { return s.name == n }) {
			return nil, fmt.Errorf("unknown shot %q", n)
		}
	}
	var picked []shot
	for _, s := range shots {
		if slices.Contains(names, s.name) {
			picked = append(picked, s)
		}
	}
	return picked, nil
}

func takeScreenshots(ctx context.Context, opts screenshotOptions, stdout io.Writer) error {
	var cli, site []shot
	for _, s := range opts.shots {
		if s.command != "" {
			cli = append(cli, s)
		} else {
			site = append(site, s)
		}
	}
	if len(cli) > 0 {
		if _, err := exec.LookPath("freeze"); err != nil {
			return fmt.Errorf("freeze not found on PATH; %s", freezeHint)
		}
	}
	docs, err := filepath.Abs(opts.docs)
	if err != nil {
		return fmt.Errorf("resolve docs folder: %w", err)
	}
	if err := os.MkdirAll(opts.out, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", opts.out, err)
	}

	tmp, err := os.MkdirTemp("", "documango-screenshots-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "bin")
	build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, "documango"), "./cmd/documango")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build documango: %w", err)
	}
	env := shotEnv(os.Environ(), bin, filepath.Join(tmp, "cache"))

	pw, err := playwright.Run(&playwright.RunOptions{Stdout: io.Discard, Stderr: os.Stderr})
	if err != nil {
		return fmt.Errorf("start playwright (run 'go run ./cmd/tools browsers' first): %w", err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return fmt.Errorf("launch chromium (run 'go run ./cmd/tools browsers' first): %w", err)
	}
	defer browser.Close()

	write := func(s shot, png []byte) error {
		path := filepath.Join(opts.out, s.name+".png")
		if err := os.WriteFile(path, png, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Fprintln(stdout, path)
		return nil
	}

	if len(cli) > 0 {
		work := filepath.Join(tmp, "work")
		if err := os.MkdirAll(work, 0o755); err != nil {
			return fmt.Errorf("create work dir: %w", err)
		}
		if err := os.Symlink(docs, filepath.Join(work, "docs")); err != nil {
			return fmt.Errorf("link docs folder: %w", err)
		}
		for _, s := range cli {
			png, err := shootCLI(ctx, browser, s, work, env)
			if err != nil {
				return fmt.Errorf("%s: %w", s.name, err)
			}
			if err := write(s, png); err != nil {
				return err
			}
		}
	}

	if len(site) > 0 {
		base, stop, err := startServer(ctx, filepath.Join(bin, "documango"), docs, env)
		if err != nil {
			return err
		}
		defer stop()
		for _, s := range site {
			png, err := shootSite(browser, s, base)
			if err != nil {
				return fmt.Errorf("%s: %w", s.name, err)
			}
			if err := write(s, png); err != nil {
				return err
			}
		}
	}
	return nil
}

// shotEnv returns environ with bin first on PATH, colour forced on and the
// documango cache isolated in cache.
func shotEnv(environ []string, bin, cache string) []string {
	env := slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return slices.Contains([]string{"PATH", "NO_COLOR", "TERM", "COLORTERM", "CLICOLOR_FORCE", "DOCUMANGO_CACHE_DIR"}, k)
	})
	return append(env,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CLICOLOR_FORCE=1",
		"DOCUMANGO_CACHE_DIR="+cache,
	)
}

// shotScript returns a shell script that prints a prompt with command and
// then runs it.
func shotScript(command string) string {
	return fmt.Sprintf("printf '\\033[90m$\\033[0m %%s\\n' '%s'\n%s\n", command, command)
}

// freezeArgs returns the freeze arguments that render script's output to an
// SVG file at out.
func freezeArgs(script, out string) []string {
	return []string{
		"--execute", "sh " + script,
		"--output", out,
		"--window",
		"--background", "#1d1f21",
		"--padding", "20,28",
		"--margin", "0",
		"--border.radius", "0",
		"--font.size", "14",
		"--line-height", "1.4",
		"--width", fmt.Sprint(freezeWidth),
	}
}

func shootCLI(ctx context.Context, browser playwright.Browser, s shot, work string, env []string) ([]byte, error) {
	script := filepath.Join(work, s.name+".sh")
	if err := os.WriteFile(script, []byte(shotScript(s.command)), 0o644); err != nil {
		return nil, fmt.Errorf("write script: %w", err)
	}
	svg := filepath.Join(work, s.name+".svg")
	cmd := exec.CommandContext(ctx, "freeze", freezeArgs(filepath.Base(script), svg)...)
	cmd.Dir = work
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("freeze: %w\n%s", err, out)
	}
	data, err := os.ReadFile(svg)
	if err != nil {
		return nil, fmt.Errorf("read freeze output: %w", err)
	}
	page, err := frameHTML(frame{Kind: "terminal", Image: dataURL("image/svg+xml", data), Width: freezeWidth, Dark: true})
	if err != nil {
		return nil, err
	}
	return capture(browser, page, true)
}

var servingLine = regexp.MustCompile(`Serving .* at (http://\S+)`)

// startServer runs documango serve on a free port and returns the site's URL
// and a function that stops the server.
func startServer(ctx context.Context, bin, docs string, env []string) (string, func(), error) {
	cmd := exec.CommandContext(ctx, bin, "serve", docs, "--port", "0", "--search", "builtin", "--no-color")
	cmd.Env = env
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("serve: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("serve: %w", err)
	}
	stop := func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}

	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := servingLine.FindStringSubmatch(sc.Text()); m != nil {
				found <- m[1]
				break
			}
		}
		_, _ = io.Copy(io.Discard, stderr)
		close(found)
	}()
	select {
	case url, ok := <-found:
		if ok {
			return strings.TrimSuffix(url, "/"), stop, nil
		}
		stop()
		return "", nil, errors.New("serve exited before printing its address")
	case <-time.After(30 * time.Second):
		stop()
		return "", nil, errors.New("serve did not print its address within 30s")
	}
}

// freezeMotion stops transitions and animations so shots are repeatable.
const freezeMotion = `*, *::before, *::after { transition: none !important; animation: none !important; caret-color: transparent !important; }`

func shootSite(browser playwright.Browser, s shot, base string) ([]byte, error) {
	scheme := playwright.ColorSchemeLight
	if s.dark {
		scheme = playwright.ColorSchemeDark
	}
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: s.width, Height: s.height},
		DeviceScaleFactor: playwright.Float(2),
		ColorScheme:       scheme,
		IsMobile:          playwright.Bool(s.mobile),
		HasTouch:          playwright.Bool(s.mobile),
	})
	if err != nil {
		return nil, fmt.Errorf("new browser context: %w", err)
	}
	defer ctx.Close()
	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("new page: %w", err)
	}
	if _, err := page.Goto(base+s.path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle}); err != nil {
		return nil, fmt.Errorf("open %s: %w", s.path, err)
	}
	if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{Content: playwright.String(freezeMotion)}); err != nil {
		return nil, fmt.Errorf("disable animations: %w", err)
	}
	if s.search != "" {
		if err := page.Keyboard().Press("/"); err != nil {
			return nil, fmt.Errorf("focus search: %w", err)
		}
		if err := page.Keyboard().Type(s.search); err != nil {
			return nil, fmt.Errorf("type query: %w", err)
		}
		results := page.GetByRole("listbox", playwright.PageGetByRoleOptions{Name: "Search results"}).GetByRole("option")
		if err := results.Nth(1).WaitFor(); err != nil {
			return nil, fmt.Errorf("wait for search results: %w", err)
		}
	}
	if _, err := page.Evaluate("document.fonts.ready.then(() => true)"); err != nil {
		return nil, fmt.Errorf("wait for fonts: %w", err)
	}
	raw, err := page.Screenshot(playwright.PageScreenshotOptions{
		Animations: playwright.ScreenshotAnimationsDisabled,
		Caret:      playwright.ScreenshotCaretHide,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	kind := "browser"
	if s.mobile {
		kind = "phone"
	}
	html, err := frameHTML(frame{Kind: kind, Image: dataURL("image/png", raw), Width: s.width, Dark: s.dark, URL: siteHost + s.path})
	if err != nil {
		return nil, err
	}
	return capture(browser, html, s.dark)
}

// capture renders html at twice the CSS resolution and returns a PNG of its
// #shot element with a transparent background.
func capture(browser playwright.Browser, html string, dark bool) ([]byte, error) {
	scheme := playwright.ColorSchemeLight
	if dark {
		scheme = playwright.ColorSchemeDark
	}
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: 1600, Height: 2400},
		DeviceScaleFactor: playwright.Float(2),
		ColorScheme:       scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("new browser context: %w", err)
	}
	defer ctx.Close()
	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("new page: %w", err)
	}
	if err := page.SetContent(html, playwright.PageSetContentOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		return nil, fmt.Errorf("load frame: %w", err)
	}
	if _, err := page.Evaluate("document.fonts.ready.then(() => true)"); err != nil {
		return nil, fmt.Errorf("wait for fonts: %w", err)
	}
	png, err := page.Locator("#shot").Screenshot(playwright.LocatorScreenshotOptions{
		OmitBackground: playwright.Bool(true),
		Animations:     playwright.ScreenshotAnimationsDisabled,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot frame: %w", err)
	}
	return png, nil
}

func dataURL(mime string, data []byte) template.URL {
	return template.URL("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data))
}

// frame describes the window drawn around a capture. Kind is terminal,
// browser or phone; Width is the capture's width in CSS pixels.
type frame struct {
	Kind  string
	Image template.URL
	Width int
	Dark  bool
	URL   string
}

//go:embed frame.html
var frameSource string

var frameTemplate = template.Must(template.New("frame").Parse(frameSource))

func frameHTML(f frame) (string, error) {
	var buf bytes.Buffer
	if err := frameTemplate.Execute(&buf, f); err != nil {
		return "", fmt.Errorf("render frame: %w", err)
	}
	return buf.String(), nil
}
