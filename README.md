# documango

documango turns a directory of Markdown files into a documentation website.
It runs a development server that reloads the browser when you save a file,
and builds a static site you can host anywhere. The sidebar comes from your
folder structure, and every site has search and a light and dark theme. There
is no configuration file.

## Install

With Go 1.25 or later:

```sh
go install github.com/desertthunder/documango/cmd/documango@latest
```

Or build from a clone of this repository:

```sh
go build ./cmd/documango
```

## Quick start

```sh
documango docs                 # serve docs/ at http://127.0.0.1:3000 with live reload
documango build docs           # write the static site to _site/
documango themes               # list the built-in color schemes
```

## Documentation

The documentation is in [`docs/`](docs/index.md) and is itself a documango
site. To read it locally, run `documango docs` from the repository root.

## Credits

The built-in color schemes come from
[tinted-theming/schemes](https://github.com/tinted-theming/schemes) under the
MIT License. See [`internal/theme/schemes/LICENSE`](internal/theme/schemes/LICENSE).
