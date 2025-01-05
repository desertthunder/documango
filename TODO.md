# TODO (aka road to v1.0.0)

## Minimal External Dependencies

- [ ] Remove reliance on github.com/gomarkdown/markdown
- [ ] Replace charmbracelet/log with hand-roller logger
- [ ] Add structured logging

## Theming

- [ ] base16 light
- [ ] base16 dark

## Server

The server should...

- [ ] do what the [build](#build) command does, while also serving
      the web pages

### Watcher

The watcher should...

- [ ] watch for changes in the content directory
- [ ] watch for changes in the templates directory
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
