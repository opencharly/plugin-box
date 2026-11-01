---
name: validate
description: |-
  `charly box validate`, validation rules, common validation errors, checking charly.yml and layer definitions.
---

# charly box validate -- Validation Commands

Invoked as `charly box validate`. See `/charly-image:image` for the family overview.

## Overview

`charly box validate` checks `charly.yml` and all layer definitions for errors. Validation collects all errors at once rather than failing on the first.

The param schema these rules enforce is **CUE-single-source**: the `@go()`-annotated `spec/schema/*.cue` defs are the sole source for both the Go param structs (generated into `spec/spec` by `charly task cue-gen`) and the load-time validation, so changing a validated field is a CUE edit → `charly task cue-gen` → see the `/charly-internals:go` recipe "How to change the charly.yml schema (CUE is the single source of truth)". The authoring surface is **node-form only** (name-first `<name>: {<kind>: …}`); a legacy kind-keyed document is rejected at load with a `charly migrate` hint.

## Quick Reference

| Action | Command | Description |
|--------|---------|-------------|
| Validate all | `charly box validate` | Check charly.yml + all layers (enabled images only) |
| Validate disabled too | `charly box validate --include-disabled` | Extend the validation pass to images marked `enabled: false` (does not modify charly.yml) |
| Check version | `charly version` | Verify CalVer computation |
| Inspect image | `charly box inspect <image>` | Show resolved config |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Validation passed |
| 1 | Validation or user error |
| 2 | Internal error |

## Validation Rules

### Layer Rules

- Layer directory must contain at least one install source — `charly.yml` with a non-empty `plan:` (carrying `run:` steps) or a `rpm:` / `deb:` / `pac:` / `aur:` packages section; an auto-detected builder manifest (`pixi.toml`, `pyproject.toml`, `environment.yml`, `package.json`, `Cargo.toml`); an `external_builder:` selection (the candy's content is the multi-stage build its selected external builder plugin bakes — see `/charly-image:layer`); a `plugin:` block; or a `candy:` composition field (pure composition layers are valid).
- **ADE is MANDATORY per candy:** every **local** candy MUST carry a non-empty `description:` string AND a `plan:` with at least one deterministic `check:` step (enforced by `validateCandyContents`; a hard error names the missing field). A fetched **remote** candy is exempt — its compliance is its own repo's concern, same scope as the mandatory `version:` rule. See `/charly-check:check`.
- `depends` must reference existing layers (local or remote).
- Circular dependencies are errors.
- `volumes` names must match `^[a-z0-9]+(-[a-z0-9]+)*$`.
- Volume names must be unique within a candy.
- `aliases` in charly.yml require both `name` and `command`.
- Alias names must match `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`.
- Setting `PATH` directly in `env` is an error (use `path_append`).
- Only one pixi manifest per layer (`pixi.toml`, `pyproject.toml`, or `environment.yml`).

### `run:` Step Rules

A `run:` step carries the deterministic state-change op inline (the same verb
catalog the legacy `task:` map used). See `/charly-image:layer` for the full
verb catalog. The validator enforces:

- **Exactly one op verb per `run:` step.** A `run:` step must carry exactly one of `cmd` / `mkdir` / `copy` / `write` / `link` / `download` / `setcap` / `build` / `config`. Zero op verbs → the step has no action; more than one → conflicting actions.
- **Per-verb required modifiers:**
  - `copy` → `to:` (destination) required; `copy:` value must be relative to the layer directory, no `..` traversal
  - `write` → `content:` required (non-empty)
  - `config` → `content:` required (non-empty); destination must be absolute/`~/`/`${HOME}`-prefixed; `validate:` (when set) names an egress kind; the config CONTENT is `${VAR}`-substituted at generate time, so an unresolved reference in it is a validate error (write: content stays verbatim — no such rule)
  - `link` → `target:` required (what the symlink points to)
  - `download` → `to:` required unless `extract: sh` (piped install scripts)
  - `setcap` with non-empty `caps:` → caps pattern check (`cap_name=flags[,cap_name=flags]`)
- **Path rules:** paths must be absolute, `~/`-prefixed, or `${HOME}`-prefixed. `copy:` source must exist under the layer directory at generate time.
- **Mode rules:** `mode:` must match `^0[0-7]{3,4}$` (octal) if present.
- **Download extract values:** must be one of `tar.gz` / `tar.xz` / `tar.zst` / `zip` / `none` / `sh` or empty.
- **`cache:` rules:** only valid on `cmd:` / `download:` `run:` steps; each path must be absolute (or `~/` / `${HOME}`-prefixed). Declares extra BuildKit cache mounts (owned per the step `run_as:`).
- **`build:` value:** must be `"all"` (initial implementation; specific builder names reserved for future use).
- **`run_as:` format:** must be `root`, `${USER}`, a literal name matching `^[a-z_][a-z0-9_-]*$`, or numeric `<uid>:<gid>`. Unresolved `${VAR}` in `run_as:` errors.

### `var:` Rules

- Keys must match `^[A-Z_][A-Z0-9_]*$` (shell identifier).
- Keys may not collide with reserved auto-exports (`USER`, `UID`, `GID`, `HOME`, `ARCH`, `BUILD_ARCH`).
- Keys may not collide with the same layer's `env:` keys.

### `${VAR}` Reference Resolution

- In non-shell fields (paths, URLs, `to`, `target`, etc.), every `${NAME}` reference must resolve against `var:` ∪ auto-exports. Unresolved references error at validate time.
- In shell fields (`cmd:` values, `write: content:`), references are passed through verbatim and resolved by bash at build time.
- **`config:` content is the ONE substituted body:** the config verb renders `content:` at GENERATE time (candy `var:` + auto-exports), so unresolved references in it are a validate-time error naming the field — the same rule the non-shell path fields follow.

### Image Rules

- `base` must reference a valid external image or another image in charly.yml
- `layers` field is required
- `builder` must reference an existing image
- Self-referencing builder is allowed (skipped by generator)
- `bootc: true` requires appropriate base image
- Duplicate alias names within an image are errors
- Box-level alias `command` defaults to `name` if omitted
- `env` accepts list of `KEY=VALUE` strings (runtime only)
- `env_file` accepts a path string (validated at runtime)
- `security` at image level overrides layer-level security (not an error)

### Bind Mount Rules

- `name` and `path` required, name must match volume name regex
- No duplicate names within an image
- Encrypted: `host` must be empty; Plain: `host` required
- Names matching layer volume names override the volume (note, not error)
- Warning if `gocryptfs` not in PATH when encrypted mounts exist

### Tunnel Rules

- Tailscale serve (tailnet-only): any port works for HTTPS
- Tailscale funnel (public): HTTPS port must be 443, 8443, or 10000
- Cloudflare: `fqdn` required

### VM / Bootc Rules

- `bootc: true` requires appropriate base image
- `vm.rootfs` must be `ext4`, `xfs`, or `btrfs`
- `vm.backend` must be `auto`, `libvirt`, or `qemu`

### Port Relay Rules

- Each relay port must be 1-65535
- Each relay port must also be declared in the layer's `port:`
- No duplicate relay ports across layers in the same image

### Tunnel Multi-Port Rules

- `ports` must be `"all"` or omitted
- When `ports: all`, the image must have at least one port defined
- Multi-port mode skips single-port HTTPS validation

### Port Protocol Rules

- Only `tcp:` prefix is supported
- Port number after prefix must be 1-65535

### Remote Layer Rules

- Two remote repos exporting the same layer name is an error
- Local layers shadow remote layers with same name (note emitted)
- A layer (bare `@github` ref) referenced via two different git tags is **not**
  an error — the git tag is only the FETCH coordinate. The resolver reads each
  fetched layer's OWN `version:` and dedups by it: same per-entity version → no
  warning (a re-tag of an unchanged layer is silent); different per-entity
  versions → warns once and resolves to the **newest** (highest CalVer). Run
  `charly box reconcile` to align the pins and clear any warning. A fetched layer
  with no `version:` IS a hard error (the layer kind requires one).
- Different layers from the same repo can use different versions
- Collection is reachability-scoped: only layers reachable from the enabled
  images' `base:`/`builder:` chains are fetched. See `/charly-internals:go`
  "Remote-layer resolver" and `/charly-build:reconcile`.

## Common Validation Errors

### "circular dependency"

Layers form a dependency cycle. Check `depends` fields.

### "layer X not found"

A `depends` entry or `charly.yml` layer references a non-existent layer.

### "PATH must not be set directly"

Use `path_append` in charly.yml instead of `env: PATH: ...`.

### "duplicate volume name"

Volume names must be unique within a candy.

## Common Workflows

### Validate Before Building

```bash
charly box validate && charly box build my-image
```

### Debug Validation Errors

```bash
charly box validate 2>&1                     # See all errors at once
charly box inspect <image>                   # Check resolved config
charly box list candies                       # Verify layer exists
```

## Project directory override

`charly box validate` resolves `charly.yml` via `os.Getwd()`. Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`. See `/charly-image:image` "Project directory resolution".

## Cross-References

### `charly box` family siblings

- `/charly-image:image` -- Family overview + charly.yml composition reference
- `/charly-build:build` -- Building validated images
- `/charly-build:generate` -- Containerfile generation after validation
- `/charly-build:inspect` -- Inspect a specific image after validation
- `/charly-build:list` -- Enumerate images/layers to validate
- `/charly-build:merge` -- Post-build layer consolidation
- `/charly-build:new` -- Scaffold new candies before validation
- `/charly-build:pull` -- Pull prebuilt images (orthogonal to validation)

### Related skills

- `/charly-image:layer` — **Canonical reference** for the `run:`/`check:` step verb catalog, `var:` substitution, YAML anchors, execution order. The validator rules above enforce what's documented there.
- `/charly-build:generate` — What the generator emits from validated input (per-verb emitters, cache-mount inheritance, inline-content staging).
- `/charly-internals:generate-source` — Internal architecture of the run-step emission pipeline.
- `/charly-internals:egress` — the **egress** counterpart: `charly box validate` proves the INPUT config; `/charly-internals:egress` validates the config files charly WRITES to a system (cloud-init, kubernetes, units, ssh_config, libvirt XML) before the bytes hit disk.
- `/charly-check:check` — `charly box validate` schema-checks every `check:` entry: exactly-one-verb, attribute types, scope/variable consistency (build-scope can't reference runtime-only vars), `id:` uniqueness per section, matcher operator allowlist, unroutable-check rejection. The live-container verbs (`cdp`/`wl`/`dbus`/`vnc`/`mcp`/`record`/`kube`/`adb`/`appium`/`spice`/`libvirt`) are out-of-process plugins: their method-name enum is enforced by CUE on core `#Op` (an unknown method is rejected with the allowed set listed) and their required-modifier checks run in the plugin at dispatch (deploy-scope-only).
- `/charly-build:charly-mcp-cmd` — the standalone reference for the `mcp:` verb: required modifiers (`tool:` for `call`, `uri:` for `read`), the 7-method allowlist, and the URL-rewrite / port-publishing behavior that authors occasionally hit.
- `/charly-check:cdp`, `/charly-check:wl`, `/charly-check:dbus`, `/charly-check:vnc` — per-verb references for the other four live-container verbs.

## Cross-kind name reuse across files — permitted; same-document collision — rejected

The unified form is **name-first**: every entity flattens to a top-level `<name>:` key whose first child is its kind discriminator (`candy:` / `pod:` / `vm:` / `kubernetes:` / `local:` / `android:` / `group:` / …). There is no `box:` KIND — both images and layers are `candy:` nodes (an image carries `base:`/`from:`, a layer carries neither). Two consequences for `charly box validate`:

- **Cross-FILE name reuse is fine.** The same name MAY exist simultaneously in SEPARATE discovered files — a layer `candy/redis/charly.yml` (a `candy:` node with no `base:`/`from:`) and an image `box/redis/charly.yml` (a `candy:` node carrying `base:`/`from:`) resolve to distinct internal maps (`uf.Candy` vs `uf.Box`, routed by `base:`/`from:` presence), and the validator does NOT flag them. Verbs disambiguate by command context (`ResolveDeployRef` is image-first; `--add-candy <name>` selects the layer-first path).
- **Same-document collision IS rejected.** Two top-level entities WITHIN one document (e.g. the root `charly.yml` carrying a `pod` + a `vm` + a `local`) MUST NOT share a name — they would collide on one YAML key. `charly box validate` flags the collision; rename one (the convention: keep the user-facing deploy, suffix the template it deploys — a `check-local` deploy + a `check-local-app` local template).

The loader still raises hard load-time errors on obsolete keys — the obsolete `qc` / `cachyos-dx` deployment names, and any obsolete `kind: deployment` doc or root-key `deployment:` (the deploy kind is now the name-first substrate kind: `pod:` / `vm:` / `kubernetes:` / `local:` / `android:` / `group:`). Every such error points at `charly migrate`. See the project rulebook (`AGENTS.md`) and `/charly-build:migrate`.

## When to Use This Skill

**MUST be invoked** when the task involves charly box validate command, validation rules, common validation errors, or checking charly.yml and layer definitions. Invoke this skill BEFORE reading source code or launching Explore agents.

**Workflow position:** Pre-build. Validate before building to catch errors early.
