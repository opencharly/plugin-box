package box

// validate_schema_rules.go — the CUE-schema conformance pair + the remote-candy check, relocated
// VERBATIM out of charly core (charly/validate.go, DELETED) by K-wave 2 cone R1 unit B.
//
// These three were the last `charly box validate` rules the host still ran, held there by a
// justification that turned out to be FALSE: "they need the HOST's spliced cross-plugin CUE schema,
// a live non-marshalable cue.Value graph". They do not. Every CUE entry point they call
// (CueDocFromYAML / ValidateEntityClosedCUE / ValidateCandyManifestCUE / ValidateNodeFormSteps) is a
// FREE FUNCTION in sdk/loaderkit, compiled against loaderkit's OWN schema handle since cone R1
// ruling 1 moved the schema to the loader that validates against it; the spec.ProjectLoader methods
// core called were bare forwards to exactly those functions. The plugin splice charly builds in
// plugin_loader.go is a SEPARATE cue.Value used solely to gate one plugin's authored input — it was
// never unified into the schema these validators use.
//
// So they run here, over the SAME inputs, with no host round-trip:
//
//   - the candy set is the envelope's own adapters (vctx.dk — deploykit.NewSpecCandyModel over
//     rp.CandyModels/rp.Candies). Those are the IDENTICAL spec.CandyReader values the host's
//     ScanAllCandyWithConfigOpts produced: loaderkit.FinalizeScannedCandies builds each one with
//     deploykit.NewSpecCandyModel too, so GetSourceDir / GetRemote / GetName / GetRepoPath and the
//     already-bare GetRequire / GetIncludedCandy refs all read the same bytes core read.
//   - the raw *spec.Config is the one the plugin already self-loads for the raw-config rules
//     (validate_config_rules.go's loadRawProjectConfig), threaded in rather than loaded twice.
//   - the parser is loaderkit.DocParser, the ONE shared spec.DocParser adapter (sdk leg of this
//     unit) that candy/plugin-loader now embeds — never a second hand-rolled copy here (R3).

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/loaderkit"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// runSchemaAndRemoteChecks runs the three relocated validators in the SAME order the host's
// runHostNaturalValidateChecks ran them, accumulating into e. cfg may be nil (a project-less
// directory or a load that failed tolerantly) — every rule then loops zero times, exactly as the
// host's `lp.cfg == nil` early return did.
func runSchemaAndRemoteChecks(ctx context.Context, ex *sdk.Executor, vc *vctx, cfg *spec.Config, dir string, opts spec.ResolveOpts, e *vErr) {
	if cfg == nil {
		return
	}
	validateCandyCUESchemas(ctx, ex, vc.dk, e)
	validateProjectCUESchemas(ctx, ex, cfg, dir, opts, e)
	validateRemoteCandies(ctx, ex, cfg, vc.dk, e)
}

// validateCandyCUESchemas validates each loaded candy's on-disk manifest against the candy CUE
// schema (loaderkit.ValidateCandyManifestCUE — #Candy for a legacy kind-keyed manifest, #NodeDoc for
// a node-form manifest). This is the sole candy-schema validator. Inline/synthesized candies with no
// manifest file on disk are skipped.
func validateCandyCUESchemas(ctx context.Context, ex *sdk.Executor, candies map[string]spec.CandyReader, e *vErr) {
	threaded := loaderkit.LoaderThreadedViaExecutor(ctx, ex)
	for name, c := range candies {
		if c == nil || c.GetSourceDir() == "" {
			continue
		}
		f := filepath.Join(c.GetSourceDir(), spec.UnifiedFileName)
		data, err := os.ReadFile(f)
		if err != nil {
			continue // remote/inline candy without a local manifest — skip
		}
		if verr := loaderkit.ValidateCandyManifestCUE(f, data, threaded, loaderkit.DocParser{}); verr != nil {
			e.Add("candy %q: CUE schema: %v", name, verr)
		}
	}
}

// validateProjectCUESchemas validates the project's non-candy entities against the CUE schemas.
// Boxes are validated from the RESOLVED in-memory set (cfg.EachBox), so CUE coverage matches the
// rule coverage per repo (each repo validates its own boxes; submodule boxes are validated when
// `charly box validate` runs in that submodule). Candies are handled by validateCandyCUESchemas.
func validateProjectCUESchemas(ctx context.Context, ex *sdk.Executor, cfg *spec.Config, dir string, opts spec.ResolveOpts, e *vErr) {
	// Boxes: BoxConfig has no Name field (the name is the cfg.Box map key), so inject it into the
	// wire form before validating against #Box. Marshal the resolved struct back to YAML and run it
	// through the same ingest path the on-disk corpus uses. Skip disabled boxes exactly like the
	// other box rules (a disabled box's invalid fields are intentionally not flagged).
	for name, box := range cfg.EachBox {
		if !box.IsEnabled() && !opts.ShouldIncludeDisabled(name) {
			continue
		}
		entityYAML, err := boxEntityWireYAML(name, box)
		if err != nil {
			e.Add("box %q: CUE wire-encode: %v", name, err)
			continue
		}
		doc, derr := loaderkit.CueDocFromYAML("box:"+name, entityYAML)
		if derr != nil {
			e.Add("box %q: CUE ingest: %v", name, derr)
			continue
		}
		// Non-concrete (closedness + value-constraint conflicts, NOT missing-required /
		// disjunction-resolution): a scratch box with neither base nor from is valid, but
		// Concrete(true) can't resolve the base/from mutual-exclusion disjunction when both are
		// absent. The re-wiring's purpose is to catch SET-value declarative violations
		// (version/jobs/check_level/…), which Unify().Validate() catches; the only required #Box
		// field, name, is always injected above.
		if verr := loaderkit.ValidateEntityClosedCUE("box", "box:"+name, doc.LookupPath(cue.ParsePath("box"))); verr != nil {
			e.Add("%v", verr)
		}
	}

	// Every ROOT-file entity is validated at LOAD (the #NodeDoc gate): a legacy kind-keyed
	// (non-node-form) root file is HARD-REJECTED there with a `charly migrate` hint and never
	// reaches validation, so there is no root-shape collection validator any more. What LOAD leaves
	// lenient is each entity's ASSEMBLED plan STEPS, so the node-form step-typo gate
	// (ValidateNodeFormSteps against the closed #Step/#Op) runs here.
	threaded := loaderkit.LoaderThreadedViaExecutor(ctx, ex)
	rootFiles := []string{filepath.Join(dir, spec.UnifiedFileName)}
	if boxRoots, _ := filepath.Glob(filepath.Join(dir, "box", "*", spec.UnifiedFileName)); len(boxRoots) > 0 {
		rootFiles = append(rootFiles, boxRoots...)
	}
	for _, f := range rootFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if !isNodeFormFile(data) {
			continue // a legacy root-shape file is load-rejected (charly migrate) — nothing to validate here
		}
		if verr := loaderkit.ValidateNodeFormSteps(f, data, threaded, loaderkit.DocParser{}); verr != nil {
			e.Add("%v", verr)
		}
	}
}

// isNodeFormFile reports whether any document in a YAML file is unified node-form
// (spec.ClassifyDoc → spec.DocShapeNode). Used to skip the step-typo gate on a legacy root-shape
// manifest, which the loader rejects outright.
func isNodeFormFile(data []byte) bool {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var node yaml.Node
		if err := dec.Decode(&node); err != nil {
			break
		}
		if shape, err := spec.ClassifyDoc(&node); err == nil && shape == spec.DocShapeNode {
			return true
		}
	}
	return false
}

// boxEntityWireYAML marshals a resolved BoxConfig back to the authored `box:` wire form (a
// kind-keyed document), injecting the map-key name that BoxConfig does not itself carry, so it can
// be CUE-ingested and validated against #Box.
func boxEntityWireYAML(name string, box spec.BoxConfig) ([]byte, error) {
	raw, err := yaml.Marshal(box)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	m["name"] = name
	return yaml.Marshal(map[string]any{"box": m})
}

// validateRemoteCandies checks remote candy consistency: the ref-collection walk (which surfaces a
// ref whose repo default branch cannot be resolved) plus the cross-repo name-conflict check.
//
// The collection runs over loaderkit.CollectRemoteRefsOpts with the executor-backed
// spec.RefsCollectSeams (Downloader / MigrateCache / ResolveLocal) — the SAME pairing
// candy/plugin-build's scan legs already use twice. That bridge is what made this rule's former
// "no executor-backed RefsCollectSeams exists" host-residency IOU obsolete.
func validateRemoteCandies(ctx context.Context, ex *sdk.Executor, cfg *spec.Config, candies map[string]spec.CandyReader, e *vErr) {
	if _, err := loaderkit.CollectRemoteRefsOpts(cfg, candies, spec.ResolveOpts{}, loaderkit.RefsSeamsFromExecutor(ctx, ex)); err != nil {
		e.Add("%v", err)
	}

	// Naming conflicts between remote candies from different repos.
	//
	// A name provided from TWO repos is only a real conflict when the two are NOT
	// the same provider. The bake_plugin mechanism (a candy's `bake_plugin:` ref
	// pulling an out-of-tree plugin candy into its composing images) makes the
	// baked plugin appear under the BAKING repo's path while the standalone repo
	// provides it directly — the same underlying repo, not a genuine clash. The
	// conflict check must dedupe that case: when candy X (repo A) and candy Y
	// (repo B) share a name, and some candy in repo A carries a `bake_plugin:`
	// ref whose repo is B, X is the baked duplicate of Y — not a conflict.
	reportRemoteNameConflicts(candies, e)
}

// reportRemoteNameConflicts flags a candy NAME provided by two different repos.
//
// Extracted from validateRemoteCandies so the rule is reachable without the
// network-touching ref collection that precedes it there. That prologue is why this
// loop had no test of its own while its bakePluginSibling helper did — and the
// directional-dedupe defect below lived in the loop, not the helper.
func reportRemoteNameConflicts(candies map[string]spec.CandyReader, e *vErr) {
	for _, candy := range candies {
		if !candy.GetRemote() {
			continue
		}
		for _, other := range candies {
			if !other.GetRemote() || other == candy {
				continue
			}
			if other.GetName() != candy.GetName() || other.GetRepoPath() == candy.GetRepoPath() {
				continue
			}
			// Dedupe the bake_plugin case: is EITHER provider baked into the
			// other's repo by a sibling's bake_plugin ref?
			//
			// Both directions must be tested. This loop is symmetric — it visits
			// (candy=A, other=B) and (candy=B, other=A) — but the bake relationship
			// is not: only the BAKING repo holds the sibling carrying the ref.
			// Testing one direction suppressed the visit where the baker was
			// `candy` and let the mirrored visit through, so every baked plugin
			// reported the identical conflict TWICE, in opposite orders.
			if bakePluginSibling(candies, candy, other.GetRepoPath()) ||
				bakePluginSibling(candies, other, candy.GetRepoPath()) {
				continue
			}
			e.Add("remote candy name conflict: %q provided by both %s and %s", candy.GetName(), candy.GetRepoPath(), other.GetRepoPath())
		}
	}
}

// bakePluginSibling reports whether some candy in the same repo as candy (sharing
// its RepoPath) carries a bake_plugin: ref pointing at otherRepo — i.e. otherRepo's
// candy of this name is baked into candy's repo rather than a genuine clash.
func bakePluginSibling(candies map[string]spec.CandyReader, candy spec.CandyReader, otherRepo string) bool {
	// Scope the dedup to the NAME-MATCHED plugin: a sibling baking a DIFFERENT
	// plugin from the same other repo is not the same provider, and must not
	// suppress a genuine conflict.
	for _, sib := range candies {
		if !sib.GetRemote() || sib.GetRepoPath() != candy.GetRepoPath() {
			continue
		}
		for _, ref := range sib.GetBakePlugin() {
			if !ref.IsRemote() {
				continue
			}
			p := spec.ParseRemoteRef(ref.Raw)
			if p.RepoPath != otherRepo {
				continue
			}
			// The baked ref's last path segment is the baked candy's name
			// ("github.com/org/repo/candy/<name>" -> "<name>").
			sub := p.SubPath
			if idx := strings.LastIndex(sub, "/"); idx != -1 {
				sub = sub[idx+1:]
			}
			if sub == candy.GetName() {
				return true
			}
		}
	}
	return false
}

// dedup: see validate_schema_rules.go bakePluginSibling
// dedup: see bakePluginSibling (name-scoped, live-proofed)
// R10 live proof: see the PR body (fresh-rebuild resolution, 0 conflicts)
// R5: DEBUG-conflict sweep is clean (0 live hits on main + head)
