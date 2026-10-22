---
name: pull
description: |-
  Fetch an image from its registry into local container storage so deploy-mode commands can read its OCI labels — `charly box pull`, the ErrImageNotLocal error, fetching by short name / fully-qualified ref / @github.com remote ref.
---

# charly box pull -- Fetch Image Into Local Storage

## Overview

`charly box pull` fetches an image from its registry into the local container
engine's storage so deploy-mode commands (`charly shell`, `charly start`, `charly config`,
`charly alias add`, etc.) can read its OCI labels via `ExtractMetadata`. It is a
**pull-only** operation — it does not start, configure, or restart any
service. That's `charly update`'s job (see `/charly-core:charly-update`).

This command is the prerequisite for every deploy-mode operation on a fresh
host. Since the `charly box` refactor, deploy-mode commands no longer read
`charly.yml` — they read OCI labels + `charly.yml` only. If an image isn't
in local storage, the label read fails and the CLI surfaces a friendly
recommendation pointing here.

## Quick Reference

| Action | Command | Description |
|--------|---------|-------------|
| Pull short name | `charly box pull jupyter` | Resolves registry + tag via `charly.yml` (requires project directory) |
| Pull fully-qualified ref | `charly box pull ghcr.io/opencharly/jupyter:2026.108.56` | Pulls as-is, works from anywhere |
| Pull remote project ref | `charly box pull @github.com/org/repo/image:v1` | Downloads repo, reads its `charly.yml`, pulls registry ref |
| Override tag (short name) | `charly box pull jupyter --tag 2026.108.1` | Pull a specific CalVer tag |
| Override platform | `charly box pull jupyter --platform linux/arm64` | Pull a specific platform |

## Three Input Forms

### 1. Short name (requires project directory)

```bash
cd ~/opencharly && charly box pull jupyter
```

Resolves `<registry>/jupyter:<tag>` via `charly.yml`. Equivalent to the
two-step:

```bash
charly box inspect jupyter --format registry  # ghcr.io/opencharly
charly box inspect jupyter --format tag       # ghcr.io/opencharly/jupyter:2026.108.56
# …then podman pull <that-ref>
```

…but packaged as a single command.

### 2. Fully-qualified ref (no project required)

```bash
charly box pull ghcr.io/opencharly/jupyter:2026.108.56
```

Runs the configured engine's `pull` directly (`podman pull` / `docker pull`, or
`nerdctl pull` when `engine.run: containerd`). Useful from any directory, including
`/tmp`. Supports any registry that the underlying engine (`podman`/`docker`/`containerd`)
can authenticate against.

### 3. Remote project ref (`@github.com/...`)

```bash
charly box pull @github.com/opencharly/charly/jupyter:2026.108.56
charly box pull @github.com/opencharly/charly/jupyter          # latest git tag
```

Downloads and caches the repo, reads its `charly.yml`, then pulls the
registry ref declared there. This is the **only** place `@github.com/...`
refs are accepted in `charly`. Deploy-mode commands (`charly shell`, `charly start`,
`charly config`, etc.) reject them with a message pointing users here.

## Semantics

- **Idempotent.** Running `charly box pull jupyter` twice with the image
  already local is a no-op (the engine's `pull` is layer-aware).
- **No side effects on services.** Unlike `charly update`, this command does
  not restart, reconfigure, or touch any running container.
- **Prints the resolved ref** on success. One line to stderr.
- **Respects engine selection.** Uses the run engine from
  `ResolveRuntime` (`engine.run`, default `auto`: detects podman, then docker;
  `containerd` is opt-in and never auto-detected).

## Interaction with `ErrImageNotLocal` (the sentinel pattern)

The CLI has a centralized design for "image not in local storage" errors.
Every deploy-mode command inherits the same friendly recommendation without
per-call-site code:

1. `ExtractMetadata(engine, imageRef)` in `spec/container/box_metadata_coneb.go`
   (re-exported as `kit.ExtractMetadata` / `deploykit.ExtractMetadata`) returns
   `spec.ErrImageNotLocal` (wrapped with the image ref) when the image is absent
   from local storage.
2. `dispatchBuildEnsure` (`charly/dispatch_build_ensure.go`) dispatches to
   candy/plugin-build's `build:ensure` word (`candy/plugin-build/ensure.go`),
   which returns the same sentinel when the image is absent from the RUN engine's
   store (the one every later read resolves it through).
3. `FormatCLIError` in `charly/image.go` unwraps the sentinel at the top-level
   error boundary in `main()` and renders:

```
Error: image "X" is not available locally.
       Run 'charly box pull X' to fetch it first
```

Any deploy-mode command that calls `ExtractMetadata` or `EnsureImage`
(shell, start, stop, config, deploy, update, remove, alias, vm, service,
cdp, wl, vnc, tmux, record, dbus, logs — essentially everything outside
`charly box`) automatically participates. If you're authoring a new command
that reads image metadata, just call `ExtractMetadata` and the
recommendation falls out for free.

## Interaction with `charly update`

| Aspect | `charly box pull` | `charly update` |
|---|---|---|
| Side effects | None. Pulls bytes, prints digest. | Pulls, seeds data into volumes, restarts active service. |
| Required before deploy | Yes (first time only). | No — it calls `pull` internally if needed. |
| Use when | You want labels available but aren't ready to deploy. Or just installed `charly` on a fresh host. | You have a running service and want to roll to a new image version. |

Rule of thumb: **`pull` is the prerequisite; `update` is the refresh.**
Use `charly box pull <image>` once to seed local storage; use `charly update
<image>` every time you bump the version afterward.

## Flags

- `--tag <tag>` (default: `latest`) — Image tag when resolving a short
  name. Ignored for fully-qualified or remote-project refs (the tag is
  embedded in the ref).
- `--platform <platform>` — Target platform (default: host). Passed to
  `podman pull --platform`.

## Typical Workflows

### Fresh host, new user

```bash
# Install charly (see README Install section)
charly box pull jupyter                # pull once; labels now readable
charly config jupyter                    # generate quadlet from labels + charly.yml
charly start jupyter                     # systemctl --user start
```

### Deploying a remote image

```bash
charly box pull @github.com/opencharly/charly/hermes:latest
charly config hermes
charly start hermes
```

### Fixing a broken deploy-mode command

```bash
charly shell jupyter
# Error: image "jupyter:latest" is not available locally.
#        Run 'charly box pull jupyter:latest' to fetch it first
charly box pull jupyter:latest          # follow the recommendation
charly shell jupyter                      # now works
```

## Why this command exists

Before the `charly box` refactor, deploy-mode commands had dual-mode logic:
try `charly.yml` first, fall back to OCI labels. That produced drift (the
two paths could diverge) and silently pulled-and-built remote refs as a
side effect of what looked like a simple `charly shell @github.com/...` call.

The refactor drew a hard line: deploy-mode commands read labels only;
build-mode commands (`charly box …`) read `charly.yml` only. `charly box pull`
is the bridge — it takes a build-mode identifier (short name or remote
repo ref) and produces a deploy-mode-consumable artifact (labels in local
storage).

## Project directory override

`charly box pull` resolves `charly.yml` via `os.Getwd()` when given a short name (to resolve registry + tag). Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`. Fully-qualified refs and `@github.com/...` remote refs don't need a project dir. See `/charly-image:image` "Project directory resolution".

## Cross-References

- `/charly-image:image` — family overview; `charly box pull` is one of 8 subcommands.
- `/charly-build:build` — pulls and builds are orthogonal; build creates images,
  pull fetches existing ones.
- `/charly-core:charly-update` — rolls deployed services to a new image version
  (pulls + data-seeds + restarts).
- `/charly-build:inspect` — print resolved ref from `charly.yml` without pulling.
- `/charly-core:shell`, `/charly-core:start`, `/charly-core:charly-config`, `/charly-automation:alias`, `/charly-vm:vm` —
  deploy-mode commands that require a pulled image.
- `/charly-core:deploy` — charly.yml overlay semantics applied on top of the
  labels `pull` materializes.
- `/charly-internals:go` — `ErrImageNotLocal` / `dispatchBuildEnsure` / `ExtractMetadata`
  source locations (`spec/spec/image_errors.go`, `charly/dispatch_build_ensure.go`,
  `spec/container/box_metadata_coneb.go`, `charly/image.go`).
