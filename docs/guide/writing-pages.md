---
title: Writing pages
description: How documango turns files and folders into pages, URLs, and navigation.
order: 1
---

documango reads every Markdown file in your docs directory and turns each one
into a page. The folder structure becomes the sidebar, and front matter at the
top of a file sets its title, description, and position.

## Files and URLs

Each Markdown file becomes a page with a URL that ends in a slash. Files with
the `.md` or `.markdown` extension count as Markdown.

| File                     | URL                     |
| ------------------------ | ----------------------- |
| `index.md`               | `/`                     |
| `install.md`             | `/install/`             |
| `guide/index.md`         | `/guide/`               |
| `guide/configuration.md` | `/guide/configuration/` |

A file named `index.md` is the page for its folder. If a folder has no
`index.md`, a `README.md` in that folder takes its place. If a folder has both,
documango uses `index.md` and ignores `README.md`. These names are
case-insensitive, so `Readme.md` and `INDEX.markdown` also work.

The `index.md` at the top of the docs directory is the home page. It doesn't
appear in the sidebar. Readers reach it through the site title in the header.

Two files can't produce the same URL. For example, `guide.md` and
`guide/index.md` both map to `/guide/`, and documango stops with a "duplicate
page URL" error that names both files.

### Ignored files

documango skips any file or folder whose name starts with `.` or `_`, along
with everything inside such a folder. Use this for drafts folders, partials,
or anything else you keep next to your docs but don't want published.

When you build into a folder inside the docs directory, documango also skips
that output folder.

### Other files

Files that aren't Markdown, such as images, PDFs, or downloads, are copied to
the site at the same path. `images/diagram.png` in the docs directory is
served at `/images/diagram.png`.

A file can't use a path that documango generates itself. A `404.html` at the
top of your docs directory, for example, stops the build with an error.

## Links and images

Link to other pages with relative paths to their Markdown files.
documango rewrites the link to the page's URL:

```markdown
See [writing pages](writing-pages.md) and the [CLI reference](../reference/cli.md#serve).
```

In a page in the `guide/` folder, those links point to `/guide/writing-pages/` and
`/reference/cli/#serve`. Links keep their `#section` and `?query` parts.

documango resolves each relative link or image path from the folder of the
page that contains it:

- A path to a Markdown file becomes that page's URL.
- A path to a folder with an `index.md` or `README.md` becomes that folder's
  URL, so `[Guide](../guide/)` works.
- Any other path, such as an image, gets the site's base path in front of it.
  See [base path](deploying.md#base-path).

documango leaves these links unchanged:

- Links with a scheme, such as `https:` or `mailto:`.
- Links that start with `/` or `#`.
- Relative links that point outside the docs directory.

> [!WARNING]
> documango doesn't check that a link target exists. A link to a missing file
> or a draft page becomes a broken link on the site, with no error.

> [!NOTE]
> Links and images written as raw HTML, such as `<a href="setup.md">` or
> `<img src="logo.png">`, aren't rewritten. Use Markdown syntax for links you
> want documango to resolve.

## Front matter

Front matter is a block of YAML at the very top of a file, between two lines
of `---`. It sets page settings that aren't part of the content.

```yaml
---
title: Installation
description: Install documango on macOS, Linux, or Windows.
order: 2
---
```

The first line of the file must be exactly `---`. The block ends at the next
line that is `---` or `...`. If the closing line is missing, or the YAML is
invalid, documango reports an error that names the file.

documango reads four fields and ignores any others:

| Field         | Type    | Default | Effect                                         |
| ------------- | ------- | ------- | ---------------------------------------------- |
| `title`       | string  | none    | Page title in the sidebar, browser tab, search, and pager. |
| `description` | string  | none    | The page's `<meta name="description">` tag.    |
| `order`       | integer | `0`     | Position among its siblings in the sidebar.    |
| `draft`       | boolean | `false` | When `true`, the file isn't published.         |

### Titles

documango picks each page's title from the first of these that exists:

1. The `title` field in front matter.
2. The text of the first level-1 heading (`# Heading`) in the page.
3. The file name with dashes and underscores turned into spaces and the first
   letter capitalized. `getting-started.md` becomes "Getting started". An
   `index.md` or `README.md` uses its folder's name instead, and the home page
   becomes "Home".

If the page content has no level-1 heading, documango shows the title as the
page heading. If you set a `title` and also write a `#` heading, the page shows
your heading, and the sidebar, browser tab, and search use the `title`.

### Site title

The site title appears in the header and in every browser tab. documango uses
the first of these that exists:

1. The `--title` flag.
2. The home page's title, if it comes from front matter or a level-1 heading.
3. "Documentation".

### Order

The sidebar lists the pages and subfolders of each folder together, sorted by:

1. `order`, lowest first. Pages without `order` count as `0`, and negative
   numbers come before them.
2. Title, ignoring case.

A subfolder sorts by the `order` and title of its `index.md` or `README.md`.
A subfolder without one appears as a plain label, named after the folder,
with order `0`.

The **Previous** and **Next** links at the bottom of each page follow the
sidebar order, starting from the home page.

### Drafts

A page with `draft: true` is left out of the site entirely. It has no URL,
no sidebar entry, and no search entry. A draft `index.md` doesn't count as
its folder's page, so a `README.md` in the same folder is used instead.

```yaml
---
title: Upcoming features
draft: true
---
```

## Page layout

Every page has the same layout:

- A header with the site title, search, and a light and dark theme toggle.
  When the site offers more than one color scheme, the header also has a
  **Color scheme** menu. See [offer several schemes](themes.md#offer-several-schemes).
- A sidebar with every page except the home page. Each folder is a section
  that readers can expand and collapse. The section that holds the current
  page starts expanded, and the others start collapsed.
- An "On this page" list of the page's level-2 and level-3 headings. It
  appears only when a page has at least two of them.
- Previous and next links.

On narrow screens, the sidebar moves behind a menu button in the header.
While the menu is open, keyboard focus stays in the header and the menu.
Press <kbd>Escape</kbd> to close it.

## Search

The search box in the header searches every page in the site. Press <kbd>/</kbd>
anywhere on a page to jump to it. Use the arrow keys to move through the
results, <kbd>Enter</kbd> to open one, and <kbd>Escape</kbd> to close the list.
Search shows up to 10 results.

Search runs in the browser. documango indexes the content of each page: its
title and the text below it. The header, sidebar, and other parts of the
layout aren't indexed. Choose the search engine with `--search`:

- `pagefind`, the default, uses [Pagefind](https://pagefind.app/). It
  searches the full text of every page and matches different forms of a
  word, such as "build" and "builds".
- `builtin` uses a smaller search built into documango. It needs no
  download.

### Pagefind

documango runs Pagefind while it builds the site. It looks for Pagefind in
this order:

1. The path in the `DOCUMANGO_PAGEFIND` environment variable.
2. A `pagefind` program on your `PATH`.
3. A copy it downloaded earlier.

If none of these exists, documango downloads Pagefind 1.5.2 from its GitHub
releases (about 5 MB), checks the file against a known checksum, and stores it in your
user cache folder, or in the `pagefind` folder inside `DOCUMANGO_CACHE_DIR`
if that variable is set. Downloads are available for macOS, Linux, and
Windows on x86-64 and ARM64. On other platforms, install Pagefind yourself
and set `DOCUMANGO_PAGEFIND` to its path.

If documango can't find, download, or run Pagefind, for example on the first
run without a network connection, it prints a warning and the site uses the
built-in search instead. The development server then keeps the built-in
search until you restart it.

The development server runs Pagefind again after every rebuild, so search
results stay current as you edit.

### Built-in search

The built-in search has these limits:

- It indexes each page's title, its level-2 and level-3 headings, and the
  first 5,000 characters of its text.
- It matches exact text, ignoring case. A result must contain every word you
  type. There's no stemming or typo tolerance, so `build` matches "builds"
  but `builds` doesn't match "build".
- It ranks pages with a match in the title above pages with a match in a
  heading, and those above matches in the text.
- Text inside raw HTML isn't indexed.
