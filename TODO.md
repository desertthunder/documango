# TODO

## Roadmap

### v0.1.0: first release

Everything planned is in; the release is ready to tag. See "Releasing" in
`docs/reference/development.md`.

### v0.2.0

- Per-page layouts: a `layout` front matter key and user templates in
  `_layouts/`, with the page layout split into a shell and one template per
  page kind.
- A sitemap when the config sets `url`.

### v0.3.0

- Collections for blogs and readers (see the parking lot below).

### v1.0.0

- Stable CLI flags, config keys, front matter fields and output URLs. Breaking
  any of them after this needs a major version.

## Parking lot: blogs and readers

Research and ideas, not scheduled work. The goal is to let documango give a
folder of Markdown a good web view, either as a blog next to the docs or as a
reader for a pile of notes and essays.

### Proposed model: collections

A collection is a folder whose pages are dated entries. Its index page lists
the entries newest first. The rest of the site stays docs-shaped.

- A blog is a collection at `blog/`.
- A zero-config reader is a collection at the root:
  `documango ~/notes --collection .`.
- A folder becomes a collection through `collection: true` in the front matter
  of its `index.md` or `README.md`, the repeatable `--collection <dir>` flag,
  or a `collections` list in the config file.
- A collection contains every published page in its folder and subfolders,
  except its own index. A subfolder `index.md` counts as one entry, so
  `blog/my-trip/index.md` can sit next to `blog/my-trip/photo.jpg`.
- Nested collections are an error.

Rejected alternatives:

- A site-wide `--mode blog` flag can't mix docs and a blog in one site.
- A separate `documango read` command would duplicate serve's flags. It could
  later become an alias for `serve --collection .`.
- Detecting collections automatically, for example from date prefixes, would
  change the layout when a file is renamed.

### Dates

An entry's date comes from, in order:

1. `date` in front matter.
2. A `YYYY-MM-DD-` prefix on the file name, or on the folder name for a
   subfolder index.
3. Nothing. The entry is undated.

Don't use file modification times or git history. Checkouts reset modification
times and shallow CI clones have no history, so builds would differ between
machines. Undated entries sort after dated ones and are left out of the feed
with one warning per collection.

### Front matter

| Field | Meaning |
| --- | --- |
| `collection` | On an index page only. Makes its folder a collection. |
| `date` | `2006-01-02`, `2006-01-02 15:04`, or RFC 3339. Invalid values fail the build. |
| `updated` | Same formats. Shown as "Updated …" and used as the feed's `updated`. |
| `tags` | A string or a list. Merged case-insensitively. Two tags with the same URL slug are an error. |
| `author` | Shown on the entry and in the feed. Defaults to the config `author`. |

`description` doubles as the listing summary and feed summary. The date and
tag parsers must handle both YAML and TOML front matter.

### Generated pages

| Page | URL | Contents |
| --- | --- | --- |
| Collection index | `/blog/` | Intro text from the index page, then entries grouped by year with date, summary, reading time and tags. |
| Tag index | `/blog/tags/` | Tags A to Z with counts. Only when tags exist. |
| Tag page | `/blog/tags/<slug>/` | Entries with that tag. |
| Feed | `/blog/feed.xml` | Atom 1.0, newest 20 dated entries with full content. Only when the site `url` is configured. |

The year-grouped index doubles as the archive. No pagination in the first
version.

### Layout

- Entries and listings use a single reading column (about 66 characters wide)
  with a header showing a link back to the collection, the date, updated date,
  author, reading time and tags.
- The docs sidebar moves into the menu drawer at every width on these pages.
  If the site has no docs pages, the menu button is omitted.
- Entry pages link to newer and older entries within the collection. Docs
  previous and next links never land on an entry.
- A collection appears in the docs sidebar as a single link.
- Add print styles for the whole site: hide the header, sidebar, table of
  contents, pager and footer.
- Pages get a kind: doc, entry or list. Each kind maps to a built-in template,
  and `layout` in front matter overrides it. This needs the page layout split
  into a shell with a `{{block "main" .}}` area and one template per kind.

### Search and drafts

- Entries carry their date into both search engines, and Pagefind gets date
  sort and tag filter attributes for a later filter UI.
- Listing and tag pages are not indexed, so entries don't appear twice.
- `serve --drafts` shows drafts with a "Draft" badge. `build` has no such flag,
  so drafts can't be published by accident.

### Later

- Wikilinks (`[[page]]`), matched by file name, case-insensitively. This is the
  first item to pull forward if Obsidian-style vaults become a target.
- Backlinks, built from the links the resolver already sees.
- Pagination, site-wide tags, series, related posts, social images,
  `<!--more-->` markers, scheduled posts and footnote popovers.

### Open questions

- Keep date prefixes in URLs (`/blog/2024-05-01-hello/`) or strip them? Keeping
  them matches the "file path is the URL" rule and avoids collisions.
- Tags per collection or site-wide? Per collection is simpler unless docs pages
  should be taggable.
- Is reading an Obsidian vault a real target? If so, wikilinks belong in the
  first version.

### Suggested work packages

1. Front matter fields and a markdown excerpt (first paragraph, up to 200
   characters).
2. An Atom feed package, independent of the rest.
3. The site model: collections, dates, sorting, tags, reading time and
   collision checks.
4. Rendering: entry and list templates, tag pages, feed link, reader layout
   CSS, and search dates.
5. CLI: `--collection`, `--url` if the config file lacks it, `serve --drafts`,
   and build warnings.
6. Documentation: a "Blogs and readers" guide and reference updates.
7. Browser tests: listing order, tag pages, newer and older links, the drawer
   at desktop width, the root reader, live reload of a new post, drafts,
   search dates, the feed and print styles.

## Parking lot: render a GitHub folder

Take a GitHub path instead of a local folder and render it as a site:

```sh
documango github.com/owner/repo/tree/main/docs
documango build github.com/owner/repo/tree/v1.2.0/docs
```

- Accept a GitHub tree URL (`https://github.com/owner/repo/tree/<ref>/<path>`)
  and a short form such as `owner/repo/docs@ref`. The ref defaults to the
  repository's default branch.
- Download the repository tarball from
  `api.github.com/repos/<owner>/<repo>/tarball/<ref>`, the same endpoint the
  theme catalog uses, extract only `<path>`, and cache it under the user cache
  directory keyed by the resolved commit SHA. Use `GITHUB_TOKEN` for private
  repositories and higher rate limits.
- Pin to a commit: resolve the ref to a SHA first so a build is reproducible,
  and print the SHA in the build summary.
- `serve` has nothing local to watch. It could check the ref every minute or
  so with a conditional request (ETag) and rebuild when the SHA changes, or
  just serve a fixed snapshot.
- Links that leave the folder, such as `../CONTRIBUTING.md`, should point at
  the file on GitHub (`github.com/owner/repo/blob/<sha>/...`) instead of
  becoming broken site links. A `README.md` without an `index.md` is already
  the folder's page.
- Relative images work once the files are extracted. Git LFS files and
  submodules are out of scope.
- Open questions: support other hosts (GitLab, Codeberg, tangled) through the
  same tarball pattern, or GitHub only? Add an "Edit on GitHub" link to each
  page when the source is a repository?

## Parking lot: brand

- A documango logo, favicon and brand icon, used in the README, this project's
  docs site, and the release pages.
- A default favicon for generated sites that set none, so browsers don't
  request a missing `/favicon.ico`. Keep it neutral, or tint it from the
  site's theme.
