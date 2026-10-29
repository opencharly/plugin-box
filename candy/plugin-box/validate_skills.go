package box

import (
	"encoding/json"
	"sort"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/spec/spec"
)

// validateSkills is the engine's ONE skill-node entry: it rejects the OWNERSHIP shapes a skill may
// not have, then resolves every remaining node's body.
//
// The two stages are ORDERED, and the order is load-bearing. A node whose owner is an image is
// reported by the shape stage and then REMOVED from the body walk, because the body walk's message
// for an absent owner is a statement about a file search — and for an image owner no search ever runs
// (deploykit's readSkillFile returns before a path is composed). Reporting the shape first keeps
// exactly one message per node, and the accurate one.
func validateSkills(cfg *spec.Config, dk map[string]deploykit.CandyModel, e *vErr) {
	validateSkillBodies(withoutSkills(cfg, validateSkillOwners(cfg, dk, e)), dk, e)
}

// validateSkillBodies reports every authored `skill:` node whose body does not resolve at
// <the owning candy's REPO ROOT>/.agents/skills/<name>/SKILL.md.
//
// The base is the REPO ROOT — the directory a harness rooted at that repo discovers natively —
// and NOT the candy's own directory. The two coincide only for a root-level standalone candy;
// for a candy under `candy/<name>/` they diverge, and the sdk derives the root structurally
// (deploykit/candy_repo_root.go → candyRepoRoot) so a repository cannot be "found" by the
// presence of an .agents tree. One base, one layout, two readers.
//
// It exists because CollectSkills reads a body only for skills whose owner is in a composed box chain
// — a LABEL-SIZE policy, not a statement about whether a body must exist. A repo that authors skill
// nodes but composes no box therefore had no check that could fail on an absent or misplaced body,
// which is how a migration moved five bodies to the repository root and shipped "0 warnings, 0 errors"
// (opencharly/opencharly#530).
//
// Separate from runValidateEngine so the behaviour is testable without a resolved-project envelope.
func validateSkillBodies(cfg *spec.Config, dk map[string]deploykit.CandyModel, e *vErr) {
	if cfg == nil {
		return
	}
	if _, err := deploykit.CollectSkillBodies(cfg, dk); err != nil {
		e.Add("skills: %v", err)
	}
}

// imageOwnerFix is the remedy the operator's ruling names, quoted verbatim in the diagnostic so the
// author reads the action and not only the prohibition (opencharly/sdk#434).
const imageOwnerFix = "move this skill to a package candy that the image composes, and set `owner:` to that candy"

// validateSkillOwners rejects the shape the operator ruled out (opencharly/sdk#434): a `skill:` whose
// `owner:` names an IMAGE — a `candy:` node carrying `base:` or `from:`.
//
// An image owner cannot work, and the reason is structural rather than a missing search path: the
// loader folds a `base:`/`from:` node into the BOX set and never the layer set, and only a candy model
// carries a source directory (`source_dir` exists on `#CandyModel`, and `#Box` has no directory field
// at all). So no placement of the body can make it reachable — deploykit's readSkillFile returns
// before a path is composed — and the node is SILENTLY absent from the `ai.opencharly.skill` label as
// well, because that projection filters by composed-candy membership and an image is never a member of
// its own candy chain. Re-homing the skill onto a package candy the image composes is therefore the
// change that makes it reach the label for the FIRST time; this rule turns the silent case into a
// loud, actionable error.
//
// It fires on the SHAPE, independent of body presence: an image-owned node that still carries an
// in-line `content:` is forbidden for exactly the same reason as one that does not, and before this
// rule it validated clean (measured: rc=0).
//
// Only an owner that is an image AND is not also a candy is rejected. Four repos declare one name as
// BOTH (layer-agentteams/agentteams, layer-openclaw/openclaw, layer-strata/strata, pod-herdr/herdr);
// there the layer resolves the skill and the shape is legal, so the candy set wins. An owner that is
// neither is left to the body walk, whose message is the one that fits.
//
// Returns the node names it reported, so the caller can keep them out of the body walk.
func validateSkillOwners(cfg *spec.Config, dk map[string]deploykit.CandyModel, e *vErr) map[string]bool {
	if cfg == nil || len(cfg.Skills) == 0 {
		return nil
	}
	// SORTED node iteration, because a migration gate has to name the same node twice: walking the
	// authored map directly made the reported node depend on Go's per-run map order (the same defect
	// sdk#435 fixed one layer down).
	nodes := make([]string, 0, len(cfg.Skills))
	for node := range cfg.Skills {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	var reported map[string]bool
	for _, node := range nodes {
		s, ok := decodeSkillNode(cfg.Skills[node])
		if !ok || s.Owner == "" {
			continue // an undecodable body, or a node with no owner, is another rule's finding
		}
		if _, isCandy := dk[s.Owner]; isCandy {
			continue
		}
		if _, isImage := cfg.Box[s.Owner]; !isImage {
			continue // neither a candy nor an image: the body walk reports the unresolved owner
		}
		if reported == nil {
			reported = map[string]bool{}
		}
		reported[node] = true
		e.Add("skills: skill %q: owner %q is an image, not a candy — a `candy:` node carrying `base:` or `from:` folds into the image set, so it has no source directory and this skill's prose can never be read (it is also silently absent from the ai.opencharly.skill label): %s (opencharly/sdk#434)",
			node, s.Owner, imageOwnerFix)
	}
	return reported
}

// decodeSkillNode decodes ONE authored `skill:` body. The same few lines live in deploykit as its
// private decodeSkillNodes; a pure decode helper duplicated across the core/plugin MODULE boundary is
// the established pattern (this package's own header names it), and reaching it from here would mean
// exporting an sdk seam for a single caller.
func decodeSkillNode(raw json.RawMessage) (spec.Skill, bool) {
	var s spec.Skill
	if len(raw) == 0 {
		return s, false
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, false
	}
	return s, true
}

// withoutSkills returns a copy of cfg whose `skill:` nodes in drop are absent, so a stage that already
// reported those nodes does not re-report them under a message that fits a different fault. A nil or
// empty drop returns cfg unchanged — a clean project is the common case and must not be copied.
func withoutSkills(cfg *spec.Config, drop map[string]bool) *spec.Config {
	if cfg == nil || len(drop) == 0 {
		return cfg
	}
	kept := make(map[string]json.RawMessage, len(cfg.Skills))
	for node, raw := range cfg.Skills {
		if drop[node] {
			continue
		}
		kept[node] = raw
	}
	filtered := *cfg
	filtered.Skills = kept
	return &filtered
}
