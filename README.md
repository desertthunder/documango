# Documango 🥭

A simple static site, documentation and blog generator tool.

## Installation

```bash
go build main.go
go install
go clean
```

Go from a README to a vibrant website with one command.

```bash
documango serve
# To build /content as a site to /dist
documango build
```

## Server

The local development server is configurable via the `[dev]` table in a
`config.toml` file found in the root of your project. See [this](./config.toml)
for an up to date example.

### Options

The `dev` section is entirely optional. The default values are as follows:

```toml
[dev]
port = 4242
content_dir = "content"
template_dir = "templates"
static_dir = "static"
level = "INFO"
```

Configuration options are populated through `context`.ß

#### Logging

Valid log levels are `DEBUG`, `INFO`, `WARN` and `ERROR`, and are case-insensitive.

```toml
[dev]
level = "info" # same as ⬆️
```

## Development

```bash
mkdir tmp
go build -o tmp

./tmp/documango -h
```

## Templates

There are three templates embedded in the binary using go's embed package. Two of which are
used for [themes](README#Theming), and one as a layout for your pages. When running through
the build process, templates are searched in this order:

1. Matches the name of the markdown file (ex. about.md looks for `{template_dir}/about.html`)
2. Looks for the template in the file's frontmatter (`layout` key).
3. Uses `{template_dir}/base.html` if it exists

## Theming

Themes come from the auto-generated repo from [tinted-theming](https://github.com/tinted-theming/schemes).
They are used to populate a snippet of css variables in `/cmd/view/themes/_theme.css`.
The stylesheet generated from these is in the same directory as `_style.css`

The pre-built themes for the site are:

| Light            | Dark             |
| ---------------- | ---------------  |
| Rosé Pine Dawn   | Rosé Pine        |
| Tokyo City Light | Tokyo City Dark  |
| Catppuccin Latte | Catppuccin Mocha |

### Color Schemes

<https://tinted-theming.github.io/tinted-gallery/>

## Resources

### Parking Lot

The initial implementation used treesitter to parse the markdown files. This was
dropped in favor of gomarkdown, which thoroughly implements the commonmark spec.
Future iterations of the project will remove parsers used for yaml, toml, and markdown
to keep the binary small.

Markdown files are parsed using an abstract syntax tree constructed with
the [inline-markdown](https://github.com/tree-sitter-grammars/tree-sitter-markdown)
grammar for treesitter.

```plaintext
(document
    (section
        (atx_heading (atx_h1_marker) heading_content: (inline))
        (paragraph (inline))
        (section
            (atx_heading (atx_h2_marker) heading_content: (inline))
            (paragraph (inline))
            (list
                (list_item (list_marker_minus) (paragraph (inline)))
                (list_item (list_marker_minus) (paragraph (inline)))
            )
        )
    )
)
```
