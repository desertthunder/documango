---
title: Command line
description: Every documango command and flag, environment variables, and exit codes.
order: 1
---

documango has three commands: `serve` runs the development server, `build`
writes the static site, and `themes` lists the built-in color schemes.

```text
documango [dir]
documango serve [dir]
documango build [dir]
documango themes
documango --version
```

`dir` is the docs directory. It defaults to the current directory.

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
rebuild.

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
| `-o`, `--out` | `_site` | Folder to write the site to. If it's inside the docs directory, documango doesn't treat its contents as docs. |
| `--clean`     | off     | Delete everything in the output folder before building. documango refuses unless the folder is empty, doesn't exist, or contains a previous documango build (a `_documango/` folder). |

Without `--clean`, files from earlier builds stay in the output folder.

`build` also accepts the [site flags](#site-flags). See
[deploying](../guide/deploying.md) for publishing the result.

## themes

`documango themes` lists the built-in color schemes. Each line shows the
scheme's name, its variant (`dark` or `light`), and its display name. When the
output isn't a terminal, the columns are separated by tabs.

| Flag        | Default | Description |
| ----------- | ------- | ----------- |
| `--variant` | all     | Show only `dark` or only `light` schemes. |

## Site flags

These flags work with `documango`, `documango serve`, and `documango build`.

| Flag            | Default          | Description |
| --------------- | ---------------- | ----------- |
| `--title`       | See description  | Site title in the header and browser tabs. Defaults to the home page's title from front matter or its first level-1 heading, else "Documentation". |
| `--dark-theme`  | `tomorrow-night` | Dark color scheme: a built-in name or a path to a base16 YAML file. |
| `--light-theme` | `tomorrow`       | Light color scheme: a built-in name or a path to a base16 YAML file. |
| `--base-path`   | `/`              | URL prefix the site is served under, such as `/docs/`. |

See [themes](../guide/themes.md) and [base path](../guide/deploying.md#base-path)
for details.

## Global flags

These flags work with every command.

| Flag              | Description |
| ----------------- | ----------- |
| `-q`, `--quiet`   | Print only errors. This also hides the server address and the build summary. |
| `-v`, `--verbose` | Print more detail, such as the files that triggered a rebuild. |

`--quiet` and `--verbose` can't be used together.
| `--no-color`      | Don't use color in output. |
| `--version`       | Print the documango version and exit. |

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
| `1`  | The command failed, for example because the docs directory doesn't exist, a page has invalid front matter, or the port is in use. |
| `2`  | The command line is invalid, such as an unknown flag, too many arguments, or `--quiet` with `--verbose`. |

A mistyped command, such as `documango biuld`, is treated as a docs directory
that doesn't exist. documango exits with code 1 and suggests the closest
command.
