---
title: Front matter
description: The fields documango reads from the top of a Markdown file.
order: 2
---

Front matter is a block of YAML or TOML at the start of a Markdown file. The
first line of the file picks the format:

- `---` starts YAML. The block ends at the next line that is `---` or `...`.
- `+++` starts TOML. The block ends at the next line that is `+++`.

The delimiter lines may have trailing spaces. A UTF-8 byte order mark before
the first line is allowed. The same delimiters later in the file are part of
the page.

```yaml
---
title: Deploying
description: Build a static site and publish it.
order: 4
draft: false
---
```

```toml
+++
title = "Deploying"
description = "Build a static site and publish it."
order = 4
draft = false
+++
```

documango reads the fields below and ignores any others.

| Field         | Type    | Default | Description |
| ------------- | ------- | ------- | ----------- |
| `title`       | string  | none    | The page title in the sidebar, browser tab, search results, and previous and next links. Without it, documango uses the first level-1 heading, then the file or folder name. |
| `description` | string  | none    | Written to the page's `<meta name="description">` tag. |
| `order`       | integer | `0`     | Sort position among the page's siblings in the sidebar, lowest first. Ties are sorted by title, ignoring case. On an `index.md` or `README.md`, it positions the whole folder. |
| `draft`       | boolean | `false` | When `true`, the page isn't built. It has no URL and doesn't appear in the sidebar or search. |

An unclosed block, or YAML or TOML that doesn't parse, stops the build with an
error that names the file. For details on titles, ordering, and drafts, see
[writing pages](../guide/writing-pages.md#front-matter).
