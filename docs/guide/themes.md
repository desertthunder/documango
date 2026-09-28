---
title: Themes
description: Choose the light and dark color schemes for a documango site, or use your own.
order: 3
---

Every documango site has a light theme and a dark theme. Each theme is a
[base16](https://github.com/tinted-theming/home) color scheme: a palette of
sixteen colors that sets the page background, text, links, callouts, and code
highlighting. documango includes over 300 schemes, and you can load your own
from a YAML file.

## Light and dark

By default, a site uses `tomorrow` as its light theme and `tomorrow-night` as
its dark theme. Readers see the theme that matches their operating system or
browser setting (`prefers-color-scheme`).

The sun and moon button in the header switches between the two themes. The
browser remembers the reader's choice and applies it to every page of the
site until they switch again. The site then stops following the system
setting for that reader.

## Choose a theme

Pass a theme's name to `--light-theme` or `--dark-theme`. Both flags work with
`documango`, `documango serve`, and `documango build`:

```sh
documango docs --light-theme gruvbox-light-medium --dark-theme gruvbox-dark-medium
```

Theme names are case-insensitive. If documango doesn't recognize a name, the
error suggests up to three similar names.

documango doesn't check that the light theme is light or that the dark theme
is dark. You can use two dark schemes, for example, if that's what you want.

## List the built-in themes

`documango themes` prints every built-in theme, one per line, with three
columns: the name you pass to the theme flags, the variant (`dark` or
`light`), and the display name.

Use `--variant dark` or `--variant light` to show only one kind:

```sh
documango themes --variant light
```

When you pipe the output to another program, documango prints tab-separated
columns, which makes the list easy to filter:

```sh
documango themes | grep -i solarized
```

The built-in schemes come from the
[tinted-theming schemes](https://github.com/tinted-theming/schemes) project and
are distributed under the MIT License.

## Use your own scheme

To use a scheme that isn't built in, pass the path to its YAML file instead of
a name:

```sh
documango docs --dark-theme themes/midnight.yaml
```

documango treats the value as a file path when it ends in `.yaml` or `.yml`,
or when it contains a `/` or `\`. Relative paths start from the directory you
run documango in.

> [!TIP]
> Keep custom schemes in a folder whose name starts with `_`, such as
> `docs/_themes/`. documango doesn't publish anything in that folder.

### Scheme format

A scheme file uses the tinted-theming format. `base00` through `base0F` are
required and must be six-digit hex colors:

```yaml
system: "base16"
name: "Midnight"
author: "Your Name"
variant: "dark"
palette:
  base00: "#1b1d23"
  base01: "#23262e"
  base02: "#2f333d"
  base03: "#5c6370"
  base04: "#7f848e"
  base05: "#c8ccd4"
  base06: "#e0e3e8"
  base07: "#f5f6f8"
  base08: "#e06c75"
  base09: "#d19a66"
  base0A: "#e5c07b"
  base0B: "#98c379"
  base0C: "#56b6c2"
  base0D: "#61afef"
  base0E: "#c678dd"
  base0F: "#be5046"
```

The other fields are optional:

- `system` must be `base16` or `base24` if present. documango uses the first
  sixteen colors of a base24 scheme.
- `variant` is `dark` or `light`. Without it, documango decides from the
  brightness of `base00`.
- `name` and `author` don't affect the site.

documango also reads the older base16 format, where the colors are top-level
keys without `#` and the name is in a `scheme` field:

```yaml
scheme: "Midnight"
author: "Your Name"
base00: "1b1d23"
base01: "23262e"
# ... through base0F
```

### Where each color appears

documango follows the base16 styling guidelines. This table shows where each
color is used outside code blocks:

| Color    | Used for                                          |
| -------- | ------------------------------------------------- |
| `base00` | Page background                                   |
| `base01` | Sidebar, code blocks, and raised surfaces         |
| `base02` | Borders and text selection                        |
| `base03` | Faint text, such as heading `#` links             |
| `base04` | Muted text                                        |
| `base05` | Body text and headings                            |
| `base08` | Caution callouts                                  |
| `base0A` | Warning callouts                                  |
| `base0B` | Tip callouts                                      |
| `base0D` | Links, focus outlines, and note callouts          |
| `base0E` | Visited links and important callouts             |

Code highlighting uses the full palette: for example, keywords use `base0E`,
strings use `base0B`, and function names use `base0D`.
