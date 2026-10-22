---
name: inspect
description: |-
  Image inspection showing resolved configuration as JSON — `charly box inspect`, viewing image configuration, querying image metadata.
---

# charly box inspect -- Image Inspection

Invoked as `charly box inspect <image>`. See `/charly-image:image` for the family overview.

## Overview

Displays the fully resolved configuration of an image as JSON. Shows base image, layers, ports, platforms, registry, tags, builder selections, volumes, and all computed metadata.

## Quick Reference

| Action | Command | Description |
|--------|---------|-------------|
| Full inspect | `charly box inspect <image>` | Show complete resolved config as JSON |
| Specific field | `charly box inspect <image> --format FIELD` | Extract a specific field |
| Disabled image | `charly box inspect <image> --include-disabled` | Operate on `enabled: false` images without flipping authored config |

## Usage

```bash
# Inspect full image configuration
charly box inspect fedora

# Get specific field
charly box inspect jupyter --format ports

# Get the base image
charly box inspect sway-browser-vnc --format base

# Get layer list
charly box inspect ollama --format layers

# Get platforms
charly box inspect fedora --format platforms

# Get the builder map (build-type → builder image)
charly box inspect arch --format builder

# Get the builder capabilities this image declares
charly box inspect fedora-builder --format builds
```

## Output Fields

The JSON output includes:

| Field | Description |
|-------|-------------|
| `name` | Image name |
| `base` | Base image (another image name or external reference) |
| `layers` | Ordered list of layers applied |
| `ports` | Exposed ports from all layers |
| `platforms` | Target platforms (e.g., `linux/amd64`, `linux/arm64`) |
| `registry` | Container registry for push |
| `tags` | Image tags |
| `builder` | Build-type → builder-image map (e.g. `{"pixi": "fedora-builder"}`) — resolved from image → base → defaults |
| `builds` | Builder capabilities this image declares (e.g. `[pixi, npm, cargo, aur]`) — not inherited |
| `volumes` | Declared volumes from layers |
| `distro` | Distro identity tags |
| `build` | Package format tags |

All `--format` values are the JSON field names from the `inspect` output. When passing `--format` to select a map (like `builder`) or list, the CLI prints one entry per line.

**Caveat — `--format bind_mounts`**: this one format option reads `charly.yml` (not `charly.yml`), because bind-mount backings are a deploy-time concept (`charly config --bind <volume>` writes them to `charly.yml`). The output is display-only — no OCI label contamination, no build-mode state leak. All other `--format` values (`ports`, `volumes`, `layers`, `base`, `builder`, …) are strictly `charly.yml`-derived per the mode-purity invariant (see `/charly-build:build` and `/charly-internals:go` "Mode purity").

## Project directory override

`charly box inspect` resolves `charly.yml` via `os.Getwd()`. Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`. See `/charly-image:image` "Project directory resolution".

## Cross-References

### `charly box` family siblings

- `/charly-image:image` -- Family overview + charly.yml composition reference
- `/charly-build:build` -- Build the inspected image
- `/charly-build:generate` -- Containerfile generation for the inspected image
- `/charly-build:list` -- Enumerate images before inspecting one
- `/charly-build:merge` -- Post-build layer consolidation
- `/charly-build:new` -- Scaffold new candies
- `/charly-build:pull` -- Fetch prebuilt images into local storage
- `/charly-build:validate` -- Validate image and layer definitions before inspect

### Related skills

- `/charly-image:layer` -- Layer-level detail shown in inspect output
- `/charly-core:deploy` -- `charly.yml` overlay applied on top of inspect's resolved config
