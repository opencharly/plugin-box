---
name: merge
description: |-
  Post-build layer optimization via merging consecutive small layers — `charly box merge`, image layer reduction, merge configuration.
---

# charly box merge -- Post-Build Layer Optimization

Invoked as `charly box merge [<image>]`. See `/charly-image:image` for the family overview.

## Overview

Reduces image layer count by merging consecutive small layers. Uses `go-containerregistry` to load the image, groups consecutive layers below a size threshold, deduplicates filesystem entries (last writer wins), and reconstructs the image. Idempotent -- safe to run multiple times.

## Quick Reference

| Action | Command | Description |
|--------|---------|-------------|
| Merge single image | `charly box merge <image>` | Merge layers in specified image |
| Dry run | `charly box merge <image> --dry-run` | Show what would be merged without changing anything |
| Custom threshold | `charly box merge <image> --max-mb N` | Set max layer size for merge candidates (default: 128 MB) |
| Merge all auto | `charly box merge --all` | Merge all images that have `merge.auto: true` |

## Usage

### Basic Merge

```bash
# Merge consecutive small layers in an image
charly box merge sway-browser-vnc

# Preview without changing
charly box merge sway-browser-vnc --dry-run
```

### Custom Size Threshold

```bash
# Only merge layers smaller than 64 MB
charly box merge sway-browser-vnc --max-mb 64
```

### Automatic Merge for All Configured Images

```bash
# Merge all images that opt in via charly.yml
charly box merge --all
```

## Configuration in charly.yml

The unified form is name-first: the image entity carries its scalars — including
the `merge:` map — inline in its `candy:` kind value (an image is a `candy:` node
carrying `base:`/`from:`).

```yaml
sway-browser-vnc:
    candy:
        base: fedora
        merge:
            auto: true      # Include in `charly box merge --all`
            max_mb: 128     # Size threshold (default: 128)
```

## How It Works

1. Load image via `go-containerregistry`
2. Group consecutive layers where each is below `max_mb`
3. Deduplicate filesystem entries across merged layers (last writer wins) and suppress whiteout conflicts
4. Reconstruct image with merged layers
5. Inline merge also runs automatically after each build level during `charly box build`

## Whiteout Handling

OCI/Docker images use special "whiteout" files to represent file deletions across layers. When merging layers, these must be handled correctly to prevent `EEXIST` errors during overlay unpack.

**Three cases:**

1. **Regular whiteout** — A file `.wh.<name>` in layer N indicates that `<name>` was deleted. If an earlier layer contains the original file, the merge suppresses the original (keeps the whiteout marker so the deletion is preserved in the merged output).

2. **Opaque whiteout** — A file `.wh..wh..opq` in directory D means "the entire directory was replaced." All entries under D from earlier layers are suppressed. Only entries from the layer containing the opaque marker (and later layers) survive.

3. **Reintroduction supersedes whiteout** — If a file is deleted (whiteout in layer M) then re-created (same path in layer N, where N > M), the whiteout is suppressed and the re-introduced file is kept. This prevents the merged layer from containing both a file and its own whiteout, which would cause overlay unpack failures.

**Why this matters:** Without whiteout suppression, merged layers could contain contradictory entries (a file and its `.wh.*` marker coexisting), causing `EEXIST` errors when the container runtime unpacks the layer onto an overlay filesystem.

## Known limitation — `podman load: file exists` on multi-stage RPM images

In some images (observed empirically against `immich:2026.128.x`), the post-build merge step succeeds at the Go-level (every merge group emits a valid tarball that passes internal consistency checks) but `podman load` rejects the final docker-archive with:

```
unpacking failed (error: exit status 1; output: file exists)
charly: error: post-build merge optimization failed (image is functional but unmerged): podman load: exit status 125
  Diagnostic: set CHARLY_MERGE_KEEP_TMP=1 and re-run `charly box merge <name>` to capture the failing /tmp/charly-merge-*.tar.
  This is a known limitation against multi-stage RPM-installed images; the build itself succeeded and the image at this tag is correct.
```

**Investigation (May 2026)** ruled out every canonical mergeLayers bug class:

- 0 within-layer duplicate Names
- 0 whiteout/file collisions inside any merged layer
- 0 broken hardlinks (all `Linkname` targets present in their own layer)
- 0 cross-layer typeflag conflicts (no path is regular-file in one layer + directory in another)

The trigger appears to be a podman-side overlay-unpack quirk under specific layer-content patterns — multi-stage RPM-installed images that touch `/usr/lib/sysimage/rpm/*` in 6+ source layers consistently reproduce. Possibly related to the known podman-5.7.x blob-reuse race (`storage_dest.go:TryReusingBlobWithOptions`) recorded in `CHANGELOG/`, but unconfirmed against 5.8.x.

### Operational impact

**The failure is non-fatal.** `mergeBox` (`candy/plugin-build/drive.go`) treats merge failure as a non-fatal warning — it never fails the build. The build itself returns 0, the image is tagged at its CalVer, every individual layer digest is valid, and `charly start` runs the unmerged image with no functional difference — only the layer count is higher than ideal (more layers than a successful merge would produce).

### Diagnostic hook: `CHARLY_MERGE_KEEP_TMP=1`

When merge fails and you want to capture the failing tarball for inspection or forensic analysis, set `CHARLY_MERGE_KEEP_TMP=1`:

```bash
CHARLY_MERGE_KEEP_TMP=1 charly box merge <name>
```

On failure the tarball is left at `/tmp/charly-merge-<random>.tar` (path printed to stderr) instead of being cleaned up. The tar is a docker-archive — extract `manifest.json` to see the layer chain, then `tar -xzf <hash>.tar.gz` on individual layers to inspect their contents.

For forensic analysis of layer contents:

```bash
# List paths within a single layer
zcat <hash>.tar.gz | tar -tvf -

# Find paths that appear in multiple layers (the cross-layer overlap pattern)
for f in *.tar.gz; do
  zcat "$f" | tar -tf - | sed "s|^|$f\t|"
done | awk -F'\t' '{cnt[$2]++} END {for (p in cnt) if (cnt[p]>1) print cnt[p], p}' | sort -rn | head
```

Source: `candy/plugin-oci/merge.go:saveImageToDaemon` (the keep-on-fail logic; `loaded` flag gates the cleanup defer) — the go-containerregistry merge engine has lived out-of-process here since the P14a cutover; it was never in `charly/merge.go` (the former core CLI shell, itself relocated to `candy/plugin-box/merge_cmd.go` at P14).

## Project directory override

`charly box merge` resolves `charly.yml` via `os.Getwd()`. Override with `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`. See `/charly-image:image` "Project directory resolution".

## Cross-References

### `charly box` family siblings

- `/charly-image:image` -- Family overview + charly.yml composition reference
- `/charly-build:build` -- Building images (merge runs inline after each build level)
- `/charly-build:generate` -- Containerfile generation
- `/charly-build:inspect` -- Inspect merged images
- `/charly-build:list` -- Enumerate images before merging
- `/charly-build:new` -- Scaffold new candies
- `/charly-build:pull` -- Pull prebuilt images into local storage
- `/charly-build:validate` -- Validate before merging

### Related skills

- `/charly-image:layer` -- Layer authoring (layer size affects merge behavior)
