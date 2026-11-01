---
name: list
description: |-
  List components from charly.yml and filesystem — `charly box list`, enumerating images, layers, build targets, services, routes, volumes, aliases.
---

# charly box list -- List Components

Invoked as `charly box list {images,layers,targets,services,routes,volumes,aliases}`.
See `/charly-image:image` for the family overview.

## Overview

Enumerate images, layers, build targets, and layer properties from `charly.yml` and the filesystem. Useful for discovery and scripting.

## Quick Reference

| Action | Command | Description |
|--------|---------|-------------|
| List boxes | `charly box list boxes` | All boxes defined in charly.yml |
| List layers | `charly box list candies` | All layers found on filesystem |
| List targets | `charly box list targets` | Build targets in dependency order |
| List services | `charly box list services` | Layers that declare a `service:` field |
| List routes | `charly box list routes` | Layers that declare a `route:` field |
| List volumes | `charly box list volumes` | Layers that declare `volume:` |
| List aliases | `charly box list aliases` | Layers that declare `alias:` |
| List image tags | `charly box list tags [<box>]` | Locally stored CalVer tags of charly-built images, newest first (rollback discovery for `charly update --tag`) |

## Usage

### List All Images

```bash
# Show all images from charly.yml
charly box list boxes

# Output: one image name per line
# fedora
# fedora-builder
# sway-browser-vnc
# ...
```

### List All Layers

```bash
# Show all layers found in candy/ directory
charly box list candies
```

### Build Targets in Order

```bash
# Dependency-ordered list -- useful for scripting builds
charly box list targets
```

### Filter by Property

```bash
# Which layers provide services?
charly box list services

# Which layers define routes (Traefik)?
charly box list routes

# Which layers declare persistent volumes?
charly box list volumes

# Which layers provide host command aliases?
charly box list aliases
```

## Project directory override

`charly box list …` resolves `charly.yml` + `candy/` via `os.Getwd()`. Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`. See `/charly-image:image` "Project directory resolution".

## Cross-References

### `charly box` family siblings

- `/charly-image:image` -- Family overview + charly.yml composition reference
- `/charly-build:build` -- Build enumerated images
- `/charly-build:generate` -- Containerfile generation
- `/charly-build:inspect` -- Detailed inspection of a specific image
- `/charly-build:merge` -- Post-build layer consolidation
- `/charly-build:new` -- Scaffold a new candy
- `/charly-build:pull` -- Pull prebuilt images into local storage
- `/charly-build:validate` -- Validate charly.yml and layer definitions

### Related skills

- `/charly-image:layer` -- Layer authoring and structure
