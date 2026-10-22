---
name: reconcile
description: |-
  When @github layer/namespace pins drift across repos and the resolver emits 'referenced at multiple versions' warnings — `charly box reconcile` aligns every pin of a repo to one version.
---

# charly box reconcile — align cross-repo `@github` version pins

Invoked as `charly box reconcile`. See `/charly-image:image` for the family overview.

## Overview

The layer resolver compares each layer's PER-ENTITY `version:` (read after fetch),
not the repo git tag — so a repo re-tag of an UNCHANGED layer does NOT warn. When
a layer DOES resolve to two different per-entity versions (a family pinned to a
genuinely newer layer than the shared infra it composes), it **warns once and uses
the newest** (see `/charly-internals:go` "Remote-layer resolver", `/charly-build:validate`).
The git `:vTAG` is only the FETCH coordinate. `charly box reconcile` aligns the
on-disk git-tag pins — for each distinct repo referenced by the project's
versioned YAML, it rewrites EVERY pin of that repo to ONE target tag, so every
reference fetches one commit per repo and the next `charly box generate` emits
**zero** version warnings. Edits are comment-preserving (yaml.v3 node API). The
default mode is idempotent; `--remote` is not, because its target is whatever the
remote's newest tag is at the moment of the call — see "`--remote` targets whatever
is newest NOW" below before reaching for that flag.

The **zero-warnings R10 gate** (the project rulebook R1 (`AGENTS.md`)) makes this load-bearing: a change
that introduces a version mismatch is not landable until `charly box reconcile`
clears the warning.

## Quick Reference

| Action | Command | Description |
|---|---|---|
| Preview rewrites | `charly box reconcile --dry-run` | Print every pin it would change; touch nothing |
| Align to newest referenced | `charly box reconcile` | Rewrite each repo's pins to the newest version ALREADY referenced (offline) |
| Align to newest remote tag | `charly box reconcile --remote` | Query `git ls-remote --tags` per repo and bump to the newest tag. **Moves pins to whatever is newest at call time — not idempotent against a moving remote; never use it to CHECK alignment** |

```bash
charly box reconcile --dry-run      # see the plan
charly box reconcile                # align to newest referenced (no network)
charly -C box/cachyos box reconcile   # reconcile a submodule's pins
```

## What it does

1. **Scan** every `@github.com/owner/repo[/path]:vTAG` ref in the project's
   versioned YAML files (`charly.yml` + discovered `box/<name>/charly.yml` /
   `candy/<name>/charly.yml`, plus any flat-imported legacy per-kind files —
   `charly.yml` / `check.yml` / `local.yml` / `pod.yml` / `kubernetes.yml` / `vm.yml` /
   `charly.yml`). Refs appear in `import:` namespaces, image `base:` / `builder:` /
   `candy:`, and `kind:local` `candy:` lists.
2. **Target** per repo: the newest version ALREADY referenced (default —
   `compareSemver`, orders CalVer correctly, no network) or, with `--remote`, the
   newest tag on the remote (`GitLatestTag`).
3. **Rewrite** every pin of that repo whose version differs from the target,
   preserving comments and key order. Unpinned refs and single-version repos are
   left untouched. In the default mode this converges: the target is the newest
   version already referenced, which after one run is every pin's version, so a
   second run rewrites nothing. `--remote` carries no such guarantee.

## `--remote` targets whatever is newest NOW

The default mode is safe to run as a QUESTION — *"are this repo's pins already
aligned?"* It reads only the versions the tree already references, so it needs no
network, converges, and answers without moving anything you did not intend to move.
Use it, or `--dry-run`, to check alignment.

`--remote` is a different operation. It ignores the referenced set entirely and
resolves each repo's target from `git ls-remote --tags` at the moment of the call
(`reconcileTargetVersion` returns `GitLatestTag` without consulting the referenced
versions at all), so it is idempotent only against a remote that has not moved.
Re-run it after a sibling repo publishes a newer tag and EVERY pin of that repo is
rewritten again — including pins a previous `--remote` run had just aligned.

The re-bump is reported, but its explanation is not. `rewrote N pin(s):` prints with
every rewritten ref, while the per-repo `<repo> -> <tag> (was at N versions)` summary
is emitted only for a repo found at MORE than one version. A freshly-aligned repo is
at exactly one — so the line that would say *why* the target moved is suppressed in
precisely the case where the target moved for a reason outside the tree.

**The operational consequence: `--remote` invalidates evidence gathered before it
ran.** A bed transcript, a build log, or a `charly check` run describes the tree at
the tags the pins held when it was produced. A later `--remote` can move those pins
to a tag no proof covers, leaving a tree that still looks correctly reconciled. So:

- To find out whether pins are aligned, run the plain `charly box reconcile` (or
  `--dry-run`). Never reach for `--remote` to answer a question.
- Run `--remote` deliberately, at the ONE point in a cutover where adopting the
  producer's newest tag is the intent — `/charly-internals:git-workflow` B6: after
  the producer is landed and tagged, BEFORE the consumer's authoritative R10.
- Once it has run, the pins name the tag the R10 must be produced against. If proofs
  already exist for an older tag, either re-run the gate against the new pin or
  restore the tag the proofs cover. Shipping a newer pin on an older proof is an R10
  failure whether or not anything rebuilt.

## Scope — one project per invocation

`charly box reconcile` operates on the CURRENT project (cwd; honors the top-level
`-C` / `--dir` / `CHARLY_PROJECT_DIR`). For a multi-repo tree (the main repo + its
`box/<distro>` submodules), run it per repo, or per submodule via `-C
image/<name>`. This pairs with the cross-repo landing order in
`/charly-internals:git-workflow` B6: land + tag the producer FIRST, then
`charly box reconcile` repoints the consumer to the producer's fresh tag before the
consumer's authoritative R10.

## Implementation

`candy/plugin-box/reconcile.go` — `dispatchReconcile` (wired as a `charly box` word in
`candy/plugin-box/box.go`; the former `charly/reconcile.go` is DELETED, K-wave 2).
Reuses `ParseRemoteRef` (`spec/spec/ref_parse.go`, re-exported via `sdk/kit/remote_ref.go`),
`IsRemoteCandyRef` / `StripVersion` (`sdk/deploykit/candy_ref.go`),
`CompareSemver` / `GitLatestTag` / `RepoGitURL` (`spec/refs/git.go`), and the
comment-preserving load/`yaml.Marshal` pattern from `sdk/kit/yaml.go`. Covered
by `candy/plugin-box/reconcile_test.go` (newest-referenced alignment, comment preservation,
idempotency, single-version-untouched, no-pins no-op).

## Cross-References

- `/charly-internals:go` "Remote-layer resolver" — the warn-and-newest-wins resolver this aligns to.
- `/charly-build:validate` — surfaces the multi-version warning reconcile clears.
- `/charly-internals:git-workflow` — cross-repo (B6) producer→consumer landing that calls reconcile.
- `/charly-build:migrate` — per-merge CalVer tags that reconcile pins point at.
- `/charly-image:image` — `import:` / namespace authoring + the family overview.

## When to Use This Skill

Invoke when the resolver warns that a layer is referenced at multiple versions,
when aligning a consumer's pins to a freshly-tagged producer, or whenever you need
the project's `@github` pins consistent for a zero-warnings R10.
