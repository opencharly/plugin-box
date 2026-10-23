---
name: build
description: |-
  Building container images, `charly box build`, pushing to registries, merging layers, build caches, or Containerfile generation. Use before any of those.
---

# charly box build -- Building Container Images

Invoked as `charly box build`. See `/charly-image:image` for the family overview.

## Overview

`charly box build` generates Containerfiles from `charly.yml` and layer definitions, then builds images in dependency order with **BuildKit**, charly's only image builder. Images at the same dependency level are built in parallel (up to `--jobs` concurrent builds).

**Mode purity**: `charly box build` reads `charly.yml` only. `charly.yml` is never read during build — this is enforced by `LoadConfig` in `charly/loader_threaded.go` (it reads the PROJECT `charly.yml` only and never merges the per-host `charly.yml` overlay — no `MergeDeployOverlay`), to guarantee OCI labels are baked strictly from authored configuration, never from local deploy-time overrides. See `/charly-internals:go` "Mode purity" for the architectural invariant this protects and the bug it prevents.

**Build dispatch**: `charly box build` does not construct a generator host-side at all (the former `NewGenerator` is DELETED) — `BuildCmd.Run` computes the build tag, holds the build-activity retention lock, and dispatches through the COMPILED-IN `build:box` plugin (candy/plugin-build) over the F10 reverse channel; on return it runs the retention prune host-side. The plugin's `runBoxBuild` → `resolveBuildEngine` (candy/plugin-build/resolve.go) runs the ENTIRE build RESOLVE plugin-side — loader, vocab, scan, connect, validate, box resolve, intermediates, order, effective versions, host-fs prep, render-seam-cache prep, render-prep, user-context, envelope projection, and drive-model — and returns `spec.BuildResolveReply` (the resolved-project envelope + a serializable drive-model: engine/platform, build order, per-image descriptors, resolved tunables; NO Containerfile content). This is the plugin-side replacement for the DELETED host `build-prep` fat seam (`charly/build_resolve_host.go`'s `hostBuildBuildResolve`). The plugin **OWNS the BuildKit DRIVE** (the build-order loop, per-image build lock, the `buildctl` argv, the store bridge, push, and the inline-merge gate) IN the candy — it imports only the sdk module. It reaches the host ONLY for thin `buildengine-*` shards a sdk-only candy cannot hold: `buildengine-scan-local` (local candy scan), `buildengine-connect-plugins` (registry plugin connect), and `buildengine-prep` (the render-seam-floor `renderGenCache` — `hostBuildPrep` stores a CHEAP candy-scan-only Generator for the render-seam floor's reverse-channel consumers and returns an EMPTY map; the host-fs PREP itself moved plugin-side in the K3 host-prep move — `runHostFSPrep` in candy/plugin-build/host_prep.go). The resolved-project envelope + drive-model are NOT returned by `buildengine-prep` — they come from `resolveBuildEngine`'s reply. The render DRIVE is `sdk/deploykit.Generator`, driven by the candy over the envelope + `HostBuild("render-seam")` for the host-coupled render seams (#67 — the host no longer renders the `.build/` tree); `bake_plugin:` binary baking is INLINE via `deploykit.EmitBakedPlugins` (no `HostBuild("bake-plugins")` round-trip — that host-builder + `host_build_bake_plugins.go` are DELETED). `charly box generate` mirrors this via `build:generate` → `runBoxGenerate` → `resolveBuildEngine(..., generateOnly=true)`: `generateOnly` is a PLUGIN-SIDE parameter (NOT a `HostBuild("buildengine-prep")` argument — that host leg takes `spec.ResolvedProjectRequest`, which has no `GenerateOnly` field) that returns after the envelope projection (no engine/order drive-model); the candy renders the `.build/` tree itself via `deploykit.Generator` + returns the written Containerfile paths; no podman. (The resolved-project envelope that OTHER verbs fetch — `charly box inspect`, `charly check`, … — comes over the separate `InvokeProvider("build","project",OpResolve)` peer seam, the `build:project` word; the build/generate path projects its own envelope inside `resolveBuildEngine`.) See `/charly-internals:plugin` (the `build` provider class + the in-proc reverse channel) and `/charly-build:generate`.

**Build-mode emission**: the build-mode Containerfile render DRIVE is `sdk/deploykit.Generator.Generate` / `generateContainerfile` (relocated from `charly/generate.go` in #67), driven by `candy/plugin-build` over the resolved-project envelope + `HostBuild("render-seam")`; the `charly/generate.go` top-level `Generate`/`writeCandySteps` orchestrator is DELETED. the pod-overlay deploy path reaches `deploykit.Generator.EmitTasks` DIRECTLY — `charly/tasks.go` holds no shim and cannot hold one (P16 import purity forbids `charly/` importing the sdk); that path stays for the pod-overlay deploy path (the candy `plugin-deploy-pod` constructs `deploykit.OCITarget` + renders via `deploykit.NewRenderGeneratorFromProject`, the per-step fragment over the `HostBuild("step-emit","oci-emit-step")` seam); the build-mode render (in `sdk/deploykit`) drives `WriteCandySteps` → `EmitTasks` directly. The render walks each layer's ops directly to write Containerfile text — NOT the `InstallPlan` IR. It shares the package-cascade / shell-snippet / localpkg compiler helpers with the IR (one source of truth, R2), but the overall walk is the direct generator. The `InstallPlan` IR + `deploykit.OCITarget.Emit` is the DEPLOY-mode path used by `charly deploy add` (the candy `plugin-deploy-pod`'s `add_candy:` overlay synthesis, rendered in its own code via `deploykit.OCITarget` + `deploykit.NewRenderGeneratorFromProject` + the external out-of-process deploys; the local/vm/kubernetes/android substrates are external plugins via `pluginDeployTarget` (S3b, dispatched through `candy/plugin-fleet`'s `Invoke(OpDeployDispatch)`) — `deploy:local` via candy/plugin-deploy-local and `deploy:vm` via candy/plugin-deploy-vm DO consume the IR (the plugin walks it via `kit.WalkPlans` over the reverse channel, the vm one over the guest `SSHExecutor`), while `deploy:kubernetes` via candy/plugin-kube does NOT, materializing its Kustomize tree PLUGIN-side (`candy/plugin-kube/materialize.go`'s `materializeKustomize`, reaching `candy/plugin-k8sgen` peer-to-peer over `verb:k8sgen`/`OpEmit`)). See `/charly-internals:install-plan` for the IR and `/charly-internals:generate-source` for the Go call graph.

**Three-phase templates**: the embedded build vocabulary's format and builder definitions split each install operation into `phases.{prepare, install, cleanup}.{container, host}` — three phases × two venues. Build-mode emission reads the `container` cell; local deploys read `host` — the `(install, container)` cell is the single source of truth for build-mode package installation. See `/charly-image:layer` "Service Declaration" for the analogue at the init-system level (`init.<name>.service_schema`).

## Quick Reference

| Action | Command | Description |
|--------|---------|-------------|
| Build all images | `charly box build` | Build all images in dependency order |
| Build specific image | `charly box build <image>` | Build single image for host platform |
| Build and push | `charly box build --push` | Build all platforms and push to registry |
| Build without cache | `charly box build --no-cache` | Disable build cache entirely |
| Merge layers | `charly box merge <image>` | Post-build layer optimization |

## charly box build Commands

```bash
charly box build [image...]                         # Build for local platform
charly box build --push [image...]                  # Build for all platforms and push
charly box build --platform linux/amd64 [image...]  # Specific platform
charly box build --cache registry [image...]         # Registry cache (read+write)
charly box build --cache image [image...]           # Image cache (read-only, default)
charly box build --cache gha [image...]             # GitHub Actions cache
charly box build --no-cache [image...]              # Disable cache entirely
charly box build --jobs N [image...]                # Max concurrent images per DAG level (0=auto from defaults.jobs, else 4)
charly box build --stage-parallelism N [image...]    # Max concurrent stages within a single build (0=auto: min(NCPU, defaults.stage_parallelism_cap))
charly box build <image> --include-disabled         # Build an `enabled: false` image without flipping authored config
charly box build <image> --dev-local-pkg            # EVAL-BED ONLY: build localpkg candies (the charly toolchain) from LOCAL in-dev source, not the published release
```

### `--dev-local-pkg` — disposable check beds bake the IN-DEVELOPMENT charly

The charly toolchain normally installs the latest **published** package in an
image build (downloaded from the per-distro repo via `download_template`). The
check-bed runner instead passes `--dev-local-pkg` for EVERY bed image build, so the
package is BUILT via the `charly generate-packages` plugin (nFPM) from the LOCAL
in-development binary + plugins — a disposable check bed always tests the
in-development charly, never a stale release. Generic across all kinds + all
localpkg candies, one decision point (`deploykit.RenderLocalPkgImageInstall`); a
production box build omits the flag. A dev build that cannot build via the plugin
HARD-errors (no silent release fallback, R3).
Full mechanics: `/charly-internals:install-plan` "Check-vs-production charly
toolchain"; the candy view: `/charly-tools:charly`.

## `--include-disabled` — operational rebuild of disabled images

Images with `enabled: false` in `charly.yml` are excluded from the build /
inspect / validate working set by default. To rebuild ONE such image without
flipping the authored flag (which would commit local-deployment intent into
shared project config), pass `--include-disabled`:

```bash
charly box build immich --include-disabled    # builds the disabled image
git diff --quiet charly.yml                  # confirms charly.yml is untouched
```

**Scoping.** When you pass positional `<image>` args together with
`--include-disabled`, the override is **scoped to those names**. Other
disabled images stay filtered out — important because widening the
working set globally would surface unrelated dep errors (e.g., a
disabled image with remote layers that haven't been fetched yet).
`charly box build --include-disabled` (no positional args) widens
globally; `charly box build immich --include-disabled` only relaxes the
gate for `immich`.

The flag flows from `BuildCmd.IncludeDisabled` → `ResolveOpts.IncludeDisabled`
+ `ResolveOpts.IncludeDisabledNames` → `ResolveAllBox` (a `*Config` method)
→ `ResolveBox`. The `shouldIncludeDisabled(name)` helper centralises the
scoping rule. Sibling commands `charly box inspect <name> --include-disabled`
and `charly box validate --include-disabled` accept the same flag for
diagnostic / validation work on disabled entries. See `/charly-build:inspect`
and `/charly-build:validate`.

## Build Config (the embedded build vocabulary)

Containerfile generation is driven by the build vocabulary (`distro:` / `builder:` / `init:` /
`resource:`). The DEFAULT vocabulary is EMBEDDED in the `charly` binary (`charly/charly.yml`,
`//go:embed` — the single embedded default config, plain node-form YAML parsed by the
same unified loader as any project `charly.yml`) and
merged as the lowest-priority base — a project needs
no build vocabulary of its own. A project EXTENDS or OVERRIDES it by declaring its own
`distro:`/`builder:`/`init:`/`resource:` entries inline in `charly.yml` or in an imported
vocabulary file (project-wins; the former `defaults.format_config:` field is gone).

The embedded `charly/charly.yml`'s build vocabulary has three top-level sections:

- **`distro:`** — Per-distro bootstrap commands (package manager setup, cache mounts, repo management), optional `base_user:` declaration (what uid-1000 account the upstream base image ships), and package format templates (how `rpm:`, `pac:`, `deb:` sections in charly.yml become `RUN` steps). Each format has `install`, `repos`, `copr`, `modules`, and `options` templates.
- **`builder:`** — Multi-stage builder patterns (pixi, npm, cargo, aur). Each builder has `build_stage` and `copy_stage` templates that generate the appropriate `FROM builder AS ...` and `COPY --from=...` steps.
- **`init:`** — Init system definitions (supervisord, systemd) including detection rules, fragment templates, entrypoint commands, and service management commands. Optional — images can omit this if they don't need an init system.
- **`resource:`** — Exclusive host-resource vocabulary: maps an arbitration token (the name used by a deploy/bed's `requires_exclusive:` and a holder's `preemptible.holds:`) to an optional hardware selector. A `gpu:` selector (`resource: {nvidia-gpu: {gpu: {vendor: "0x10de"}}}`) lets `charly vm create` AUTO-ALLOCATE the matching PCI `<hostdev>` for a GPU-requiring VM (detect → persist into the per-host `instance.yml` → inject) — or FAIL HARD when no matching card is present. The selector lives in YAML, never hardcoded in Go: adding a resource is a config edit. Optional + additive (configs without it load unchanged). See `/charly-internals:disposable` "resource-arbitration axis", `/charly-vm:vm` "GPU passthrough", `/charly-core:deploy` `requires_exclusive`.

All sections use Go `text/template` syntax with access to layer config data. Source: `sdk/buildkit/format_config.go` (the distro/builder type aliases, including `DistroDef = spec.ResolvedDistro` with its `BaseUser`), `sdk/buildkit/init_config.go` (`InitConfig = spec.InitConfig`), `sdk/buildkit/render.go` (`RenderTemplate` — the former `charly/format_config.go`/`init_config.go`/`format_template.go` are DELETED, K-wave 2).

### `base_user:` — declaring a pre-existing base-image account

`base_user:` handles upstream base images that ship a pre-existing uid-1000 account (e.g. Ubuntu 24.04's `ubuntu:ubuntu`). Declare it under the distro when the upstream base image ships a uid-1000 account; leave it out otherwise.

```yaml
distro:
  ubuntu:
    inherits: debian
    base_user:
      name: ubuntu
      uid: 1000
      gid: 1000
      home: /home/ubuntu
    # ... bootstrap inherited from debian
```

All four fields (`name`, `uid`, `gid`, `home`) are required when the block is present. Inherited across distro inheritance chains — if the child has no `base_user:` but the parent does, the child inherits it (see `ResolveInherits` in `spec/spec/distro_config_methods.go`).

Consumed by the `user_policy:` reconciliation in `sdk/buildkit/config_resolve.go:ResolveBox` — see `/charly-image:image` "user_policy" for the three-value policy (`auto` / `adopt` / `create`) and the decision matrix.

No `base_user:` currently declared for Fedora, Arch, or Debian (their canonical base images ship no pre-existing uid-1000 account). Add one in your project's `charly.yml` build-vocabulary override if you're basing on a distro-cloud variant that DOES ship one (e.g. `debian:13-cloud`).

### `version:` — the canonical distro version (for VM per-version reach)

Each distro may declare a canonical `version:` (`distro.debian.version: "13"`, `distro.ubuntu.version: "24.04"`, `distro.fedora.version: "43"`; arch/cachyos are rolling and omit it). It is the single source for synthesizing the most-specific-first tag chain `[<distro>:<version>, <distro>]` on a target that carries only a bare distro name — a `target: vm` deploy, where no image-authored `distro:` tag supplies the version. `syntheticVmBox` + the `distroTagChain` helper consume it so a VM deploy of an ubuntu guest reaches per-version `distro:` sections (e.g. `ubuntu-24.04`) exactly like an image build does. Inherited child-wins via `resolveInherits` (cachyos inherits arch → stays version-less). Image builds carry the version in their own `charly.yml` `distro:` tags and don't need it. See `/charly-image:layer` "Package Surface" and `/charly-internals:install-plan`.

### The `builder:` name in two places

`builder:` appears in both the embedded build vocabulary (top-level section) and `charly.yml` (per-image map, plus `defaults.builder`). They share the name on purpose — both maps key on the same slot (the build-type name, e.g. `pixi`, `npm`, `cargo`, `aur`):

- the embedded build vocabulary's `builder.pixi` — **definition**: detection rules, stage template, cache mounts for the pixi builder.
- `charly.yml` `builder.pixi` — **selection**: which image (e.g. `fedora-builder`) to use as the pixi builder for this image.

An image's effective builder map resolves as: `box.builder[type]` → `base_box.builder[type]` → `defaults.builder[type]` → `""`. Self-references (a builder image pointing at itself) are filtered automatically.

### Why one file, not three

The embedded build vocabulary was unified from three former files (`distro.yml` + `builder.yml` + `init.yml`) because they were always resolved together through the same `format_config:` ref. One file → one loader (`LoadBuildConfigForBox`) → three in-memory configs (`DistroConfig` / `BuilderConfig` / `InitConfig`) — the internal split is preserved, only the YAML surface is unified. The `init:` section is optional (absent = no init system); `distro:` and `builder:` are required.

## Containerfile Generation

`charly box build` runs `charly box generate` internally. You can also run it standalone to inspect generated Containerfiles:

```bash
charly box generate                          # Write .build/ (Containerfiles)
charly box generate --tag v1.0.0             # Override CalVer tag
cat .build/my-image/Containerfile    # Inspect generated output
```

## Build Flow

1. Run `charly box generate` internally (produces Containerfiles in `.build/`)
2. Resolve runtime config (there is no build-engine selection: BuildKit is the builder)
3. Resolve image build order (dependency ordering, grouped by level)
4. Filter to requested images (and their base dependencies)
5. For each level: build images in parallel (up to `--jobs` concurrent, default 4)
6. After all builds: `charly box merge --all` (if `merge.auto` enabled, skipped for `--push`)

## Parallelism: `--jobs` vs `--stage-parallelism`

charly exposes **two** parallelism knobs with distinct meanings:

| Flag | Env var | `defaults:` key | What it controls |
|---|---|---|---|
| `--jobs N` | `CHARLY_BUILD_JOBS` | `jobs` | **Outer** concurrency: how many charly-level images to build in parallel within a DAG level (e.g., when `charly box build` rebuilds the whole graph). |
| `--stage-parallelism N` | `CHARLY_STAGE_PARALLELISM` | `stage_parallelism` (+ `stage_parallelism_cap`) | **Inner** concurrency: how many stages of a *single* multi-stage build run concurrently. It becomes the BuildKit daemon's `[worker.oci] max-parallelism`, applied when the daemon starts. |

Both knobs are **config-driven**: precedence is **CLI flag → env → `defaults:`
in `charly.yml` → built-in fallback** (4). The inner auto value (when
`--stage-parallelism` / `CHARLY_STAGE_PARALLELISM` / `defaults.stage_parallelism` are all unset) is
CPU-proportional, capped at `defaults.stage_parallelism_cap`: `min(NCPU, cap)`.
The cap is the operative ceiling, and it is the project's own to set. (The 20-run
race gate required before raising it on a new engine version is in `CHANGELOG/`.)

```yaml
# charly.yml — defaults: block
defaults:
  jobs: 4              # outer: images per DAG level
  stage_parallelism: 0       # inner: 0 = auto = min(NCPU, stage_parallelism_cap)
  stage_parallelism_cap: 8   # ceiling for the auto calc
```

```bash
charly box build <image> --stage-parallelism 16       # fully parallel stages (override)
CHARLY_STAGE_PARALLELISM=8 charly update <image> --build  # via env
charly box build <image> --stage-parallelism 1        # fully serialised, worst-case debugging
```

Source: `candy/plugin-build/resolve.go` (`resolveDriveStageParallelism`, feeding
`buildkit.ResolveStageParallelism`; the host-resolved `StageParallelism` value is the
`MaxParallelism` the drive hands the daemon in `candy/plugin-build/buildkit.go`).
Covered by `buildkit/build_helpers_test.go` (`TestResolveStageParallelism`). The outer `--jobs` knob and the inner `--stage-parallelism`
are separate fields so the two semantics don't get conflated.

## Build-context excludes (`defaults.context_ignore`)

`charly box generate` writes BOTH `.containerignore` and `.dockerignore` at the
project root from a single source: a built-in baseline (`.git`, `bin`, `charly`,
`*.md`, plus editor/python/node cache-bust globs) **plus** every entry in
`defaults.context_ignore`. Both files are generated artifacts (gitignored) — do
not hand-edit them; add excludes to `defaults.context_ignore` instead.

```yaml
# charly.yml — defaults: block
defaults:
  context_ignore:        # heavy dirs never COPYed by any Containerfile
    - image
    - .check
    - output
    - pkg
    - tests
```

Why it matters: the build context tar is streamed to the engine on **every**
build regardless of cache state, so excluding large never-COPYed directories is
the dominant warm-rebuild win. BuildKit's dockerfile frontend reads `.dockerignore`;
`.containerignore` is the engine-native name kept for engine-native builds — emitting both
keeps the two readers in lockstep. Only add a
directory you've confirmed no Containerfile COPY/ADDs from (generated
Containerfiles COPY only from `candy/`, `templates/`, `.build/`). Source:
`candy/plugin-build/host_prep.go:writeContextIgnore` + `charly/host_build_buildengine.go`
(`baselineContextIgnore`, the embedded `context_ignore_baseline` directive the
`buildengine-context-ignore-baseline` seam serves; the former `charly/generate.go` is
DELETED, K-wave 2).

## Image-tag retention (`defaults.keep_images`)

After `charly box build` (push runs excluded), charly prunes **tag rows** within each
`ai.opencharly.box` group, keeping the newest `defaults.keep_images` **distinct
images** and at most that same number of tags of each — except for rows the engine
refuses to date, which are never removed **however many tags their image wears**, so
the per-image cap binds datable rows only. A *distinct image* is a distinct image
ID — every tag pointing at one ID is one image, however many tags it wears — so with
`keep_images: 3`, four images carrying one tag each lose the oldest, while one image
carrying six datable tags keeps three of them.

**Which verb reclaims what.** The post-build pass cannot remove an image's LAST tag,
so it never reclaims a *distinct image* — only surplus tags of images that wear
several. `charly box build` holds the build-activity lock across its own prune, so
that prune always observes a live build and the last-tag guard always engages. The
distinct-image budget above is therefore enforced by `charly clean`, and a project
that only ever builds will accumulate distinct images past `keep_images` without any
surface reporting it.

**Tag rows, not "CalVer tags":** a row is exempted from removal only when it has
NEITHER a datable `ai.opencharly.version` label NOR a datable `:YYYY.DDD.HHMM` tag
(`retention.go`'s guard is an AND). A bed build has a datable label and a non-CalVer
tag, so it is a normal removal candidate rather than an excluded one.

**Preview before enabling it**, with the value you are considering rather than one you
have already committed to:

```bash
charly clean --keep 3 --images --dry-run   # what keep_images: 3 WOULD remove
```

`--keep` supplies the budget on the command line, so this answers the question while
`defaults.keep_images` is still absent — with the key unset, retention is disabled and a
plain `charly clean --dry-run` correctly shows nothing, which is not the preview you
want. The count that surprises people is **tag rows, not images**.

**Ordering is owned by `/charly-core:clean`**, which documents the retention engine
itself — the full comparator chain, which key decides which ordinal, and why the build
tag cannot serve as the image recency key. The two consequences that matter here:
**between distinct images**, the datable `ai.opencharly.version` label decides when the
two differ, and creation time decides when they tie — which is the usual case, because
that label is **declared, not derived from content**: it is the box's own `version:`,
else the highest candy `version:` across its whole candy set, else the base image's
(`sdk/deploykit/effective_version.go`). So DISTINCT images built from DIFFERENT trees
carry the SAME label until somebody bumps one of those fields, and creation time is
what actually orders them; **within one image's tag rows**, the
`:YYYY.DDD.HHMM` tag decides, because those rows share everything else. Do not carry
*"creation time, not the tag"* across to the tag budget — it is true of the first and
false of the second.

Images referenced by a running container are skipped, and removal runs without `-f` as a
backstop (`charly clean` owns the prune; never call the engine's prune directly).

```yaml
# charly.yml — defaults:
defaults:
  keep_images: 3   # newest distinct images per box, and at most 3 tags of each;
                   # 0 (or absent) disables
```

This stops the iterative-build tag accumulation that otherwise reclaims
nothing. Run it on demand (and clear a backlog) with `charly clean` — see
`/charly-core:clean` for the full retention surface (images + check runs + makepkg).

## Build Cache

| Mode | Backend | Use Case |
|------|---------|----------|
| `image` | `<registry>/<image>` | Read-only cache from registry image (default) |
| `registry` | `<registry>/cache:<image>` | Production CI/CD (read + write) |
| `gha` | GitHub Actions cache | CI builds on GitHub Actions |
| `none` | No cache | Same as `--no-cache` |

```bash
charly box build --cache registry my-image        # Read+write registry cache
charly box build --cache image my-image          # Read-only from registry image
CHARLY_BUILD_CACHE=registry charly box build         # Via environment variable
```

The default cache mode is also config-driven: precedence is `--cache` →
`CHARLY_BUILD_CACHE` → `defaults.cache` in `charly.yml` → auto (`image` for local
builds, `registry` for `--push`). Set `defaults.cache: image` to make the
read-only registry cache the project default without per-invocation flags.

## CalVer-only tagging (no `:latest`)

`charly box build` tags every image with exactly one tag — its CalVer
(e.g. `ghcr.io/opencharly/fedora-supervisord:2026.114.1022`). charly does
**not** emit a `:latest` tag, ever. Short-name resolution (in
`sdk/kit/local_image.go`, `kit.ResolveLocalImageRef` — moved from
`charly/local_image.go` in P12a) picks the newest CalVer for a given short name
via the `ai.opencharly.box=<short>` + `ai.opencharly.version=<calver>`
OCI labels. The CLI accepts an explicit `--tag <calver>` for pinning;
an empty `--tag` resolves to newest-local automatically.

Rationale:
1. **Nothing can pin a stale float tag.** charly never emits `:latest`, so a build can never
   resolve to a float tag a previous build left behind.
2. **No "ambiguous short name" errors under multiple tags.** Previously
   the resolver listed every `:latest` + `:<calver>` tag of the same
   image as competing candidates; the CalVer sort picks the newest
   one deterministically.
3. **One-tag-per-build keeps registry + local storage lean.** A float
   tag is a permanent nameable handle you have to garbage-collect;
   a CalVer tag is self-describing and rotates naturally.

Any stray `:latest` tag in local storage is never refreshed by a new
build; prune it with `charly clean` if you want it gone.

## Cache Efficiency

### Core invariant — cache hits fully when nothing has changed

**Running `charly box build <image>` with no source changes completes in seconds — every RUN/COPY/LABEL step hits the cache.** This is the invariant the rest of this section builds on. Cache-miss only happens when something in the build input genuinely changes; a rebuild you triggered "just to be sure" is free.

Concretely: the cache is keyed by `(parent-image-SHA, instruction-text, COPY-source-content)`. The parent-image SHA resolves from the FROM reference at build time — a fresh CalVer tag that still points at the same image SHA keeps the subsequent RUN/COPY steps cached. Only when the underlying image SHA changes (because that upstream was rebuilt with changed content) does cache-miss cascade to the downstream steps.

### What legitimately invalidates cache

Three kinds of source changes are real cache invalidators — if you see a long rebuild, one of these is the cause:

1. **Layer source file content changed.** Editing a file under `candy/<name>/` — the canonical case is a local-source charly artifact being rebuilt by the R9 source build (the worktree-local CalVer-stamped build, see `/charly-internals:go` R9) after a Go source edit — changes the scratch stage's content hash, which invalidates `COPY --from=<layer>` and everything downstream that depends on it.
2. **Package list / task text changed.** Adding/removing an rpm/deb/pac entry or editing a `command:` body changes the RUN instruction text emitted for that layer, invalidating cache from that RUN onward.
3. **Upstream image content changed.** If a base image (external like `fedora` or internal like `fedora-supervisord`) has different content from the last cached build, the FROM step resolves to a new SHA and downstream RUN/COPY steps all cache-miss. This cascades through the dependency graph — rebuilding `fedora-supervisord` forces its children to re-run from the `FROM fedora-supervisord` step.

### What does NOT invalidate cache

- **CalVer tag shifts alone.** The Containerfile emits `ARG BASE_IMAGE=<registry>/<name>:<calver>` with a fresh CalVer on every generate. That ARG default appears in the Containerfile text but is not part of the cache key for subsequent RUN/COPY steps — BuildKit resolves the FROM to an image digest first, and caches downstream steps off that digest. If the SHA is unchanged, cache hits. The cache cost comes from content changes in the layer itself, not the tag.
- **Baked `plan:` edits.** LABEL directives are emitted last in every final stage (after the last USER). Editing a baked step (a `check:`/`agent-check:` step, or a runtime-context `run:` step) — a common layer mutation — only re-runs the final LABEL block (~2 seconds on a 138-step stack like `immich-ml`). (Editing a build/deploy-context `run:` step changes the install timeline, so it re-runs that RUN/COPY instead.) See `/charly-internals:generate-source` for the rationale.
- **Re-running `charly box build` without source changes.** Fully cached; seconds to complete.

### `write:` vs `copy:` — cache granularity

- `write:` steps use `StageInlineContent` (content-addressed staging under `.build/<image>/_inline/<layer>/<sha256>/`). Editing a `write: content:` block changes only that single COPY layer's cache key — siblings in the same layer keep their cache.
- `copy:` steps reference files from the layer directory. Editing any file under `candy/<name>/` changes the WHOLE scratch stage's content hash (since `COPY candy/<name>/ /` is one instruction), invalidating every downstream `COPY --from=<layer>` step.

### Rule of thumb for rebuild cost

| Edit | Cost |
|------|------|
| `charly box build` with zero source changes | Seconds — every step cache-hits |
| A baked `check:` step / label entry | ~2 sec (LABEL re-emit only) |
| A `write:` step's content | Just that single content-addressed COPY layer |
| A `copy:` source file's content | Rebuild from that layer's COPY onward + downstream |
| A `command:` / `download:` run step body | Rebuild from that RUN onward + downstream |
| A package added/removed in `rpm:`/`deb:`/`pac:` | Rebuild from the install RUN onward + downstream |
| a local-source charly artifact rebuilt (R9 source build) | Rebuild the `charly` layer + every image that includes it |
| An upstream image got content-changed and rebuilt | Rebuild from the FROM step onward in every descendant |

## Build Flow Details

**Internal base images** use exact CalVer tags in Containerfiles (`FROM ghcr.io/opencharly/fedora:2026.46.1415`), so each image names the precise version of its parent. **The builder does not resolve that tag through a local-image lookup**: BuildKit's OCI worker has no local-first resolution.

Every build's export is NAMED — `io.containerd.image.name=<FullTag>` plus `org.opencontainers.image.created` on the manifest — and written to the **charly image store**, one engine-neutral OCI layout at `$XDG_DATA_HOME/charly/images` (`CHARLY_IMAGE_STORE_DIR` relocates it for a disposable target). The store is the build's canonical artefact, and it is what a dependent box's parent is solved from: when `ARG BASE_IMAGE=<ref>` names an image already in the store, the drive hands the store to the daemon as an `--oci-layout` and binds that ref to the parent's manifest digest as a named context, so the parent is read **from the store and not from the registry** (`opencharly/opencharly#481`). The registry remains the fallback when the parent is absent locally, and the buildkitd mirrors `ghcr.io/opencharly` to charly's local OCI registry, so any base the store does not hold resolves internally rather than against the upstream registry (`opencharly/charly#937`).

The image is then materialised into the configured RUN engine's store with a plain `load` — **no `tag` step**: the naming annotation is what applies the reference, and the drive verifies the reference resolves before the build is reported successful. Every later read of it — capability labels, deploy, status, inline merge, `charly clean` — resolves it through that engine.

## Push Mode

BuildKit's image exporter is the ONE push path — `--output type=image,name=<ref>,push=true` —
and the only one that can push the multi-platform image a `--push` build produces. It
replaces the engine-native push drivers — no per-engine push path remains.

Source: `candy/plugin-build/buildkit.go` (`pushOutput`).

## Layer Merging

Post-build optimization that merges consecutive small layers:

```bash
charly box merge <image> --dry-run         # Preview what would be merged
charly box merge <image>                   # Merge small layers
charly box merge --all                     # Merge all images with merge.auto enabled
charly box merge <image> --max-mb 512      # Custom per-layer threshold
charly box merge <image> --max-total-mb 4096  # Custom total image size limit
```

Configure in charly.yml:

```yaml
defaults:
  merge:
    auto: true        # Auto-merge after builds
    max_mb: 128       # Max merged layer size (MB)
    max_total_mb: 0   # Max total image size for merge (0 = no limit)
```

CLI flags `--max-mb` and `--max-total-mb` override `charly.yml`. `auto` is only used by `charly box merge --all` to select which images to merge; `charly box merge <image>` always merges regardless. `max_total_mb` controls whether large images skip merging entirely (the merge process decompresses layers in memory). Set to `0` to disable (default), or a positive value like `2048` to cap on low-memory CI runners.

### Algorithm

1. Load image from engine via `<engine> save` -> `tarball.ImageFromPath()`
2. Get compressed sizes via `layer.Size()`
3. Group consecutive layers into groups totaling <= `max_mb`
4. Single-layer "groups" are kept as-is (need 2+ layers to merge)
5. For each merge group: read uncompressed tarballs, deduplicate entries by path (last writer wins), write combined tar into a single new layer
6. Reconstruct image with `mutate.Append()`, preserving OCI history alignment (empty-layer entries for ENV/USER/EXPOSE kept in correct positions)
7. Save via `tarball.WriteToFile()` -> `<engine> load`

Merge is idempotent -- running again after merging shows all layers as `[keep]`. The engine itself lives out-of-process in `candy/plugin-oci/merge.go` (`runMerge`/`mergeImageRef`, the P14a cutover); `charly box merge`'s CLI surface is `candy/plugin-box/merge_cmd.go` (P14 — relocated OUT of charly core, which now carries zero merge-specific code).

### Inline Merge

Images are merged immediately after building, before their children are built. Child images inherit a merged (fewer-layer) base, producing smaller final images. Both local and push builds merge inline. `candy/plugin-build/drive.go`'s `mergeBox` handles this -- it checks `MergeAuto` on the build-resolve model and asks `verb:oci` to run the merge via `InvokeProvider` if enabled.

For filtered builds (`charly box build <image>`), only the built images are merged. For full builds (`charly box build`), merge runs after each dependency level completes.

## Engine Configuration

```bash
charly settings set engine.run docker     # or podman / containerd — which engine RUNS containers
```

`engine.run` accepts `auto` (detect podman, then docker), `podman`, `docker`, and
`containerd`; `containerd` is opt-in and driven through the `nerdctl` CLI (`nerdctl` is the
binary, not an engine value). See `/charly-build:settings`.

**There is no `engine.build`**: the image builder is BuildKit, which is not a container
engine — it cannot run a container and therefore cannot be selected like one. `engine.run`
is unaffected and still selectable.

## Host Prerequisites (First Time)

Building images needs the `charly` binary and a container engine — `podman`, `docker`, or `containerd`, whichever `engine.run` names (BuildKit builds the image; the drive bridges it into
that engine's store). Nothing else, and no project checkout: `charly box build` runs against the `charly.yml` in the current directory, one started by `charly box new project <dir>` or read from a published project with `--repo <owner>/<repo>`.

Install `charly` as a distro-native package with your OWN package manager (`pacman -U` / `dnf install` / `apt install` / `apk add`) — the `charly generate-packages` plugin (nFPM) builds the `.pkg.tar.zst`/`.rpm`/`.deb`/`.apk`/`.ipk` published to the per-distro package repos. Obtaining the binary is covered end to end at [Install](https://opencharly.ai/start/install/); it is the one page that describes building from a source tree.

```bash
charly box build   # Generate + build + merge all images
```

Multi-platform `--push` builds additionally need a Docker buildx builder that can target more than the host platform — a Docker-side prerequisite, not a charly one. Single-platform local builds need no builder setup.

## Common Workflows

### Build a Single Image

```bash
charly box build my-app
```

### Rebuild After Layer Changes

```bash
charly box build my-app
# Only changed layers are rebuilt (Docker layer cache)
```

### Push to Registry

```bash
# Authenticate first
docker login ghcr.io
# or: podman login ghcr.io

charly box build --push
```

## Troubleshooting

### "charly not found"

Install a distro-native package (built by the `charly generate-packages` plugin and published to the per-distro package repos) with your OWN package manager (`pacman -U`/`dnf install`/`apt install`/`apk add`). If it is installed but not resolving, `$PATH` is the usual cause — `which charly` reports which copy wins.

### Build Fails with Missing Base

Build base images first. `charly box build` handles dependency ordering automatically, but if building a single image, its base must already exist.

### Cache Miss

First build on a new machine won't have cache. Use `--cache registry` to pull from registry cache if available.

### RPM Conflict: ffmpeg-free vs negativo17

If a build fails with `conflicting requests` involving `libavcodec-free` vs `libavcodec` (epoch 1), the candy is trying to install `ffmpeg-free` (Fedora) in an image that has negativo17's `ffmpeg-libs` (via cuda candy). Fix: change `ffmpeg-free` to `ffmpeg` in the candy's `rpm.packages` and add the `fedora-multimedia` repo from negativo17. See the `immich` candy for the correct pattern.

### YAML Unmarshal Error on charly.yml

If you see `cannot unmarshal !!str ... into int` or similar YAML parsing errors on layer fields, the `charly` binary is older than the schema the file is stamped at. `charly version` prints its CalVer; upgrade to a newer published package and re-run `charly box validate`.

### Stale `charly` binary produces stale Containerfiles

Beyond the YAML-unmarshal symptom above, a stale `charly` binary can produce *syntactically valid but outdated* Containerfile output — e.g. emitting an old broken form of a template a later release already fixed. Symptom: a build fails on a step whose generated shell does not match what the current release documents. Quick diagnostic: `charly version` against the CalVer of the release that carries the fix; if the binary predates it, upgrade to a newer published package and rebuild.

If the symptom persists after upgrading, the copy on `$PATH` may not be the one you just installed — `which charly` reports which wins.

### Cache-mount corruption (pixi tzdata et al.)

Cache mounts are declared as `--mount=type=cache,id=charly-<path>,dst=<path>,sharing=locked` (pixi, npm, cargo,
rattler and dnf all use one — `buildkit.SharedCacheMount` emits the format). Their **id is stable on purpose**:
that is what makes the cache survive layer-hash churn, which is the entire reason cache mounts exist
(`candy/plugin-build/cache_mount_test.go`). A cache mount is therefore not the build cache: `charly box build
--no-cache` disables the build cache for that build, and the mount's contents persist regardless. A disk-full event mid-download can leave a partial package in the cache, and every subsequent build re-hits the same broken file:

```
error: × failed to link tzdata-2025c-hc9c84f9_1.conda
  ├─▶ failed to copy file ... zoneinfo/<Region>/<City>: No such file or directory
```

Options, in order of least to most disruptive:

1. **Just retry the build.** Often the cache clears itself once the partial package finishes downloading or gets superseded by a checksum mismatch.
2. **Bump a content-hash on the builder layer** (e.g. `echo "" >> candy/pixi/charly.yml`) to evict the cache mount associated with that build-stage's hash, then revert the cosmetic change. Same workaround as the scratch-stage cache issue above.
3. **The engine's `image prune -af` is NOT the right hammer** — use `charly clean`; see next note.

### Engine-level `image prune -a -f` caveat — removes tagged-but-idle images

An engine-level `image prune -a -f` deletes tagged images that are **not referenced by a running container**. That means the image you just built via `charly box build` — and haven't started as a container yet — is eligible for prune. Observed sequence: `charly box build` → an engine-level `image prune -af` → a manual rootful reload fails with `image not known`. Protect the fresh build by starting a container or use `prune` with explicit filters instead.

### Forcing a rebuild of one layer's scratch stage

A layer's scratch stage is keyed by the layer directory's own content — the cache key is
`(parent-image-SHA, instruction-text, COPY-source-content)` (see "Cache Efficiency" above) — so editing any
file under `candy/<name>/` rebuilds that stage and everything downstream of it. To force that rebuild without
changing the file that matters, bump the layer's `charly.yml`:

```bash
echo "" >> candy/labwc/charly.yml          # bump the layer's content hash
charly box build selkies-desktop            # rebuilds the labwc scratch stage
git diff candy/labwc/charly.yml             # the appended line, and nothing else
```

Remove the added line to revert it. Do not reach for `git checkout -- <file>` to undo it: that restores the
file from the index and discards any other uncommitted work it holds.

## Project directory override

`charly box build` (like every build-mode command) resolves `charly.yml` via `os.Getwd()`. Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>` — honoured before Kong dispatch. See `/charly-image:image` "Project directory resolution" for the canonical reference and the `charly mcp serve` use case. Typical use: building from an `charly mcp serve` MCP tool where the container cwd doesn't hold the project.

## Cross-References

### `charly box` family siblings

- `/charly-image:image` -- Family overview + charly.yml composition reference
- `/charly-build:generate` -- Containerfile generation (called internally; stale `:latest` FROM lives there)
- `/charly-build:inspect` -- Inspect resolved image config before building
- `/charly-build:list` -- Enumerate images, layers, build targets
- `/charly-build:merge` -- Post-build layer consolidation (runs inline after each build level)
- `/charly-build:new` -- Scaffold a new candy directory before adding to `charly.yml`
- `/charly-build:pull` -- Pull prebuilt images; orthogonal to building (use for downstream deploy-mode commands)
- `/charly-build:validate` -- Validate `charly.yml` + layers before building

### Related skills

- `/charly-image:layer` -- Layer definitions that get built
- `/charly-check:check` -- Tests are embedded as `ai.opencharly.description` OCI label at build time; LABEL-at-end optimization (see Cache Efficiency above) makes test edits cheap.
- `/charly-core:charly-update` -- `charly update <image> --build` invokes `BuildCmd.Run` and picks up the same `--jobs` cap and stale-`:latest` caveat
- `/charly-vm:vm` -- Building bootc disk images (`charly vm build`)
- `/charly-core:charly-config` -- Engine configuration
- `/charly-automation:enc` -- Encrypted-volume restart path interacts with the `--build` flow (`charly config mount` short-circuit means builds can restart services without touching the keyring)

## When to Use This Skill

**MUST be invoked** when the task involves building images, pushing to registries, build caches, or layer merging. Invoke this skill BEFORE reading source code or launching Explore agents.

**Workflow position:** Typically first in a lifecycle chain.
Next step: `/charly-core:deploy` (quadlet setup, tunnels) → `/charly-core:service` (start and manage).

## Related skills

- `/charly-internals:capabilities` — OCI labels emitted during the build stage; `CapabilityLabelMap` completeness check

Live-deploy verification: see /charly-check:check (the 11 Testing Standards) and /charly-internals:disposable.
