---
title: Markdown
description: The Markdown syntax documango supports, with rendered examples.
order: 2
---

documango follows [CommonMark](https://commonmark.org/) with the GitHub
Flavored Markdown extensions, plus footnotes, definition lists, custom heading
IDs, callouts, and syntax highlighting. Each section below shows the Markdown
source followed by how it renders on this site.

## Tables

Separate columns with `|` and put a row of dashes under the header. Colons in
the dash row set the column alignment.

```markdown
| Command            | Purpose               |
| :----------------- | --------------------: |
| `documango serve`  | Preview with reload   |
| `documango build`  | Write the static site |
```

| Command            | Purpose               |
| :----------------- | --------------------: |
| `documango serve`  | Preview with reload   |
| `documango build`  | Write the static site |

## Task lists

Start a list item with `[ ]` or `[x]` to show a checkbox. Readers can't change
the checkboxes.

```markdown
- [x] Write the installation page
- [ ] Add screenshots
```

- [x] Write the installation page
- [ ] Add screenshots

## Strikethrough

Wrap text in `~~` to strike it through.

```markdown
Run ~~`make docs`~~ `documango build docs` instead.
```

Run ~~`make docs`~~ `documango build docs` instead.

## Autolinks

URLs and email addresses in text become links without any extra syntax.

```markdown
Source code: https://github.com/desertthunder/documango
```

Source code: https://github.com/desertthunder/documango

## Footnotes

Add a marker such as `[^1]` in the text and define it anywhere in the page.
documango collects footnotes at the bottom of the page.

```markdown
Themes use the base16 palette.[^base16]

[^base16]: A base16 scheme defines exactly sixteen colors.
```

Themes use the base16 palette.[^base16]

[^base16]: A base16 scheme defines exactly sixteen colors.

## Definition lists

Put the term on one line and its definition on the next line, starting with
`: `.

```markdown
Base path
: The URL prefix a site is served under, such as `/docs/`.
```

Base path
: The URL prefix a site is served under, such as `/docs/`.

## Headings and anchors

Each heading gets an ID, so you can link to it with `#id`. documango builds the
ID from the heading text in the same way GitHub does: letters and numbers are
lowercased, spaces become `-`, and punctuation is dropped. A heading named
"Headings and anchors" gets the ID `headings-and-anchors`. If two headings
produce the same ID, the second gets `-1` added, the third `-2`, and so on.

Hover over a heading to show a `#` link to it.

To choose the ID yourself, add `{#id}` at the end of the heading:

```markdown
### Custom IDs {#custom-heading-id}
```

### Custom IDs {#custom-heading-id}

This heading's ID is `custom-heading-id`, so
[this link](#custom-heading-id) points to it.

The first level-1 heading on a page can set the page title. See
[titles](writing-pages.md#titles). Level-2 and level-3 headings appear in the
"On this page" list and in search.

## Callouts

A callout highlights information the reader shouldn't miss. Write a
blockquote whose first line is `[!NOTE]`, `[!TIP]`, `[!IMPORTANT]`,
`[!WARNING]`, or `[!CAUTION]`. This is the same syntax GitHub uses.

```markdown
> [!NOTE]
> Useful information that readers should know, even when skimming.

> [!TIP]
> Advice for doing things better or more easily.

> [!IMPORTANT]
> Key information readers need to achieve their goal.

> [!WARNING]
> Urgent information that needs immediate attention to avoid problems.

> [!CAUTION]
> Advice about risks or negative outcomes of certain actions.
```

> [!NOTE]
> Useful information that readers should know, even when skimming.

> [!TIP]
> Advice for doing things better or more easily.

> [!IMPORTANT]
> Key information readers need to achieve their goal.

> [!WARNING]
> Urgent information that needs immediate attention to avoid problems.

> [!CAUTION]
> Advice about risks or negative outcomes of certain actions.

The marker is case-insensitive and must be alone on the first line. A
blockquote with any other first line, or an unknown type such as `[!INFO]`,
renders as an ordinary blockquote.

## Code

Indent code or wrap it in a fence of three backticks. Name the language after
the opening fence to highlight it:

````markdown
```go
package main

import "fmt"

func main() {
	fmt.Println("Hello from documango")
}
```
````

```go
package main

import "fmt"

func main() {
	fmt.Println("Hello from documango")
}
```

documango highlights [the languages Chroma supports](https://github.com/alecthomas/chroma#supported-languages),
using colors from the current theme. Code without a language name, or with a
name Chroma doesn't know, is shown without highlighting. documango doesn't
guess the language.

## Raw HTML

HTML in a Markdown file is passed through to the page unchanged. Use it for
anything Markdown can't express, such as keyboard keys or collapsible sections:

```markdown
<details>
<summary>Show the keyboard shortcut</summary>

Press <kbd>/</kbd> to search.

</details>
```

<details>
<summary>Show the keyboard shortcut</summary>

Press <kbd>/</kbd> to search.

</details>

Raw HTML has these limits:

- documango doesn't rewrite links inside it. See
  [links and images](writing-pages.md#links-and-images).
- The [built-in search](writing-pages.md#built-in-search) doesn't index text
  inside raw HTML. Pagefind does.

> [!CAUTION]
> documango doesn't filter HTML, and that includes `<script>` tags. Only
> build sites from Markdown you trust.
