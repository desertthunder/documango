---
title: Configuration
description: The documango.toml and documango.yaml config file, its keys, and how it combines with flags.
order: 2
---

A config file sets the site title, page metadata, themes, fonts, header
links, and footer.
It's optional: without one, documango uses its defaults and the
[site flags](cli.md#site-flags).

## Where documango looks

documango reads `documango.toml`, `documango.yaml`, or `documango.yml` from
the top of the docs directory. If more than one of them exists, documango
stops with an error. Keep one.

To use a file somewhere else, pass `--config`:

```sh
documango build docs --config site/documango.toml
```

The file extension picks the format: `.toml`, `.yaml`, or `.yml`. If the file
given to `--config` doesn't exist, documango stops with an error.

The config file is never copied into the built site.

While `documango serve` runs, saving the config file rebuilds the site with
the new settings. If the file has an error, the page shows it in the "Build
failed" banner and the server keeps the last successful build. Changing the
base path is the one exception: restart the server to use a new base path.
The server only watches the docs directory, so edits to a `--config` file
outside it take effect on the next change inside the docs directory or on a
restart.

## Example

```toml
title = "Acme Docs"
description = "Docs for Acme."
url = "https://acme.dev/docs/"
author = "Acme Inc."
language = "en"
favicon = "favicon.svg"
logo = "logo.svg"
footer = """
Copyright Acme Inc. Read the [install guide](guide/install.md).

[Status page](https://status.acme.dev)
"""

[theme]
dark = ["tomorrow-night", "github-dark"]
light = "tomorrow"
search = "pagefind"

[fonts]
body = "Inter"
mono = "JetBrains Mono"

[[links]]
title = "GitHub"
url = "https://github.com/acme/acme"
```

The same settings in YAML:

```yaml
title: Acme Docs
description: Docs for Acme.
url: https://acme.dev/docs/
author: Acme Inc.
language: en
favicon: favicon.svg
logo: logo.svg
footer: |
  Copyright Acme Inc. Read the [install guide](guide/install.md).

  [Status page](https://status.acme.dev)

theme:
  dark: [tomorrow-night, github-dark]
  light: tomorrow
  search: pagefind

fonts:
  body: Inter
  mono: JetBrains Mono

links:
  - title: GitHub
    url: https://github.com/acme/acme
```

## Keys

Every key is optional.

| Key           | Default | Description |
| ------------- | ------- | ----------- |
| `title`       | See description | Site title in the header and browser tabs. Defaults to the home page's title, else "Documentation". |
| `description` | none    | Description for search engines and link previews, used on pages without a `description` in their [front matter](front-matter.md). |
| `url`         | none    | Full address of the published site, such as `https://acme.dev/docs/`. It must start with `http://` or `https://`. When set, each page gets a canonical link and an Open Graph URL. |
| `base_path`   | the path of `url`, else `/` | URL prefix the site is served under. See [base path](../guide/deploying.md#base-path). |
| `author`      | none    | Author name, in a `<meta name="author">` tag. |
| `language`    | `en`    | Language of the pages, as a language tag such as `en` or `pt-BR`. |
| `favicon`     | none    | Icon for browser tabs: the path of an image in the docs directory, such as `favicon.svg`. SVG, PNG, ICO, GIF, JPEG, WebP, and AVIF files get a matching type. |
| `logo`        | none    | Image shown before the title in the header, sized to the header's height: the path of an image in the docs directory. |
| `theme.dark`  | `tomorrow-night` | Dark color schemes: one name or a list. Each is a name from `documango themes` or a path to a base16 YAML file. The first is the default. |
| `theme.light` | `tomorrow` | Light color schemes, in the same form as `theme.dark`. |
| `theme.search` | `pagefind` | Search engine: `pagefind` or `builtin`. |
| `fonts.body`  | system font | Font for the page text, such as `"Inter"`. See [fonts](../guide/themes.md#fonts). |
| `fonts.heading` | `fonts.body` | Font for headings. |
| `fonts.mono`  | system monospace font | Font for code, such as `"JetBrains Mono"`. |
| `links`       | none    | Links shown in the header, in order. Each has a `title` and a `url`. |
| `footer`      | "Built with documango by Owais" | Markdown shown at the bottom of every page. Set it to `""` to remove the footer. See [footer](#footer). |

Paths to theme files are relative to the config file. Paths to the favicon
and the logo are relative to the docs directory.

### Header links

Each entry in `links` adds a link to the header:

```toml
[[links]]
title = "GitHub"
url = "https://github.com/acme/acme"

[[links]]
title = "Changelog"
url = "/docs/changelog/"
```

Links open in the same tab. On narrow screens, they move into the navigation
menu. documango uses each `url` as written, so a link to a page of your own
site needs the base path.

### Footer

`footer` holds Markdown that documango shows at the bottom of every page. It
can span several lines: use a `"""` string in TOML or a `|` block in YAML, as
in the [example](#example).

Relative links in the footer work as they do in a page at the top of the docs
directory: `guide/install.md` links to the install page, with the base path in
front. Links open in the same tab.

Without a `footer` key, pages show "Built with documango by Owais", with links
to the documango repository and the author's site. To remove the footer, set
`footer` to an empty string:

```toml
footer = ""
```

### Page metadata

documango adds these tags to the `<head>` of every page:

- `<html lang>` from `language`.
- `<meta name="description">` from the page's `description`, else the site
  `description`.
- `<meta name="author">` from `author`.
- Open Graph tags for link previews: `og:title`, `og:description`, and
  `og:site_name`.

With `url` set, each page also gets `<link rel="canonical">`, `og:url`, and
`og:type`. documango builds each page's full address from `url` and the
page's path, so `https://acme.dev/docs/` and the page `guide/install.md`
give `https://acme.dev/docs/guide/install/`.

## Flags and the config file

A flag given on the command line wins over the config file, and the config
file wins over the defaults:

| Flag            | Config key     |
| --------------- | -------------- |
| `--title`       | `title`        |
| `--base-path`   | `base_path`, else the path of `url` |
| `--dark-theme`  | `theme.dark`   |
| `--light-theme` | `theme.light`  |
| `--search`      | `theme.search` |

For example, to preview a site at `/` whose config sets `base_path = "/docs/"`:

```sh
documango docs --base-path /
```

## Validation

documango checks the config file before it builds the site, and stops with an
error that names the file when:

- A key isn't one of those listed above, such as a misspelled `titel`.
- A value has the wrong type, such as `title = 3`.
- `url` isn't a full `http://` or `https://` address, or has a query or
  fragment.
- `base_path` isn't a URL path.
- `favicon` or `logo` isn't a file in the docs directory, or is in a folder
  or file whose name starts with `_` or `.`, which documango skips.
- `theme.dark` or `theme.light` has an empty entry or lists a theme twice.
- A theme doesn't exist.
- `theme.search` isn't `pagefind` or `builtin`.
- A font name contains `"`, `;`, `{`, `}`, `<`, `\`, or a line break.
- A font doesn't exist on Fontsource. This check runs when documango
  downloads the font.
- A link has no `title` or no `url`.
