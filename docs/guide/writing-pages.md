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

If the docs directory has no `index.md` or `README.md` at the top, documango
makes a contents page for `/` instead. It lists every page in sidebar order,
with the site title as its heading, and isn't included in search. documango
prints a warning when it does this, so add an `index.md` to write your own
home page.

Two files can't produce the same URL. For example, `guide.md` and
`guide/index.md` both map to `/guide/`, and documango stops with a "duplicate
page URL" error that names both files.

### Ignored files

documango skips any file or folder whose name starts with `.` or `_`, along
with everything inside such a folder. Use this for drafts folders, partials,
or anything else you keep next to your docs but don't want published. The
[`include`](../reference/configuration.md#include) key can publish files from
these folders, and a few site files, such as `.nojekyll`, are always published.
See [other files](#other-files).

documango also skips every `node_modules` folder, so the packages of a
JavaScript project in your docs folder never become pages.

A symbolic link to a file works like the file itself. documango skips a
symbolic link to a folder and prints a warning.

When you build into a folder inside the docs directory, documango also skips
that output folder.

### Other files

documango publishes a file that isn't Markdown, such as an image or a PDF, only
when the site uses it. It copies the file to the site at the same path, so
`images/diagram.png` in the docs directory is served at `/images/diagram.png`.
A file is published when:

- A page or the footer links to it with a Markdown link or image, such as
  `![Diagram](images/diagram.png)` or `[Report](files/report.pdf)`.
- It's the `favicon` or `logo` in the [config file](../reference/configuration.md).
- It's one of these files at the top of the docs directory, which hosts read:
  `CNAME`, `robots.txt`, `favicon.ico`, `humans.txt`, `.nojekyll`, `_headers`,
  `_redirects`, or anything in `.well-known/`.
- It matches a pattern in the [`include`](../reference/configuration.md#include)
  config key.

Other files, such as `package.json` or a config file for another tool, stay
out of the site. Run `documango build -v` to list them.

documango doesn't see links written as raw HTML, such as
`<img src="images/diagram.png">`. Add the files they point to to `include`.

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
- A path without an extension that names a Markdown file, such as
  `[Install](../installation)` for `installation.md`, becomes that page's URL.
  A file or folder with the exact name wins.
- Any other path, such as an image, gets the site's base path in front of it.
  See [base path](deploying.md#base-path).

documango leaves these links unchanged:

- Links with a scheme, such as `https:` or `mailto:`.
- Links that start with `/` or `#`.
- Relative links that point outside the docs directory.

> [!NOTE]
> Links and images written as raw HTML, such as `<a href="setup.md">` or
> `<img src="logo.png">`, aren't rewritten or checked. Use Markdown syntax for
> links you want documango to resolve.

### Broken links

documango checks every relative link and image in your Markdown.
`documango build` and `documango serve` print a warning for each one that
won't work, even with `--quiet`. The warning names the page and the link as you
wrote it:

```text
WARN guide/setup.md: link to "instal.md" does not match a page
```

| Warning | Meaning |
| --- | --- |
| `does not match a page` | The link points to a Markdown file that isn't a page: it doesn't exist, it's skipped, or it's an excluded path. |
| `is a draft` | The link points to a page with `draft: true`. |
| `does not match a page or file` | The link points to a path that is neither a page, a folder with a page, nor a file in the docs directory. |
| `points outside the docs folder` | The link climbs above the docs directory, such as `../../notes.md`. |
| `the target page has no heading "#install"` | The page exists, but no heading or other element on it has that ID. Heading IDs set with `{#id}` count. |

A link that starts with `#` is checked against the page it's in.

Links with a scheme and links that start with `/` aren't checked. While
`documango serve` runs, each warning is printed once, when it first appears.

## Front matter

Front matter is a block of YAML or TOML at the very top of a file. It sets
page settings that aren't part of the content. YAML goes between two lines of
`---`:

```yaml
---
title: Installation
description: Install documango on macOS, Linux, or Windows.
order: 2
---
```

TOML goes between two lines of `+++`:

```toml
+++
title = "Installation"
description = "Install documango on macOS, Linux, or Windows."
order = 2
+++
```

The first line of the file must be exactly `---` or `+++`. A YAML block ends at
the next line that is `---` or `...`, and a TOML block ends at the next `+++`.
If the closing line is missing, or the block doesn't parse, documango reports
an error that names the file.

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
  appears only when a page has at least two of them. See
  [headings](#headings).
- Previous and next links.
- A footer, which you can change or remove with the `footer` key in the
  [config file](../reference/configuration.md#footer).

On narrow screens, the sidebar moves behind a menu button in the header.
While the menu is open, keyboard focus stays in the header and the menu.
Press <kbd>Escape</kbd> to close it.

### Headings

A heading made of an image, such as `## ![Architecture](diagram.svg)`, takes
its name from the image's alt text. The name appears in the "On this page"
list, and a level-1 image heading gives the page its title.

A heading with no text, such as an image heading without alt text, is left out
of the "On this page" list. `documango build` and `documango serve` print a
warning that names the file, even with `--quiet`:

```text
WARN guide/about.md: a level-2 heading has no text; give its image "diagram.svg" alt text
```

While `documango serve` runs, a warning is printed when it appears. Later
rebuilds don't repeat it until you fix the heading and the problem comes back.

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
