# Documango 🥭

A simple static site, documentation and blog generator tool.

## Usage

```bash
go build main.go
go install
go clean

documango serve
```

## Server

## Development

## Resources

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
## Theming

Themes come from the auto-generated repo from [tinted-theming](https://github.com/tinted-theming/schemes)

### Color Schemes

<https://tinted-theming.github.io/tinted-gallery/>
