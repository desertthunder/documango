//go:build e2e

// Package e2e drives documango's served pages in a headless Chromium.
//
// Install the browser once with `go run ./cmd/tools browsers`, then run
// `go test -tags e2e ./e2e`.
package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	"github.com/desertthunder/documango/internal/cli"
	"github.com/desertthunder/documango/internal/theme"
)

const (
	timeout      = 10 * time.Second
	installHint  = "run 'go run ./cmd/tools browsers' to install the Playwright driver and Chromium"
	eventsSuffix = "/_documango/events"
)

var (
	browser    playwright.Browser
	skipReason string
	expect     = playwright.NewPlaywrightAssertions(float64(timeout.Milliseconds()))
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	pw, err := playwright.Run(&playwright.RunOptions{Stdout: io.Discard, Stderr: os.Stderr})
	if err == nil {
		browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Timeout: playwright.Float(float64(timeout.Milliseconds()))})
		if err != nil {
			_ = pw.Stop()
		}
	}
	if err != nil {
		skipReason = fmt.Sprintf("browser unavailable (%v); %s", err, installHint)
		// CI must not pass by skipping everything.
		if os.Getenv("CI") != "" {
			fmt.Fprintln(os.Stderr, "e2e:", skipReason)
			return 1
		}
		return m.Run()
	}
	code := m.Run()
	_ = browser.Close()
	_ = pw.Stop()
	return code
}

// newPage opens a page in a fresh browser context, closed when t ends.
func newPage(t *testing.T, opts ...playwright.BrowserNewContextOptions) playwright.Page {
	t.Helper()
	if skipReason != "" {
		t.Skip(skipReason)
	}
	o := playwright.BrowserNewContextOptions{Viewport: &playwright.Size{Width: 1280, Height: 800}}
	if len(opts) > 0 {
		o = opts[0]
	}
	ctx, err := browser.NewContext(o)
	if err != nil {
		t.Fatalf("new browser context: %v", err)
	}
	t.Cleanup(func() { _ = ctx.Close() })
	ctx.SetDefaultTimeout(float64(timeout.Milliseconds()))
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	return page
}

// mobile is a phone-sized browser context.
var mobile = playwright.BrowserNewContextOptions{
	Viewport: &playwright.Size{Width: 375, Height: 740}, IsMobile: playwright.Bool(true), HasTouch: playwright.Bool(true),
}

// fixture is the docs folder every test serves.
var fixture = map[string]string{
	"index.md": `---
title: Acme Docs
---
Acme turns widgets into gadgets.

![Acme logo](logo.svg)

## Getting started

Read the [install guide](guide/install.md) first.

## Next steps

Tune Acme with the [configuration options](guide/configure.md).
`,
	"logo.svg": `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><circle cx="32" cy="32" r="30" fill="#4a90d9"/></svg>`,
	"guide/index.md": `---
title: Guide
order: 1
---
The guide walks through installing and configuring Acme.

## Overview

Start with installation.

## Audience

Operators who run Acme in production.
`,
	"guide/install.md": `---
title: Install
order: 1
---
Install Acme before your zeppelin takes off.

## Requirements

| System  | Minimum version | Notes                         |
| ------- | --------------- | ----------------------------- |
| Linux   | 5.10            | glibc or musl                 |
| macOS   | 13              | Apple silicon and Intel       |
| Windows | 11              | Run from PowerShell or cmd    |

### Disk space

Acme needs 200 MB of free space.

## Download

` + "```go" + `
package main

import "fmt"

func main() {
	fmt.Println("a deliberately long line of code that must scroll inside its block instead of widening the page on a phone")
}
` + "```" + `

> [!NOTE]
> Checksums are published with every release.

> [!WARNING]
> Old releases are not supported.

### Verify the download

` + "```sh" + `
sha256sum --check acme.sha256
` + "```" + `

## Troubleshooting

Reinstall when the binary fails to start.
`,
	"guide/configure.md": `---
title: Configure
order: 2
---
Configuration lives in acme.yaml.

## Options

Set ` + "`workers`" + ` to the number of CPU cores.

## Environment

Variables override the file.
`,
	"reference/cli.md": `---
title: CLI
order: 1
---
The acme command.

## Flags

Pass --help for every flag.

| Flag | Default | Description |
| --- | --- | --- |
| ` + "`--title`" + ` | home page title | Site title shown in the header and the browser tab |
| ` + "`--dark-theme`" + ` | ` + "`tomorrow-night`" + ` | Dark mode themes, comma-separated; the first is the default |
| ` + "`--light-theme`" + ` | ` + "`tomorrow`" + ` | Light mode themes, comma-separated; the first is the default |
| ` + "`--base-path`" + ` | ` + "`/`" + ` | URL path the site lives under |

## Exit codes

Zero means success.
`,
	"reference/api.md": `---
title: API
order: 2
---
The HTTP API.

## Endpoints

GET /status reports health.

## Errors

Errors are JSON objects.
`,
}

// served is a running documango serve.
type served struct {
	URL string // with a trailing slash
	Dir string // the docs folder, which tests may edit
}

// serve writes fixture to a temporary folder and runs `documango serve` on it
// with the built-in search, an isolated cache and a random port, plus args.
// The server stops when t ends.
func serve(t *testing.T, env []string, args ...string) *served {
	t.Helper()
	if skipReason != "" {
		t.Skip(skipReason)
	}
	dir := t.TempDir()
	for name, body := range fixture {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	env = append([]string{"DOCUMANGO_CACHE_DIR=" + t.TempDir(), "NO_COLOR=1"}, env...)
	argv := append([]string{"serve", dir, "--port", "0", "--search", "builtin"}, args...)
	exited := make(chan int, 1)
	go func() {
		code := cli.Execute(ctx, argv, cli.Env{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: pw, Environ: env})
		_ = pw.Close()
		exited <- code
	}()

	var mu sync.Mutex
	var log strings.Builder
	urls := make(chan string, 1)
	serving := regexp.MustCompile(`^Serving .* at (http://\S+)`)
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			mu.Lock()
			log.WriteString(sc.Text() + "\n")
			mu.Unlock()
			if m := serving.FindStringSubmatch(sc.Text()); m != nil {
				urls <- m[1]
			}
		}
	}()
	output := func() string {
		mu.Lock()
		defer mu.Unlock()
		return log.String()
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(timeout):
			t.Errorf("documango serve did not stop")
		}
		if t.Failed() {
			t.Logf("documango serve output:\n%s", output())
		}
	})

	select {
	case u := <-urls:
		return &served{URL: u, Dir: dir}
	case code := <-exited:
		t.Fatalf("documango serve exited with code %d:\n%s", code, output())
	case <-time.After(timeout):
		t.Fatalf("documango serve did not start:\n%s", output())
	}
	return nil
}

// write replaces a file in the served docs folder.
func (s *served) write(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.Dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// open navigates page to path under the site and fails t on an error or a
// non-200 response.
func (s *served) open(t *testing.T, page playwright.Page, path string) {
	t.Helper()
	resp, err := page.Goto(s.URL + strings.TrimPrefix(path, "/"))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if resp.Status() != 200 {
		t.Fatalf("open %s: status %d", path, resp.Status())
	}
}

// watchErrors records console errors, uncaught exceptions, failed requests
// and error responses on page. The live reload stream is excluded: the
// browser cancels it on every navigation.
func watchErrors(page playwright.Page) func() []string {
	var mu sync.Mutex
	var errs []string
	add := func(format string, args ...any) {
		mu.Lock()
		errs = append(errs, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	page.OnConsole(func(m playwright.ConsoleMessage) {
		if m.Type() == "error" {
			add("console: %s", m.Text())
		}
	})
	page.OnPageError(func(err error) { add("exception: %v", err) })
	page.OnRequestFailed(func(r playwright.Request) {
		if !strings.HasSuffix(r.URL(), eventsSuffix) {
			add("request failed: %s %v", r.URL(), r.Failure())
		}
	})
	page.OnResponse(func(r playwright.Response) {
		if r.Status() >= 400 {
			add("response %d: %s", r.Status(), r.URL())
		}
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), errs...)
	}
}

// background is the computed body background of a scheme's base00.
func background(t *testing.T, slug string) string {
	t.Helper()
	s, ok := theme.Lookup(slug)
	if !ok {
		t.Fatalf("unknown scheme %q", slug)
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(s.Palette[0], "#"), 16, 32)
	if err != nil {
		t.Fatalf("scheme %s base00 %q: %v", slug, s.Palette[0], err)
	}
	return fmt.Sprintf("rgb(%d, %d, %d)", n>>16&0xff, n>>8&0xff, n&0xff)
}

// check fails t when an assertion or action returned an error.
func check(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// screenshotDir returns the folder for review screenshots, creating it.
func screenshotDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("DOCUMANGO_E2E_SCREENSHOTS")
	if dir == "" {
		dir = "screenshots"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
