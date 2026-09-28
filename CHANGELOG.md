# Changelog

All notable changes to documango are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `documango [dir]` previews a folder of Markdown with live reload. A page
  that fails to build shows its error in the browser while the last good
  version keeps serving.
- `documango build` writes a static site with clean URLs and a 404 page, and
  removes files left over from earlier builds.
- Only files your pages link to are published, plus the favicon, logo, and
  host files such as `CNAME`. The `include` config key publishes others.
- A contents page at `/` when the docs folder has no `index.md` or
  `README.md`.
- `documango init` creates a starter docs folder and config file.
- A sidebar built from your folders, an "On this page" list, and previous and
  next links on every page.
- GitHub Flavored Markdown with footnotes, definition lists, callouts and
  syntax highlighting, plus YAML or TOML front matter.
- Light and dark themes from any tinted-theming base16 scheme. List several
  to give readers a color scheme menu, and pick one in the terminal with
  `documango themes pick`.
- Full-text search with Pagefind, falling back to a built-in index when
  Pagefind is unavailable.
- An optional `documango.toml` or `documango.yaml` for the title, page
  metadata, favicon, logo, header links, footer, themes and fonts.
- Fonts from Fontsource, downloaded once and served from your own site.
- `--open` to open the preview in your browser, and `--base-path` for sites
  served from a subpath.
- A warning for headings with no text, such as an image heading without alt
  text.
- Warnings for relative links and images that point to a missing page, file,
  or heading, a draft, or somewhere outside the docs folder.
- Links to pages without the `.md` extension, such as `[Install](installation)`,
  as other docs tools write them.
