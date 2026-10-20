package box

import (
	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/spec/spec"
)

// validateSkillBodies reports every authored `skill:` node whose body does not resolve at
// <owning candy SourceDir>/.agents/skills/<name>/SKILL.md.
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
