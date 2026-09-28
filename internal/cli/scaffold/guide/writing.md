---
title: Writing pages
description: Add pages, set their titles and order, and format their content.
order: 1
---

Every `.md` file in the docs folder becomes a page. Its path sets the URL, so
`guide/writing.md` is served at `/guide/writing/`. Folders become sections in
the sidebar.

## Front matter

The YAML block at the top of this file is front matter. documango reads four
fields:

- `title`: the page title in the sidebar and browser tab. Without it,
  documango uses the first `#` heading, then the file name.
- `description`: a one-line summary for search engines and link previews.
- `order`: the page's position among its siblings, lowest first.
- `draft`: set to `true` to leave the page out of the site.

## Callouts

Start a blockquote with `[!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]`,
or `[!CAUTION]` to turn it into a callout:

> [!TIP]
> Run `documango` on this folder and keep the browser open. The page reloads
> each time you save a file.

## Code

Fenced code blocks are highlighted when you name the language:

```sh
documango build
```

## Links

Link to other pages by their Markdown file, relative to the current file.
documango turns the link into the page's URL. For example, this link goes
back to the [home page](../index.md).
