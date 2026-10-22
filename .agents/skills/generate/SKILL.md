---
name: generate
description: |-
  Containerfile generation from charly.yml and layers — `charly box generate`, .build/ contents, the task-verb emission pipeline. Use before any of those.
---

# charly box generate -- Containerfile Generation

Invoked as `charly box generate`. See `/charly-image:image` for the family overview.

## Overview

Parses `charly.yml`, scans `candy/`, resolves the dependency graph, and emits all build artifacts into the `.build/` directory. Called internally by `charly box build` but can be run standalone to inspect generated output before a build.

The generator is **config-driven** — distro format templates, builder stage templates, and init system fragments all come from the build vocabulary embedded in the `charly` binary at `charly/charly.yml` (three top-level sections: `distro:`, `builder:`, `init:`) — and **declarative per-task** for install logic — each task verb (see `/charly-image:layer`) has a dedicated emitter that writes the right Containerfile directive.

**Build dispatch**: `charly box generate` does not construct a generator host-side at all (the former `NewGenerator` is DELETED) — it is served by the COMPILED-IN `command:box` plugin (candy/plugin-box, nested under the `box` grammar group), whose generate handler invokes the peer `build:generate` plugin (candy/plugin-build) over the F10 reverse channel (InvokeProvider). The plugin's `runBoxGenerate` → `resolveBuildEngine(..., generateOnly=true)` (candy/plugin-build/resolve.go) runs the ENTIRE build RESOLVE plugin-side — loader, vocab, scan, connect, validate, box resolve, intermediates, order, effective versions, host-fs prep, render-seam-cache prep, render-prep, user-context, and envelope projection — and returns the resolved-project envelope (`spec.BuildResolveReply.ResolvedProject`); `generateOnly` is a PLUGIN-SIDE parameter that returns after the envelope projection (no engine/order drive-model). The host is reached ONLY for thin `buildengine-*` shards (`buildengine-scan-local`, `buildengine-connect-plugins`, and `buildengine-prep` — the render-seam-floor `renderGenCache` populate, which returns an EMPTY map; the host-fs PREP itself moved plugin-side); the envelope + drive-model come from `resolveBuildEngine`'s reply, NOT from `HostBuild("buildengine-prep")` (that host leg takes `spec.ResolvedProjectRequest`, which has no `GenerateOnly` field). The candy renders the Containerfiles itself via `sdk/deploykit.Generator` over the envelope + `HostBuild("render-seam")` (#67 — the host no longer renders); the written paths are returned — no podman (generate builds nothing). `charly box build` uses the SAME resolve pipeline (`runBoxBuild` → `resolveBuildEngine(..., generateOnly=false)`, which additionally computes the drive-model), then the candy additionally OWNS the BuildKit DRIVE (the `buildctl` argv / push / inline-merge) IN the candy. See `/charly-internals:plugin` (the `build` provider class + the in-proc reverse channel) and `/charly-build:build`.

**Build-mode emission is `WriteCandySteps` → `EmitTasks`** in `sdk/deploykit` (`deploykit.Generator`, relocated from `charly/generate.go` in #67, driven by `candy/plugin-build` over the resolved-project envelope + `HostBuild("render-seam")`; the `charly/generate.go` top-level `Generate`/`writeCandySteps` orchestrator is DELETED). the pod-overlay deploy path reaches `deploykit.Generator.EmitTasks` DIRECTLY — `charly/tasks.go` holds no shim and cannot hold one (P16 import purity forbids `charly/` importing the sdk); that path stays for the pod-overlay deploy path (the candy `plugin-deploy-pod` constructs `deploykit.OCITarget` + renders via `deploykit.NewRenderGeneratorFromProject`, the per-step fragment over the `HostBuild("step-emit","oci-emit-step")` seam). The render walks each layer's ops directly to write Containerfile text — NOT the `InstallPlan` IR. Build shares the package-cascade, shell-snippet, and localpkg compiler helpers with the IR (`ResolveCascadePackages`, `CompileShellSnippetSteps`, `deploykit.RenderLocalPkgImageInstall` — one source of truth, R2, relocated to sdk/deploykit in W3), but the overall walk is the direct generator, not an `EmitTarget`. The `InstallPlan` IR + `deploykit.OCITarget.Emit` is the DEPLOY-mode path: the candy `plugin-deploy-pod`'s overlay-Containerfile synthesis (rendered in its own code via `deploykit.OCITarget` + `deploykit.NewRenderGeneratorFromProject`) when `add_candy:` is set, plus the external execution targets. (The local/vm/kubernetes/android substrates are external out-of-process plugins via `pluginDeployTarget` (S3b, dispatched through `candy/plugin-fleet`'s `Invoke(OpDeployDispatch)`): `deploy:local` (candy/plugin-deploy-local) and `deploy:vm` (candy/plugin-deploy-vm) DO consume the IR — the plugin walks it via `kit.WalkPlans` over the reverse channel, the vm one over the guest `SSHExecutor` so the walk runs inside the guest — while `deploy:kubernetes` (candy/plugin-kube) does NOT, materializing its Kustomize tree PLUGIN-side (`candy/plugin-kube/materialize.go`'s `materializeKustomize`, reaching `candy/plugin-k8sgen` peer-to-peer over `verb:k8sgen`/`OpEmit`).) See `/charly-internals:install-plan` for the IR catalog and `/charly-internals:generate-source` for the Go-level call graph.

**Build-time plugin execution**: the build path connects the project's external (out-of-tree) plugin candies via `loadProjectPlugins` (`charly/host_build_buildengine.go:51`, and `charly/check_cmd.go:70` for the check path), so a `run:` plugin verb — and a plugin builder — is registered + dialable DURING image generation, the SAME loader the deploy/check paths use. The host-side `NewGenerator` that used to perform this connect is DELETED. A builtin is already registered via `init()` and needs no connect; only an external one is host-built + connected here. `EmitTasks` then dispatches a `plugin:` verb step PLACEMENT-AGNOSTICALLY above the registry: a builtin `ProvisionActor` renders an act shell `RUN` in-proc (the zero-JSON fast path), while ANY other resolved provider — an external `grpcProvider`, or a builtin emitting a richer fragment — renders via `invokeOpEmitFragmentOpt` → `Invoke(OpEmit)` (in-proc for a builtin, go-plugin gRPC for an external), and the returned `spec.EmitReply.Fragment` is spliced verbatim into the Containerfile (egress-validated with the rest before write). This is operator-authorized build-time execution of host-built plugin code — a project's composed external plugins run as host code during its image builds. Detail → `/charly-internals:plugin` (placement) + `/charly-internals:generate-source`.

**Build-time BUILDER leg (all via `OpResolve`)**: the builder stage emission is `deploykit.Generator.EmitBuilderStages` / `EmitBuilderArtifacts` / `EmitExternalBuilderStages` / `EmitExternalBuilderArtifacts` (`sdk/deploykit/builders_render.go` — relocated from `charly/generate.go` in #67, driven by `candy/plugin-build` over the envelope + `HostBuild("render-seam")`); the shared builder `OpResolve` helper (`renderSeamCaller.resolveBuilderStage`, `sdk/deploykit/render_generator_from_project.go`) drives both legs (the former `charly/generate.go` is DELETED, K-wave 2). These render the four DETECTION-builders' stages (pixi/npm/aur — selected by a candy's `pixi.toml` / `package.json` / `aur:` section; cargo is INLINE) by `Invoke(OpResolve)`ing their plugins — NOT from an in-core `builders.<name>` vocabulary (C10 moved the stage templates into the plugins' `kit.BuilderResolve`). For each detected `(candy, builder)`, `EmitBuilderStages` connects the plugin on-demand (`ensureBuildersConnected`) and calls the shared `resolveBuilderStage` → `prov.Invoke(OpResolve)` with a `spec.BuilderResolveInput` (the host-computed render context — builder ref, stage name, copy src, uid/gid/home, detected manifest/lockfile/build-script, aur packages/options, PRE-RENDERED cache-mount flags) as `op.Params` + a `spec.BuildEnv` as `op.Env`; the returned `spec.BuilderResolveReply.Stage` is written verbatim PRE-main-FROM (cached per `(candy, builder)`), `EmitBuilderArtifacts` writes the cached `CopyArtifacts` + once-per-builder `CopyBinary` POST-main-FROM, and cargo's `InlineFragment` splices in `WriteCandySteps` (deploykit). A candy that instead selects an OUT-OF-TREE builder via its `external_builder: <word>` field gets its multi-stage emitted by `EmitExternalBuilderStages` / `EmitExternalBuilderArtifacts` (run right after their detection counterparts), sharing the SAME `resolveBuilderStage` but sending a MINIMAL input (candy name only): the word resolves through `providerRegistry.ResolveBuilder` to an EXTERNAL `*grpcProvider`, and `EmitExternalBuilderStages` (the BUILDER-leg analogue of `invokeOpEmitFragmentOpt`) requires a non-empty `Stage`. An empty `Stage` (or empty cargo `InlineFragment`), an unresolvable word, or an Invoke error fails LOUDLY (R3) — never a silently-dropped builder stage. Every half is egress-validated with the rest of the Containerfile before write. Detail → `/charly-internals:plugin` (placement) + `/charly-internals:generate-source`.

**Three-phase templates**: the embedded build vocabulary's format (`formats.<fmt>`) and builder (`builders.<name>`) definitions carry a `phases: { prepare, install, cleanup }.{ container, host }` structure. The generator reads `phases.install.container` as the single source for build-mode package installation. The `host:` cell is consumed only when a `SystemPackagesStep` renders on a host venue (`RenderHostPackageCommand`) — for the external `local:` AND `vm:` deploys the plugin drives the `SystemPackages` host render via the host's `RunHostStep` channel — never by the generator.

## Quick Reference

| Action | Command | Description |
|---|---|---|
| Generate all | `charly box generate` | Generate Containerfiles for all enabled images |
| With tag | `charly box generate --tag TAG` | Override the image tag |

`charly box generate` takes an OPTIONAL positional box list (`generate [<boxes> ...]`; the sentinel `all`, or no argument, means every enabled image in `charly.yml`). To inspect a single image's output, run `charly box generate <image>` — or the full `charly box generate` (fast — it reuses scratch-stage caches) — and then `cat .build/<image>/Containerfile`. Filtering to one image happens implicitly via `charly box build <image>`, which invokes generate internally and then builds only the requested image + its dependencies.

```bash
# Generate all Containerfiles
charly box generate

# Generate with a custom tag
charly box generate --tag v1.2.3

# Inspect generated output
cat .build/fedora/Containerfile
```

## Generated Output

| Path | Description |
|---|---|
| `.build/<image>/Containerfile` | The generated Containerfile — unconditional RUN / COPY / ENV / LABEL directives |
| `.build/<image>/_inline/<layer>/<sha256>` | Inline-content bytes from `write:` tasks (content-addressed, idempotent) |
| `.build/<image>/traefik-routes.yml` | Traefik dynamic config (only for images with `route:` layers) |
| `.build/<image>/<fragment_dir>/*.conf` | Init-system service configs (e.g. `supervisor/`, `systemd/`) |
| `.build/_layers/<name>` | Symlinks to remote layer directories |

All generated files start with `# <path> (generated -- do not edit)`.

## Per-layer emission pipeline

For each layer, `deploykit.Generator.WriteCandySteps` (relocated from `charly/generate.go` in #67) runs this sequence:

```
1. Comment header:    # Layer: <name>
2. ENV from `var:` + ARG TARGETARCH + ENV ARCH=${TARGETARCH} (once per layer)
3. Package install (rpm/deb/pac/aur/apk) — always USER root
4. tasks: iterated in author order:
   a. Resolve ${VAR} in non-verbatim fields
   b. Determine user: (default root)
   c. Emit USER <value> if different from running USER
   d. Dispatch to the verb-specific emitter
   e. Adjacent same-verb same-user tasks coalesce into one directive
      (mkdir, link, setcap) — see /charly-image:layer execution-order section
   f. Parent-dir auto-insertion for copy/write when no earlier mkdir covers
5. Builders (pixi/npm/cargo/aur) — placement is end-of-layer unless an
   explicit `- build: all` task appears in tasks:
6. Reset to USER root (unless last layer and no further root steps follow)
```

**An unrenderable install is a LOUD failure, never a dropped step.** Step 3 above resolves
a candy's packages through the shared cascade; if they cannot then be rendered, generation
hard-errors naming the candy, the format and the box. Four conditions used to drop them
silently — a nil `DistroDef`, a primary build format with no definition in the box's distro,
a `RenderTemplate` error discarded by an `if err == nil` with no `else`, and an `EmitTasks`
error written into the Containerfile as a comment. The third one shipped: a format's
`phases.install.container` calling a function absent from `buildkit.TemplateFuncs` is an ordinary
authoring mistake, and the emitter turned it into a Containerfile that built successfully,
exited 0, and produced an image with NONE of the candy's packages. Nothing in the artifact
showed a missing step; it surfaced later, elsewhere, as an absent binary.

A candy that resolves NO packages for this box still passes through untouched — that is the
ordinary per-distro case (a `distro:` map with no section for this box), and it is not an
error. The distinction that matters is *resolved but unrenderable* versus *nothing to do*.

Two authoring consequences:

- **A box inherits `defaults.build:` when it declares no `build:`.** A box on an Alpine base
  with no `build:` took `[rpm]`, so the cascade resolved its packages correctly and then had
  no rpm template in the alpine distro to render them with. Declare `build:` on any box whose
  base is not the project default's family.
- **Check a template function against `buildkit.TemplateFuncs` before using it.** The set is
  twelve: `anyRepoHasURL`, `cacheMounts`, `cacheMountsAuto`, `cacheMountsOwned`,
  `default`, `hasSuffix`, `join`, `printf`, `quote`, `replace`, `shquote`, `splitFirst`.
  Anything else — `base`, say — fails at render time, which is now loud but is still
  cheaper to avoid. Prefer doing the work in the shell the template emits.

  A partial list here is worse than saying nothing: the paragraph exists to BE the list a
  reader greps against, so naming six of the twelve teaches that `join` and `replace`
  fail when they are shipped helpers. The way that happens is a `grep` over a windowed
  region of the FuncMap whose output is published as complete. Read the map's full
  literal, or count it in Go — never a
  windowed grep.

### Per-verb emitters (`sdk/deploykit/tasks_emit.go`)

| Verb | Emitter | Containerfile output |
|---|---|---|
| `mkdir` | `EmitMkdirBatch` | `RUN mkdir -p p1 p2 … [ && chmod <mode> p1 … ]` (one RUN per batch; grouped by mode) |
| `copy` | `EmitCopy` | `COPY --from=<layer-stage> --chmod=<mode> [--chown=<uid>:<gid>] <src> <dest>` — **no RUN** |
| `write` | `EmitWrite` | `COPY --chmod=<mode> [--chown=<uid>:<gid>] .build/<image>/_inline/<layer>/<sha256> <dest>` — **no `--from=`** (the source is a build-context path, not a stage), **no RUN, no heredoc** |
| `link` | `EmitLinkBatch` | `RUN ln -sf t1 l1 && ln -sf t2 l2 …` (one RUN per batch) |
| `download` | `EmitDownload` | `RUN --mount=type=cache,dst=/tmp/downloads <shell-probe> 'export BUILD_ARCH=$(uname -m); … __c=/tmp/downloads/<sha256 of url>; [ -s "$__c" ] \|\| { curl -fsSL "$__u" -o "$__c.part" && mv -f "$__c.part" "$__c"; } && <extractor> "$__c"'` — the fetch is **content-addressed to a cache file, never piped**, so a re-run with a warm cache skips the download (one RUN per download; `export ...;` termination is required so the shell expands `${BUILD_ARCH}` in the URL) |
| `setcap` | `EmitSetcapBatch` | `RUN setcap -r … && setcap caps path …` (strip + set chained) |
| `cmd` | `EmitCmd` | `RUN --mount=type=bind,from=<layer-stage>,source=/,target=/ctx [--mount=type=cache,…] <shell-probe> <<'OVCMD'` + the body + `OVCMD` — a heredoc carries the multi-line body without escaping, which podman's Dockerfile parser accepts (it would otherwise split at unescaped newlines) |
| `build` | handled inline in `WriteCandySteps` (deploykit) | Existing pixi/npm/cargo/aur multi-stage + inline blocks |
| `plugin` | `ProvisionActor.RenderProvisionScript` (builtin, in-proc) **or** `invokeOpEmitFragmentOpt` → `Invoke(OpEmit)` | A builtin act-renderer emits a shell `RUN`; any other resolved provider (external `grpcProvider`, or a fragment-emitting builtin) returns a `spec.EmitReply.Fragment` spliced verbatim. The single seam every Containerfile plugin-step emit flows through; an unresolved verb is a loud error. `plugin: command` is the one exception — its act IS the full `EmitCmd` install-task RUN. |

`<shell-probe>` above is the shared prefix both emit sites use instead of naming a
shell directly (`sdk/deploykit.BuildStepShellDashC` / `BuildStepShellHeredoc`):
`sh -c 'SH=/bin/sh; [ -x /bin/bash ] && SH=/bin/bash; exec "$SH" …' sh`. It prefers
`/bin/bash` where the base image has it and falls back to POSIX `sh` where it does not,
so a candy's shell steps build on a busybox base (Alpine) as well as on a bash one. The
authored command reaches the chosen shell unchanged — the `-c` form passes it as `$1` and
re-execs, the heredoc form relies on `exec` preserving stdin — so author `$(cmd)` and
`${VAR}` still expand in the shell that runs them.

## Cache-mount inheritance

Cache mounts come from the embedded build vocabulary — the `distro:` section (format caches) and `builder:` section (builder caches). `sdk/deploykit/tasks_emit.go`'s `TaskCacheMounts` (line 203) picks the right set based on task context:

| Task context | Cache mount(s) |
|---|---|
| `cmd:` as root | Distro format caches (`/var/cache/libdnf5` / `/var/cache/apt` / `/var/cache/pacman/pkg`) + `/ctx` bind mount |
| `cmd:` as non-root | `/tmp/npm-cache` (UID-scoped) + `/ctx` bind mount |
| `download:` | `/tmp/downloads` (shared across layers) |
| Package install | Distro format caches from the embedded `distro:` section |
| pixi builder | `/tmp/pixi-cache` + `/tmp/rattler-cache` (UID-scoped) |
| npm builder | `/tmp/npm-cache` (UID-scoped) |

The `/ctx` bind mount exposes the layer's own directory tree to `cmd:` tasks — so you can still reference `/ctx/<file>` inside an escape-hatch shell block for one-off file access (though `copy:` / `write:` are strongly preferred).

## USER emission

- `user: root` → `USER 0`
- `user: ${USER}` → `USER <numeric UID>` (numeric form avoids an `/etc/passwd` dependency at the switch point)
- `user: <uid>:<gid>` → `USER <uid>:<gid>` (e.g. `1010:1010`)
- `user: <name>` → `USER <name>` (literal; requires user to exist — create via earlier `cmd: useradd` task)

`COPY --chown=` uses numeric `<UID>:<GID>` for `${USER}` (BuildKit-safe), name-pairs for literal users.

## WriteBootstrap — adopt vs create

`WriteBootstrap` (in `sdk/deploykit`, relocated from `charly/generate.go` in #67) emits the user-creation section of the base-image Containerfile and branches on `ResolvedBox.UserAdopted`:

**Adopt mode** (`UserAdopted = true`) — the base image already ships the declared user; no `useradd` is needed. Emits:

```dockerfile
# User ubuntu (uid=1000) adopted from base image (declared in the embedded charly/charly.yml distro.base_user) — no useradd needed

WORKDIR /home/ubuntu
USER 1000
```

**Create mode** (`UserAdopted = false`) — classic idempotent useradd:

```dockerfile
RUN if ! getent passwd 1000 >/dev/null 2>&1; then \
      LOGIN_SHELL=/bin/sh; [ -x /bin/bash ] && LOGIN_SHELL=/bin/bash; \
      if command -v useradd >/dev/null 2>&1; then \
        (getent group 1000 >/dev/null 2>&1 || groupadd -g 1000 user) && \
        useradd -m -u 1000 -g 1000 -s "$LOGIN_SHELL" user; \
      else \
        (getent group 1000 >/dev/null 2>&1 || addgroup -g 1000 user) && \
        adduser -D -h /home/user -u 1000 -G user -s "$LOGIN_SHELL" user; \
      fi; \
    fi

WORKDIR /home/user
USER 1000
```

The pivot is `ResolvedBox.UserAdopted`, set by the `user_policy:` reconciliation in `sdk/buildkit/config_resolve.go:ResolveBox`. See `/charly-image:image` "user_policy" for the policy semantics, `/charly-build:build` "base_user:" for the declaration, and `/charly-distros:ubuntu` for the canonical adopt-mode worked example.

Neither branch does destructive metadata mutation (no `usermod -l` rename). Fedora/Arch/Debian always hit the create branch (no `base_user:` declared); Ubuntu under `user_policy: auto` hits the adopt branch.

## Tag-section install emission

Distro-version tag sections like `debian:13:` and `ubuntu:24.04:` are resolved via first-match-wins on the image's `distro:` priority list (e.g. `["ubuntu:24.04", "ubuntu", "debian"]`). Each matched tag section uses the primary format's full install template — so a tag section can carry `repos:`, `keys:`, `options:`, and `package:`, not just packages alone. See `charly/layers.go:TagPkgConfig.Raw` for the map that captures full tag-section YAML, and `/charly-image:layer` for authoring reference.

## ARCH / TARGETARCH emission

Every layer section begins with:

```dockerfile
ARG TARGETARCH
ENV ARCH=${TARGETARCH}
```

`${ARCH}` is then resolvable by BuildKit's ENV substitution in subsequent `COPY` paths, `ENV` values, and inside `RUN` shell. `${BUILD_ARCH}` is uname-style and auto-injected as a shell-local variable at the top of each `cmd:` / `download:` RUN (`BUILD_ARCH=$(uname -m)`), so multi-arch URL templates using either form work without author ceremony.

## Inline-content staging (`write:` verb)

`write: content:` is written to `.build/<image>/_inline/<layer>/<sha256>` at generate time, where `<sha256>` is the SHA-256 of the content. This means:

- **Idempotent:** rewriting the same content is a no-op
- **Content-addressed:** editing the content changes the hash, which changes the COPY source path, which invalidates only that single COPY layer's cache
- **No shell heredoc:** the COPY delivers the bytes directly; no `$`, backticks, or `EOF` markers need escaping

The Containerfile references the file by its relative path: `COPY --chmod=<mode> .build/<image>/_inline/<layer>/<sha256> <dest>` — no `--from=`, because the source is a build-context path rather than a stage.

## Behavior

- Generation is idempotent — safe to run repeatedly.
- `.build/` is disposable and gitignored; `charly box generate` will recreate it from scratch.
- Layer dependencies resolve transitively and topologically; circular `require:` is a validation error (surfaced by `/charly-build:validate`).
- Pixi manylinux fix is injected into `pixi.toml` files during the pixi builder stage.
- Multi-stage builds use builder images declared in the embedded `builder:` section (`pixi-builder`, `npm-builder`, `arch-builder` for AUR, etc.).
- Stale `.build/<image>/` directories (from removed or renamed images) are cleaned at the start of each generation.

### LABEL placement (cache efficiency)

All `ai.opencharly.*` LABEL directives are emitted at the **end** of
the final stage, after the last `USER` directive. This means a test or
label edit only re-runs the LABEL steps themselves (metadata-only, ~2
sec) instead of invalidating the buildkit cache for every upstream
RUN/COPY. Particularly important for test authoring: `check:` edits on
a 138-step stack like `immich-ml` cost seconds, not minutes per
iteration. See `/charly-internals:generate-source` "LABEL Placement" for the
rationale and `/charly-check:check` for author-facing workflow implications.

## Bootc-specific generator behaviour

Three emission rules matter specifically for bootc images (those whose `base:` is a bootc container such as `quay.io/fedora/fedora-bootc:43`).

### 1. `initHasFragments` pre-scan gates empty init stages

Each init system defined in the embedded `init:` vocabulary (`supervisord`, `openrc` and `systemd`) emits a `FROM scratch AS <stage_name>` scratch stage that layers COPY fragments into, plus an `assembly_template` RUN that bind-mounts from that scratch stage. `sdk/deploykit` (relocated from `charly/generate.go` in #67) pre-scans the layer chain for each init system to check whether any layer contributes fragments (per-entry rendered from the unified `service:` list via `ServiceSchema.ServiceTemplate` / `ServiceSchema.SupportsPackaged`, plus relay configs, plus systemd `.service` files). If none, **both** the scratch stage and the assembly_template RUN are suppressed. Without this, a bootc image with only packaged-unit entries (`use_packaged:`, no rendered body) would emit an empty `FROM scratch AS systemd-services` + a RUN that bind-mounts from it — which fails at build time. The `post_assembly_template` for that init still runs — it is independent of the scratch stage. `system_enable_template` is different: it renders for the image's RESOLVED init only (`img.InitSystem`), never for every ACTIVE init. `activeInits` is a SET — a candy chain routinely contributes fragments for several inits at once, and each legitimately gets its own fragment stage and assembly step — but a distro-shipped `use_packaged:` unit is enabled by the ONE init that boots the image. Rendering the shared unit list through every active init emitted one enable command PER init into a single Containerfile, so a supervisord container carrying an openrc fragment received `rc-update add` on a base with no OpenRC (opencharly/sdk#143, opencharly/charly#313).

### 2. `anyRepoHasURL` helper → prepend `dnf5-plugins`

The RPM install phase (`phases.install.container`) prepends `dnf install -y dnf5-plugins` whenever any layer `rpm.repos:` entry declares a `url:` (checked via the `anyRepoHasURL` template helper in `sdk/buildkit/render.go`). Required because `quay.io/fedora/fedora-bootc:43` strips `dnf5-plugins` (which provides the `config-manager` subcommand) from the default install. Without the prepend, `dnf5 config-manager addrepo --from-repofile=…` — emitted for every URL-based repo — fails with `Unknown argument "config-manager" for command "dnf5"`. `/charly-selkies:ffmpeg` is the canonical URL-repo consumer (adds negativo17's `fedora-multimedia.repo`).

### 3. `export BUILD_ARCH=…;` (not prefix assignment) in `download:` tasks

The `download:` task emits `export BUILD_ARCH=$(uname -m); curl -fsSL "…${BUILD_ARCH}…"` with an explicit semicolon-separator `export`, not the prefix form `BUILD_ARCH=$(uname -m) curl ...`. Bash prefix assignments set the variable in the spawned command's environment **after** the shell has already expanded `${BUILD_ARCH}` in the command's arguments — the expansion sees an unset variable, the URL resolves with an empty arch string, and the download 404s. Source: `sdk/deploykit/tasks_emit.go:242` (`envPrefix`) with the documenting comment. Layers that use `${BUILD_ARCH}` in a `download:` URL: `/charly-languages:pixi`, `/charly-coder:typst`, `/charly-tools:yay`, `/charly-infrastructure:vectorchord`, `/charly-tools:sherpa-onnx`.

## Project directory override

`charly box generate` resolves `charly.yml` via `os.Getwd()`. Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`. See `/charly-image:image` "Project directory resolution".

## Cross-References

### `charly box` family siblings

- `/charly-image:image` -- Family overview + charly.yml composition reference
- `/charly-build:build` -- Building images (calls generate internally)
- `/charly-build:inspect` -- Inspect generated output for a specific image
- `/charly-build:list` -- Enumerate targets before generation
- `/charly-build:merge` -- Post-build layer consolidation
- `/charly-build:new` -- Scaffold a new candy to generate into
- `/charly-build:pull` -- Pull prebuilt images (orthogonal to generate)
- `/charly-build:validate` -- Validation rules for images and layers (including per-verb task rules)

### Related skills

- `/charly-image:layer` — **Canonical task verb catalog, `var:` substitution, YAML anchors, execution order.** Read this first for authoring questions.
- `/charly-check:check` — test-authoring workflow; `check:` blocks are embedded via `writeJSONLabel` and benefit directly from LABELs-at-end cache efficiency.
- `/charly-internals:generate-source` — Deep dive on Containerfile emission internals, `Task` struct, per-verb emitters, `StageInlineContent`, the `spec.ShellQuote` quoting helper and the `BuildStepShellDashC`/`BuildStepShellHeredoc` shell-prefix pair, LABEL-placement rationale.
- `/charly-internals:go` — Source-code map: `charly/tasks.go` (64 lines, one function: `invokeOpEmitFragmentOpt`), `sdk/deploykit/tasks_emit.go` (the per-verb emitters), `sdk/deploykit` (`WriteCandySteps` + `WriteLabels`, relocated in #67), `charly/layers.go` struct definitions.
- `/charly-selkies:ffmpeg` — canonical URL-repo consumer (triggers the `dnf5-plugins` prepend).
