---
title: Command line
description: Every documango command and flag, environment variables, and exit codes.
order: 1
---

documango has four commands: `init` creates a starter docs folder, `serve`
runs the development server, `build` writes the static site, and `themes`
lists the color schemes.

```text
documango init [dir]
documango [dir]
documango serve [dir]
documango build [dir]
documango themes
documango themes pick
documango --version
```

`dir` is the docs directory. It defaults to the current directory, except for
`init`, where it defaults to `docs`.

## init

`documango init` creates a starter site: a home page, a short guide on
writing pages, and a config file with every option listed and explained.
Missing folders are created.

```sh
documango init docs --title "Acme Docs"
```

| Flag       | Default                  | Description |
| ---------- | ------------------------ | ----------- |
| `--title`  | the project folder name  | Site title, written to the home page and the config file. By default, documango uses the name of the folder that holds `dir` when `dir` is named `docs` or `doc`, and the name of `dir` otherwise. `~/code/acme/docs` gets the title "Acme". |
| `--format` | `toml`                   | Config file format: `toml` writes `documango.toml`, `yaml` writes `documango.yaml`. |
| `--force`  | off                      | Write into a folder that isn't empty. Starter files that already exist are skipped and listed. |

Without `--force`, `init` refuses to write into a folder that isn't empty and
lists any starter files that are already there. It never overwrites a file.

When it finishes, `init` lists the files it created and the commands to
preview and build the site. With `--quiet`, it prints nothing.

## serve

`documango serve` builds the site in memory and serves it for local
development. Running `documango` with no command, or with only a directory,
does the same thing:

```sh
documango docs
documango serve docs
```

The server watches the docs directory. When a file changes, documango
rebuilds the site and every open page reloads. If a rebuild fails, the server
keeps serving the last successful build and the page shows the error in a
"Build failed" banner. If the first build fails, documango exits with the
error.

Changes to files and folders whose names start with `.` don't trigger a
rebuild. Saving the [config file](configuration.md) rebuilds the site with
the new settings, except for the base path, which needs a restart.

| Flag           | Default     | Description |
| -------------- | ----------- | ----------- |
| `-p`, `--port` | `3000`      | Port to listen on. |
| `--host`       | `127.0.0.1` | Address to listen on. Use `0.0.0.0` to reach the server from other devices on your network. |

`documango` with no command accepts the same flags. `serve` also accepts the
[site flags](#site-flags).

## build

`documango build` writes the site as static files.

```sh
documango build docs --out public --clean
```

| Flag          | Default | Description |
| ------------- | ------- | ----------- |
| `-o`, `--out` | `_site` | Folder to write the site to. If it's inside the docs directory, documango doesn't treat its contents as docs. It can't be the docs directory or a folder that contains it. These checks follow symbolic links. |
| `--clean`     | off     | Delete everything in the output folder before writing the site. documango refuses unless the folder is empty, doesn't exist, or contains a previous documango build (a `_documango/` folder). |

Each build lists the files it writes in `_documango/manifest.json`. The next
build into the same folder deletes the files from that list that it no
longer writes, such as the page of a renamed Markdown file, and any folders
this leaves empty. Files that documango didn't write stay in place.

If the site can't be built, for example because a page has invalid front
matter, documango leaves the output folder unchanged, even with `--clean`.

`build` also accepts the [site flags](#site-flags). See
[deploying](../guide/deploying.md) for publishing the result.

## themes

`documango themes` lists the color schemes you can pass to `--dark-theme` and
`--light-theme`: the built-in schemes and the tinted-theming catalog, which
documango downloads and caches. In a terminal, each line shows a color
sample, the scheme's name, its variant (`dark` or `light`), and its display
name. When the output isn't a terminal, each line holds the name, variant,
display name, and `builtin` or `download`, separated by tabs.

| Flag        | Default | Description |
| ----------- | ------- | ----------- |
| `--variant` | all     | Show only `dark` or only `light` schemes. |
| `--offline` | off     | Show only the built-in schemes, without going online. |

See [themes](../guide/themes.md#list-the-themes) for examples.

## themes pick

`documango themes pick` opens an interactive list of the color schemes with a
preview, and prints the names you choose to standard output:

```sh
documango docs --dark-theme "$(documango themes pick --variant dark)"
```

It needs an interactive terminal. If you cancel, it prints nothing to
standard output and exits with code 1. See
[pick a theme interactively](../guide/themes.md#pick-a-theme-interactively)
for the keys.

| Flag        | Default | Description |
| ----------- | ------- | ----------- |
| `--variant` | all     | Show only `dark` or only `light` schemes. |
| `--offline` | off     | Show only the built-in schemes, without going online. |
| `--multi`   | off     | Select several schemes with <kbd>Space</kbd> and print them separated by commas. |

## Site flags

These flags work with `documango`, `documango serve`, and `documango build`.

| Flag            | Default          | Description |
| --------------- | ---------------- | ----------- |
| `--title`       | See description  | Site title in the header and browser tabs. Defaults to the home page's title from front matter or its first level-1 heading, else "Documentation". |
| `--dark-theme`  | `tomorrow-night` | Dark color schemes, separated by commas. Each is a name from `documango themes` or a path to a base16 YAML file. The first is the default. |
| `--light-theme` | `tomorrow`       | Light color schemes, in the same form as `--dark-theme`. |
| `--search`      | `pagefind`       | Search engine: `pagefind` or `builtin`. |
| `--base-path`   | `/`              | URL prefix the site is served under, such as `/docs/`. |
| `--config`      | See description  | Config file to read. Defaults to `documango.toml`, `documango.yaml`, or `documango.yml` in the docs directory, if one exists. |

Flags given on the command line override the
[config file](configuration.md). See [themes](../guide/themes.md),
[search](../guide/writing-pages.md#search), and
[base path](../guide/deploying.md#base-path) for details.

## Global flags

These flags work with every command.

| Flag              | Description |
| ----------------- | ----------- |
| `-q`, `--quiet`   | Print only warnings and errors. `serve` still prints the address it serves the site at. |
| `-v`, `--verbose` | Print more detail, such as the files that triggered a rebuild. |
| `--no-color`      | Don't use color in output. |

`--quiet` and `--verbose` can't be used together.

`documango --version` prints the documango version and exits.

## Environment variables

| Variable              | Description |
| --------------------- | ----------- |
| `NO_COLOR`            | Any non-empty value turns off color in output. |
| `TERM`                | The value `dumb` turns off color in output. |
| `DOCUMANGO_CACHE_DIR` | Folder for downloaded files: the theme catalog in `schemes/` and Pagefind in `pagefind/`. Defaults to a `documango` folder in your user cache folder. |
| `DOCUMANGO_PAGEFIND`  | Path to a Pagefind binary to use instead of one on your `PATH` or the download. |
| `GITHUB_TOKEN`        | GitHub token sent when documango downloads the theme catalog, which raises GitHub's rate limit. |

## Color output

documango uses color only when it writes to a terminal. It also turns color
off when any of these is true:

- The `--no-color` flag is set.
- The `NO_COLOR` environment variable is set to any non-empty value.
- The `TERM` environment variable is `dumb`.

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0`  | Success. |
| `1`  | The command failed, for example because the docs directory doesn't exist, a page has invalid front matter, or the port is in use. Cancelling `documango themes pick` also exits with `1`. |
| `2`  | The command line is invalid, such as an unknown flag, too many arguments, or `--quiet` with `--verbose`. |

A mistyped command, such as `documango biuld`, is treated as a docs directory
that doesn't exist. documango exits with code 1 and suggests the closest
command.
