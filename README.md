# Documango 🥭

A simple static site, documentation and blog generator tool.

## Usage

```bash
go build main.go
go install
go clean

documango serve

# To build the site to ./dist
documango build -c /path/to/your/content
```

## Server

## Development

```bash
mkdir tmp
go build -o tmp

./tmp/documango -h
```

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
