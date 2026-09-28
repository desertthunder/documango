# documango

documango is a Go CLI that turns a folder of Markdown into a documentation
site: `documango [dir]` serves it with live reload, `documango build` writes
static files. The user docs in `docs/` are themselves a documango site.

## Layout

| Path | Role |
| --- | --- |
| `cmd/documango` | Entry point; calls `internal/cli`. |
| `cmd/tools` | Dev tools: `schemes`, `browsers`, `screenshots`. |
| `internal/cli` | Cobra commands, flags, config precedence, build and serve wiring. |
| `internal/config` | `documango.toml` / `documango.yaml` loading and validation. |
| `internal/site` | Loads a docs folder into pages, URLs and the nav tree. |
| `internal/markdown` | Goldmark rendering, headings, callouts, search text. |
| `internal/frontmatter` | YAML (`---`) and TOML (`+++`) front matter. |
| `internal/render` | HTML templates, CSS, `app.js`, search index. |
| `internal/server` | Dev server, file watcher, live reload over SSE. |
| `internal/theme` | base16 schemes: curated embedded set plus the GitHub catalog. |
| `internal/pagefind` | Downloads, verifies and runs Pagefind. |
| `e2e` | Playwright browser tests (build tag `e2e`). |

## Commands

Run Go with `GOTOOLCHAIN=local`; `go.mod` pins Go 1.25.

```sh
go vet ./... && go vet -tags e2e ./...
go test -race ./...
go run ./cmd/tools browsers          # once, installs Chromium
go test -tags e2e -count=1 ./e2e
node --check internal/render/assets/app.js
go mod tidy -diff                    # the release fails if this prints anything
```

## Conventions

- Test first. Keep coverage at or above 90% in the package you touch.
  Browser behaviour gets a test in `e2e/`.
- Unit tests stay offline: use `httptest`, `t.TempDir()`, the seeded cache in
  `internal/cli` tests, and the fake pagefind. Real-network tests are gated
  behind `DOCUMANGO_E2E=1`.
- CSS follows the `css` skill: tokens in `tokens.css`, one component per file,
  registered in `cssFiles` in `internal/render/render.go`.
- Docs and user-facing copy follow the `writing` skill: plain technical prose,
  sentence-case headings, the reader's language.
- CLI output follows clig.dev: errors as `Error:` plus an optional `Hint:` on
  stderr, exit 1 for runtime errors and 2 for usage errors, colour only on a
  terminal.
- Commits use conventional commit messages, short, with no LLM attribution
  trailers.

## Working with agents

Features are built by subagents and checked before they are committed:

1. The main session splits the work into packages with disjoint file
   ownership and states each package's API contract up front.
2. An `implementer` agent builds each package test first and touches only
   the files it owns. Implementers never run git.
3. `reviewer` agents look for defects through different lenses, and an
   `adversarial-reviewer` tries to disprove each finding before it counts.
4. The main session reads the diff, runs the checks above, and commits.

The `implement-review` workflow in `.claude/workflows/` runs steps 2 and 3 for
one package. Pass the task and the files the implementer owns:

```text
Run the implement-review workflow with args
{"task": "...", "owns": ["internal/foo/**"], "checks": "go test -race ./internal/foo"}
```
