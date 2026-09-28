---
title: documango
description: Turn a directory of Markdown files into a documentation website.
---

documango turns a directory of Markdown files into a documentation website.
Run it against a folder of `.md` files and you get a site with a sidebar built
from your folder structure, a table of contents on each page, search, and a
light and dark theme. The files and their front matter describe the site. An
optional [config file](reference/configuration.md) sets the title, page
metadata, themes, and header links.

documango has two modes:

- A development server that rebuilds the site and reloads the browser when you
  save a file.
- A static build that writes plain HTML, CSS, and JavaScript files you can
  host anywhere.

This site is built with documango from the `docs/` directory of the
[documango repository](https://github.com/desertthunder/documango).

## Install

documango is a single binary written in Go. With Go 1.25 or later installed,
run:

```sh
go install github.com/desertthunder/documango/cmd/documango@latest
```

To build from a clone of the repository instead:

```sh
go build ./cmd/documango
```

## Quick start

Create a directory with a Markdown file, then start the development server:

```sh
mkdir my-docs
printf '# My project\n\nHello.\n' > my-docs/index.md
documango my-docs
```

Open <http://127.0.0.1:3000/> in your browser. Edit `my-docs/index.md` and
the page reloads with your change.

To start from a sample site instead, run `documango init my-docs`. It creates
a home page, a short guide on writing pages, and a `documango.toml` config
file.

When you're ready to publish, build the static site:

```sh
documango build my-docs
```

The site is written to `_site/`. Upload that directory to any static host.

## Next steps

- [Guide](guide/index.md) explains how files become pages, which Markdown
  features work, how to pick a theme, and how to deploy.
- [Reference](reference/index.md) lists every command, flag, and front matter
  field.
