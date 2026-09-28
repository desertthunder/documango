---
title: Themes
description: Choose the light and dark color schemes and the fonts of a documango site.
order: 3
---

Every documango site has a light theme and a dark theme. Each theme is a
[base16](https://github.com/tinted-theming/home) color scheme: a palette of
sixteen colors that sets the page background, text, links, callouts, and code
highlighting. You can use any base16 scheme from the
[tinted-theming schemes](https://github.com/tinted-theming/schemes) project,
or load your own from a YAML file.

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

documango includes 26 schemes, so they work without a network connection:

`catppuccin-latte`, `catppuccin-mocha`, `default-dark`, `default-light`,
`dracula`, `everforest-dark-medium`, `everforest-light-medium`, `github`,
`github-dark`, `gruvbox-dark-medium`, `gruvbox-light-medium`, `kanagawa`,
`nord`, `nord-light`, `one-light`, `onedark`, `rose-pine`, `rose-pine-dawn`,
`solarized-dark`, `solarized-light`, `tokyo-city-dark`, `tokyo-city-light`,
`tokyo-night-dark`, `tokyo-night-light`,
`tomorrow`, and `tomorrow-night`.

When you name any other scheme, documango downloads the full tinted-theming
catalog from GitHub, which has over 300 base16 schemes. See
[the theme download](#the-theme-download) for where it's stored and what
happens offline.

If a scheme in `--dark-theme` is a light scheme, or one in `--light-theme` is
dark, documango prints a warning and uses it anyway.

## Offer several schemes

Both flags take a comma-separated list. The first scheme in each list is the
default, and the others let readers pick a different one:

```sh
documango docs --dark-theme tomorrow-night,dracula,nord
```

When either list has more than one scheme, the site header shows a
**Color scheme** menu. It lists the schemes for the mode the reader is
viewing, light or dark, and is hidden in a mode that has only one scheme. The
reader's browser remembers the choice separately for light and dark mode and
applies it on every page of the site.

A list can mix names and [scheme files](#use-your-own-scheme). Each scheme can
appear only once in a list.

This site offers a sample of the built-in schemes. Open the **Color scheme**
menu in the header to try them, and use the light and dark toggle to switch
between the two lists.

## List the themes

`documango themes` prints every scheme you can use, one per line. In a
terminal, each line shows a sample of the scheme's colors, its name, its
variant (`dark` or `light`), and its display name, and marks the built-in
schemes.

Use `--variant dark` or `--variant light` to show only one kind:

```sh
documango themes --variant light
```

To list only the built-in schemes without going online, add `--offline`:

```sh
documango themes --offline
```

When you pipe the output to another program, documango prints four
tab-separated columns: the name, the variant, the display name, and `builtin`
or `download`. This makes the list easy to filter:

```sh
documango themes | grep -i solarized
```

If the catalog can't be downloaded, `documango themes` prints a warning and
lists the built-in schemes.

## Pick a theme interactively

`documango themes pick` lets you browse the schemes in your terminal, with a
preview of each one's colors on a sample of code. It prints the name of the
scheme you choose to standard output, so you can pass the result straight to
a theme flag:

```sh
documango docs --dark-theme "$(documango themes pick --variant dark)"
```

Use these keys in the picker:

| Key                                                   | Action             |
| ----------------------------------------------------- | ------------------ |
| Any text                                              | Filter the list    |
| <kbd>Up</kbd>, <kbd>Ctrl+K</kbd>, <kbd>Ctrl+P</kbd>   | Move up            |
| <kbd>Down</kbd>, <kbd>Ctrl+J</kbd>, <kbd>Ctrl+N</kbd> | Move down          |
| <kbd>Page Up</kbd>, <kbd>Page Down</kbd>              | Move by one screen |
| <kbd>Enter</kbd>                                      | Choose the scheme  |
| <kbd>Esc</kbd>, <kbd>Ctrl+C</kbd>                     | Cancel             |

With `--multi`, <kbd>Space</kbd> selects or clears a scheme, and
<kbd>Enter</kbd> prints the selected names separated by commas, in the order
you selected them. The first one you select becomes the default:

```sh
documango build docs --light-theme "$(documango themes pick --variant light --multi)"
```

The picker accepts `--variant` and `--offline` like `documango themes`. It
needs an interactive terminal. If you cancel, it prints nothing to standard
output and exits with code 1.

## The theme download

documango downloads the tinted-theming catalog the first time you name a
scheme that isn't built in or run `documango themes` without `--offline`. It
stores the catalog in your user cache folder:

- Linux: `~/.cache/documango/schemes/`, or under `$XDG_CACHE_HOME` if it's set
- macOS: `~/Library/Caches/documango/schemes/`
- Windows: `%LocalAppData%\documango\schemes\`

Set `DOCUMANGO_CACHE_DIR` to store it somewhere else. documango then uses the
`schemes` folder inside that directory.

The download stays in use for 7 days. After that, documango downloads the
catalog again. If that fails, for example because you're offline, documango
keeps using the old copy and prints a warning. Without any copy, only the
built-in schemes and scheme files work.

documango downloads the catalog through the GitHub API. When the
`GITHUB_TOKEN` environment variable is set, documango sends it with the
request, which raises GitHub's rate limit.

The built-in schemes always come from documango itself, even when the
downloaded catalog has a newer version of the same scheme, so a build that
uses only built-in schemes gives the same result on every machine.

## Use your own scheme

To use a scheme that isn't in the catalog, pass the path to its YAML file
instead of a name:

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
- `name` is the scheme's label in the **Color scheme** menu.
- `author` doesn't affect the site.

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

The first six colors set the page's surfaces and text:

| Color    | Used for                                  |
| -------- | ----------------------------------------- |
| `base00` | Page background                           |
| `base01` | Sidebar, code blocks, and raised surfaces |
| `base02` | Borders and text selection                |
| `base03` | Faint text, such as heading `#` links     |
| `base04` | Muted text                                |
| `base05` | Body text and headings                    |

Links and callouts need colors of a particular hue, such as blue for links and
red for caution callouts. The base16 guidelines assign each hue to a slot, for
example blue to `base0D`, but many schemes put their colors in other slots.
documango looks at the hues of `base08` through `base0F` and matches each
color below to the closest one. For a scheme that follows the guidelines, the
result is the slot shown in the table.

| Color  | Usual slot | Used for                                  |
| ------ | ---------- | ----------------------------------------- |
| Red    | `base08`   | Caution callouts                          |
| Yellow | `base0A`   | Warning callouts and search highlights    |
| Green  | `base0B`   | Tip callouts                              |
| Blue   | `base0D`   | Links, focus outlines, and note callouts  |
| Purple | `base0E`   | Visited links and important callouts      |

Code highlighting uses the slots directly, following the base16 guidelines:
for example, keywords use `base0E`, strings use `base0B`, and function names
use `base0D`.

## Fonts

Pages use your system's fonts unless you pick others in the `[fonts]` section
of the [config file](../reference/configuration.md):

```toml
[fonts]
body = "Inter"
heading = "Lora"
mono = "JetBrains Mono"
```

`body` sets the page text, `heading` the headings, and `mono` code. Headings
use the body font unless you set `heading`. Use a family name from
[Fontsource](https://fontsource.org), which has the Google Fonts collection
and other open source fonts. If documango doesn't know a name, it stops and
suggests close ones.

documango copies the font files into your site, under `_documango/fonts/`.
Readers' browsers load them from your site, so the site sends no requests to
Google or any other font service. Browsers download only the parts of a font
that a page needs, such as the Latin characters.

### The font download

documango downloads each font from Fontsource the first time you use it. A
font for Latin text is a few hundred kilobytes; fonts for Chinese, Japanese,
or Korean are about 10 MB. documango stores the files in the `fonts` folder
of your user cache folder, or of `DOCUMANGO_CACHE_DIR` if it's set, and uses
them from there on every later build.

If the download fails, for example because you're offline, documango prints a
warning and builds the site with the system fonts. Build again once you're
connected.

A downloaded font stays at the same version. To get a newer version, delete
its folder, such as `fonts/inter`, from the cache folder.

### Font licenses

Most fonts on Fontsource use the SIL Open Font License, which allows
publishing them on your site. documango copies each font's license next to
its files, as `_documango/fonts/LICENSE-inter.txt` for Inter.
