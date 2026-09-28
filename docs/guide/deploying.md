---
title: Deploying
description: Build a static documango site and publish it, including under a subpath.
order: 4
---

`documango build` writes your site as static files: HTML pages, a stylesheet,
a script, a search index, and your images and other files. Any static web
host can serve them, with no server-side code.

## Build the site

Run `documango build` with your docs directory:

```sh
documango build docs
```

documango writes the site to `_site/`. Choose a different folder with
`--out`:

```sh
documango build docs --out public
```

The output folder contains:

| Path                         | Contents                                                     |
| ---------------------------- | ------------------------------------------------------------ |
| `index.html`                 | The home page                                                |
| `guide/deploying/index.html` | One `index.html` per page, at its URL                        |
| `404.html`                   | A "Page not found" page                                      |
| `_documango/`                | The stylesheet, script, built-in search index, and file list |
| `pagefind/`                  | The Pagefind search files, when the site uses Pagefind       |
| Everything else              | Images and other files from your docs                        |

To search with Pagefind, documango runs it during the build. The first build
may download it. See [search](writing-pages.md#search) for details and for
`--search builtin`, which skips Pagefind.

documango records the files it writes in `_documango/manifest.json`. When you
build into the same folder again, it deletes the files from the previous
build that it no longer writes, such as the old page of a renamed Markdown
file. Other files in the output folder stay in place. To start from an empty
folder, add `--clean`:

```sh
documango build docs --out public --clean
```

> [!WARNING]
> `--clean` deletes everything in the output folder. To protect other files,
> documango refuses to clean a folder unless it is empty, doesn't exist yet, or
> contains a previous documango build (a `_documango/` folder).

## Base path

A site served from a subpath, such as `https://example.com/docs/`, needs a
base path so that its links, stylesheet, and search results point to the
right place. Set it with `--base-path`:

```sh
documango build docs --base-path /docs/
```

The leading and trailing slashes are optional: `docs`, `/docs`, and `/docs/`
give the same result.

documango adds the base path to the links it generates and to relative links
in your Markdown. It doesn't change links that start with `/`, so a link
written as `[Install](/guide/install/)` ignores the base path. Use relative
links to `.md` files instead. See [links and images](writing-pages.md#links-and-images).

To check a subpath site before you publish it, pass the same base path to the
development server:

```sh
documango docs --base-path /docs/
```

The site is then served at `http://127.0.0.1:3000/docs/`.

### Set the site address in the config file

Instead of passing `--base-path` on every run, set `url` in the
[config file](../reference/configuration.md) to the address you publish the
site at:

```toml
url = "https://example.com/docs/"
```

documango takes the base path from the path of `url`, here `/docs/`, and
adds canonical links and link preview URLs to every page. If your host serves
the site under a different path than the one in `url`, set `base_path` too.
A `--base-path` flag overrides both.

## The 404 page

`404.html` at the top of the output folder is a "Page not found" page with
the site's header and sidebar. Many hosts, including GitHub Pages, Netlify,
and Cloudflare Pages, show it automatically for missing URLs. On other hosts,
configure the 404 page to be `404.html`.

## GitHub Pages

A GitHub Pages project site is served from
`https://<user>.github.io/<repository>/`, so it needs the repository name as
its base path. A user or organization site at `https://<user>.github.io/`
doesn't need one.

The following workflow is an example. It builds the `docs/` directory on every
push to `main` and publishes it with GitHub Pages. Save it as
`.github/workflows/docs.yml`, then set **Settings > Pages > Source** to
**GitHub Actions** in your repository.

```yaml
name: Docs

on:
  push:
    branches: [main]

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: true

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/desertthunder/documango/cmd/documango@latest
      - run: documango build docs --out _site --base-path "/${{ github.event.repository.name }}/"
      - uses: actions/upload-pages-artifact@v3
        with:
          path: _site

  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - id: deployment
        uses: actions/deploy-pages@v4
```

For a user or organization site, or a site on a custom domain, remove the
`--base-path` flag.
