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

To preview these docs with your local changes:

```sh
go run ./cmd/documango docs
```

## Project layout

documango is a single Go module. The command-line entry point is small, and
each step of turning Markdown into a site lives in its own package under
`internal/`.

| Path                   | Purpose |
| ---------------------- | ------- |
| `cmd/documango`        | The `documango` binary. |
| `cmd/tools`            | Development helpers, such as the scheme updater. |
| `internal/cli`         | Commands, flags, and output for the binary. |
| `internal/frontmatter` | Splits YAML front matter from a Markdown file. |
| `internal/markdown`    | Renders Markdown to HTML with goldmark, and collects the title, headings, and search text. |
| `internal/site`        | Walks the docs directory, assigns URLs, rewrites relative links, and builds the navigation tree. |
| `internal/render`      | Applies the HTML template and writes pages, the 404 page, the stylesheet, the script, and the search index. |
| `internal/theme`       | Loads base16 schemes, embeds the built-in ones, and generates the code highlighting CSS. |
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
   including folders created later. It ignores hidden paths and the build
   output folder.
3. When files change, the server waits until 100 milliseconds pass with no
   further changes, so saving several files at once causes one rebuild.
4. The server rebuilds the whole site in memory. On success, it swaps in the
   new files. On failure, it keeps the previous files.
5. Each open page holds a Server-Sent Events connection to
   `/_documango/events` under the base path. After a rebuild, the server sends
   a `reload` event and the page reloads, or an `error` event with the message
   that the page shows in a banner.

Static builds don't include the live reload connection.

## Update the built-in themes

The built-in schemes in `internal/theme/schemes/` are copied from the
[tinted-theming schemes](https://github.com/tinted-theming/schemes) project,
which is distributed under the MIT License. Its license is kept next to the
schemes in `internal/theme/schemes/LICENSE`.

To download them again, run either command from the repository root:

```sh
go run ./cmd/tools schemes
go generate ./internal/theme
```

The command downloads the schemes repository at the `spec-0.11` git ref and
replaces the YAML files and license in `internal/theme/schemes/`. Pass `-ref`
to choose a different git ref, or `-out` to write somewhere else:

```sh
go run ./cmd/tools schemes -ref main -out /tmp/schemes
```
