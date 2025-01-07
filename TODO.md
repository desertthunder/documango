# TODO (aka road to v1.0.0)

## Minimal External Dependencies

These are for version 1 and onwards.

- Remove reliance on github.com/gomarkdown/markdown
- Remove reliance on YAML & TOML parsers
- Replace charmbracelet/log with hand-roller logger

## CLI

- [ ] Add structured logging
      - [ ] Config key:value
      - [ ] Enable/Disable flag
      - [ ] Default sink?
- [ ] Configuration vs CLI argument priority

## Theming

- [x] base16 light
- [x] base16 dark
- [x] simple js toggle for data attribute

## Server

The server should...

- [x] do what the [build](#build) command does, while also serving
      the web pages
- [x] use a toml config file
- [ ] build a list of available links for the top level navigation

### Watcher

The watcher should...

- [x] watch for changes in the content directory
- [x] watch for changes in the templates directory
- [x] watch for changes in the static directory
- [ ] open a websocket when the browser is open
- [ ] trigger a reload in the browser when the above three directories
      have changes

## Build

The build command should...

- [x] (v0) create a dist/build directory
- [x] (v0) create html files for each markdown file
- [x] (v0) copy files from the static directory to the dist directory
- [ ] (v0) rename previous build dir to _{name} or put in temp dir
- [ ] (v0) recursively sift through directories and nested directories
- (v1) build categories and tags for structured content
- (v1) create a directory with an index.html file for each markdown file

## Deploy

The deploy command should...

- [ ] call `wrangler` for cloudflare support on the user's machine
- [ ] optionally rebuild the site

## Future

These are all version 1 onward

- define base16 to website color scheme matching
- define base24 to website color scheme matching
- base16 color scheme support
- base24 color scheme support
- cache themes
- light & dark themes
- netlify support via TOML
- gh-pages support
- HTMX CMS

## Bugs

- [x] 1/5/25: sigkill shouldn't print sww to stderr

## Parking Lot

- Pages list?
- Which theme files should be embedded in the binary?
