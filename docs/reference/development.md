---
title: Development
description: Build, test, and change documango itself.
order: 3
---

This page is for people working on documango's source code. You need Go 1.25
or later.

## Build and test

From the root of the repository:

```sh
go build ./cmd/documango
go test ./...
```

The tests don't use the network. They run a fake Pagefind program and fill
the theme cache with test schemes. A few end-to-end tests download
the real Pagefind release and theme catalog. They run only when
`DOCUMANGO_E2E` is set to `1`:

```sh
DOCUMANGO_E2E=1 go test ./internal/pagefind ./internal/theme
```

To preview these docs with your local changes:

```sh
go run ./cmd/documango docs
```

## Browser tests

The tests in `e2e/` start `documango serve` on a sample docs folder and drive
the pages in headless Chromium with
[playwright-go](https://github.com/mxschmitt/playwright-go). They cover
navigation, themes, the color scheme menu, the sidebar, search, the mobile
menu, live reload, and basic accessibility checks. They are behind the `e2e`
build tag, so `go test ./...` skips them.

Install the Playwright driver and Chromium once:

```sh
go run ./cmd/tools browsers
```

On Linux, add `-with-deps` to also install the system packages Chromium
needs. This runs the package manager with `sudo`.

Then run the tests:

```sh
go test -tags e2e ./e2e
```

Without the driver or Chromium, every test is skipped with a message that
names the install command. When the `CI` environment variable is set, a
missing browser fails the run instead.

The tests never download anything. They use the built-in search and an empty
theme cache. The Pagefind search test runs only when a Pagefind binary is
available, either named by `DOCUMANGO_PAGEFIND` or already in documango's
default cache folder. Otherwise it is skipped.

The tests also save full-page screenshots of the desktop page in light and
dark mode, the mobile page with the menu closed and open, and open search
results. They go to `e2e/screenshots/`, or to the folder named by
`DOCUMANGO_E2E_SCREENSHOTS`. Nothing compares them; they are for reviewing
visual changes by eye.

## Project layout

documango is a single Go module. The command-line entry point is small, and
each step of turning Markdown into a site lives in its own package under
`internal/`.

| Path                   | Purpose |
| ---------------------- | ------- |
| `cmd/documango`        | The `documango` binary. |
| `cmd/tools`            | Development helpers: the built-in scheme updater and the browser installer for the browser tests. |
| `e2e`                  | Browser tests, built only with the `e2e` tag. |
| `internal/cli`         | Commands, flags, and output for the binary. |
| `internal/frontmatter` | Splits YAML front matter from a Markdown file. |
| `internal/markdown`    | Renders Markdown to HTML with goldmark, and collects the title, headings, and search text. |
| `internal/site`        | Walks the docs directory, assigns URLs, rewrites relative links, and builds the navigation tree. |
| `internal/render`      | Applies the HTML template and writes pages, the 404 page, the stylesheet, the script, and the built-in search index. Matches link and callout colors to each scheme by hue. |
| `internal/pagefind`    | Finds or downloads the Pagefind binary and runs it over the rendered pages. |
| `internal/theme`       | Loads base16 schemes, embeds the built-in ones, and generates the code highlighting CSS. `catalog.go` downloads and caches the full tinted-theming catalog. |
| `internal/server`      | Serves a build from memory, watches for changes, and sends live reload events. |

The page template, stylesheets, and browser script are in
`internal/render/templates/` and `internal/render/assets/`, and are embedded
in the binary.

## How live reload works

The development server never writes the site to disk. It keeps every output
file in memory and replaces the whole set after each rebuild:

1. At startup, the server builds the site once. If this build fails, the
   command exits.
2. A file watcher watches the docs directory and every folder inside it,
   including folders created later. It ignores hidden paths.
3. When files change, the server waits until 100 milliseconds pass with no
   further changes, so saving several files at once causes one rebuild.
4. The server rebuilds the whole site in memory and, unless the site uses the
   built-in search, runs Pagefind over the new pages. On success, it swaps in
   the new files. On failure, it keeps the previous files.
5. Each open page holds a Server-Sent Events connection to
   `/_documango/events` under the base path. After a rebuild, the server sends
   a `reload` event and the page reloads, or an `error` event with the message
   that the page shows in a banner.

Static builds don't include the live reload connection.

## Update the built-in themes

The 26 built-in schemes in `internal/theme/schemes/` are copied from the
[tinted-theming schemes](https://github.com/tinted-theming/schemes) project,
which is distributed under the MIT License. Its license is kept next to the
schemes in `internal/theme/schemes/LICENSE`. The list of built-in schemes is
the `curated` variable in `internal/theme/theme.go`. Every other scheme is
downloaded at run time.

To download the built-in schemes again, run either command from the
repository root:

```sh
go run ./cmd/tools schemes
go generate ./internal/theme
```

The command downloads the schemes repository at the `spec-0.11` git ref and
replaces the YAML files and license in `internal/theme/schemes/` with the
schemes from the `curated` list. It fails if any of them is missing upstream.
Pass `-ref` to choose a different git ref, or `-out` to write somewhere else:

```sh
go run ./cmd/tools schemes -ref main -out /tmp/schemes
```

To add or remove a built-in scheme, edit the `curated` list and run the
command again.
