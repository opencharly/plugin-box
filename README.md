# plugin-box

The build-mode `charly box` command handlers for OpenCharly — twelve nested
commands under the shared `box` command group.

`charly box` is a shared group whose subcommands have different owners: the core
`BoxCmd` keeps only the grammar spine (a bare `kong.Plugins` holder), the
`plugin-authoring` candy contributes the authoring words, and this candy
contributes the twelve build-mode commands. Each is compiled into charly and
dispatches in-process (`Invoke(OpRun)`), so the handlers own charly's real
stdio. Placement is invisible: the same provider compiles in or serves
out-of-process.

## What it provides

| Capability | Surface |
|---|---|
| `command:generate:box` | `charly box generate` — render the `.build/` Containerfile tree |
| `command:validate:box` | `charly box validate` — the per-kind/op rule engine + resolution-graph checks |
| `command:new:box` | `charly box new candy\|project\|box` — the scaffold engine |
| `command:pull:box` | `charly box pull` — ensure an image is present (pull, else build) |
| `command:build:box` | `charly box build` — the build drive |
| `command:inspect:box` | `charly box inspect` — the resolved box view |
| `command:list:box` | `charly box list <sub>` — boxes/candies/targets/services/routes/volumes/aliases |
| `command:labels:box` | `charly box labels <ref>` — the image's OCI labels |
| `command:load:box` | `charly box load <target> <image>` — stream an image into a running pod's nested store |
| `command:merge:box` | `charly box merge` — layer merging |
| `command:reconcile:box` | `charly box reconcile` — align cross-repo `@github` pins |
| `command:feature:box` | `charly box feature run <image>` — build-scope Agent Driven Evaluation |

The heavy build drive (`build:box` / `build:generate` / `build:ensure`) lives in
the sibling `plugin-build` candy; `generate`, `pull`, `build`, and `merge` reach
it over the provider registry.

## How to use it

The commands are compiled in — no candy composition is needed:

```bash
charly box generate my-box
charly box validate
charly box inspect my-box
charly box list boxes
```

## Layout

- `candy/plugin-box/` — the plugin module: `plugin.go`, `box.go`, `validate*.go`,
  `inspect_list.go`, `box_load.go`, `merge_cmd.go`, `reconcile.go`,
  `cmd/serve/main.go`.
- `candy/plugin-box/charly.yml` — the `plugin-box:` candy entity (`plugin:`
  block, `plan:` check).
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-image:image` — the `charly box` command family, box
  definitions, and the box dependency graph.
- `/charly-build:build` · `/charly-build:validate` · `/charly-build:generate` —
  the individual command skills.
- `/charly-internals:plugin` — the plugin/provider model.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
