---
name: load
description: |-
  When a locally-built image must reach the NESTED podman store inside a running pod (a candybox running its own containers). `charly box load <target> <image>` — the container twin of `charly vm cp-box`.
---

# charly box load — deliver an image into a pod's NESTED store

Invoked as `charly box load <target> <image>`. The container-venue twin of
`charly vm cp-box` (`/charly-vm:vm`); both are bindings of one shared path.

## What problem it solves

A pod composing `container-nesting` runs its own rootless podman, with an image
store that is genuinely SEPARATE from the host's. Anything that must run inside
that boundary needs its images in the NESTED store.

The alternatives do not work. Binding the host's podman socket into the pod
dissolves the boundary — containers spawned there land in the HOST store, beside
everything else running on the machine. A registry pull is no escape when the
image is private or was never pushed. That leaves local delivery, and `box load`
is its charly-native form: before this verb the only path was a hand-run
`podman save | podman --remote load` pipeline, a manual container-engine command
against a charly-managed deploy (forbidden — the CLI is the only operational
interface).

## Prerequisite: the venue must serve a socket

The target pod must compose `nested-podman-socket`
(`/charly-distros:nested-podman-socket`), which serves the nested podman's
API socket at uid 1000 and declares the named volume at `/run/user/1000` that
makes it possible. Without it the verb reports that the venue serves no socket
and names the path it tried.

The default socket is `/run/user/1000/podman/podman.sock` — the uid-1000
rootless path that composition serves. `--socket` overrides it.

Composing it looks like this. Note which entity this is: `base:` marks it a
BOX — the thing you build. Deploying that box on a container venue gives you
the pod this verb targets; on a VM venue the twin verb is `charly vm cp-box`.
The two arguments follow from that: `<target>` names the running deploy, never
the box, and `<image>` is the artifact being delivered into it.

```yaml
my-spike-box:
    candy:
        version: 2026.230.1200
        base: quay.io/fedora/fedora:43
        description: A box that serves its own rootless podman socket, so a deploy of it can receive images.
        distro:
            - fedora:43
            - fedora
        candy:
            - '@github.com/opencharly/pod-nested-podman-socket'  # pulls in container-nesting itself
```

## Usage

```bash
charly box load <target> <image>                  # deliver into the venue's store
charly box load <target> <image> --as <stable-ref> # …and tag it there
charly box load <target> <image> --socket <path>   # non-default socket path
charly box load <target> <image> --instance <n>    # when the target runs several
```

`<target>` is the running deploy, named exactly as `charly shell` and `charly cp`
take it. `<image>` is either an explicit local ref — used as authored, because
naming a tag IS the choice — or a bare box name, which resolves to the newest
local BUILD. That resolution is strict: it refuses to elect an image older than
the newest local build, because delivering a stale artifact is the wrong-artifact
class `charly check box` refuses to certify, and it is far harder to notice here
(the load succeeds; the venue simply runs the wrong image).

## The transfer is verified, not assumed

`podman load` can exit 0 on a TRUNCATED stream, registering an image whose overlay
layers are incomplete — a later `podman run` then fails with
`…/storage/overlay/<hash>: no such file`. So the transfer is never trusted on the
load exit code alone:

- the idempotency skip fires only when the venue already holds the ref AND that
  image is verified intact, so a present-but-torn image is not wrongly skipped;
- after a load the image is probed, and on the overlay-corruption signature it is
  dropped and re-streamed ONCE; a second failure is a hard error rather than a
  broken image shipped silently.

Nothing touches disk in between — the image streams `save | load` with no
intermediate tarball, which also matters because a multi-GB image will not fit in
a size-limited `/tmp`.

## Verification

`check-boxload-pod` is the R10 bed — a deploy in the project's own `charly.yml`
marked `disposable: true`, the flag that authorizes charly to destroy and
rebuild it unattended. It delivers a host image into the pod's nested
store and asserts, THROUGH the socket, that the image is there. The assertion is
socket-scoped deliberately — a plain in-container `podman images` would pass even
if the image had gone to the wrong store.

A successful delivery reports each stage, so a skip is distinguishable from a
transfer:

```
box load: streaming localhost/my-image:latest into venue storage (save | load)...
box load: localhost/my-image:latest is now in venue storage (verified intact)
```

and an already-present, verified image says so instead of re-streaming:

```
box load: venue already has localhost/my-image:latest (verified intact) — skipping transfer
```
