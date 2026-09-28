# AGENTS.md — plugin-box

Standalone plugin repo for the build-mode `charly box` commands
(`command:*:box`). The plugin is a Go module at `candy/plugin-box/` (module path
`github.com/opencharly/plugin-box/candy/plugin-box`); the root `charly.yml` only
declares `discover: candy` so the repo is a project and its candy is scanned.

Canonical files:

- `candy/plugin-box/charly.yml` — the `plugin-box:` candy entity (`plugin:`
  block, `plan:` check).
- `candy/plugin-box/` — the Go source: `plugin.go`, `box.go`, `validate*.go`,
  `inspect_list.go`, `box_load.go`, `merge_cmd.go`, `reconcile.go`,
  `cmd/serve/main.go`.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-image:image` — the `charly box` command family, box definitions in
  `charly.yml`, and the box dependency graph. Load before changing a command.
- `/charly-build:build` — `charly box build` / `charly box generate`.
- `/charly-build:validate` — `charly box validate` rules.
- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the unified Provider model, nested command parents. Load before
  touching the provider.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-box/` — compile the plugin module.
- `go test ./...` in `candy/plugin-box/` — the plugin's Go tests.
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The live R10 witness is the check-commands bed in `opencharly/charly` (the
  full `charly box generate/validate/new` end-to-end).

## Modify this repo

- These are **nested command** providers (`command:<word>:box`); keep the parent
  identity on the wire when adding a verb.
- The build drive lives in `plugin-build` — `generate`/`pull`/`build`/`merge`
  reach it over `InvokeProvider`, never by duplicating the drive.
- `validate` runs the whole rule engine in-plugin over the resolved-project
  envelope; keep new rules there, not in core.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
