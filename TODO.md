# TODO (aka road to v1.0.0)

## Minimal External Dependencies

- [ ] Remove reliance on github.com/gomarkdown/markdown
- [ ] Remove reliance on YAML & TOML parsers
- [ ] Replace charmbracelet/log with hand-roller logger
- [ ] Add structured logging

## Theming

- [ ] base16 light
- [ ] base16 dark
- [ ] simple js toggle for data attribute

## Server

The server should...

- [ ] do what the [build](#build) command does, while also serving
      the web pages
- [ ] accept a toml config file

### Watcher

The watcher should...

- [x] watch for changes in the content directory
- [x] watch for changes in the templates directory
- [x] watch for changes in the static directory
- [ ] open a websocket when the browser is open
- [ ] trigger a reload in the browser when the above two directories
      have changes

## Build

The build command should...

- [ ] (v0) create a dist/build directory
- [ ] (v0) create html files for each markdown file
- [ ] (v0) copy files from the static directory to the dist directory
- [ ] (v1) create a directory with an index.html file for each markdown file

## Deploy

The deploy command should...

- [ ] for cloudflare support, call `wrangler` on the user's machine
- [ ] optionally rebuild the site

## Future

These are all version 1 onward

- [ ] define base16 to website color scheme matching
- [ ] define base24 to website color scheme matching
- [ ] base16 color scheme support
- [ ] base24 color scheme support
- [ ] light & dark themes

## Bugs

- [ ] 1/5/25: sigkill shouldn't print sww to stderr
