package box

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// validate_skill_owner_test.go — a candy whose only content is the skill nodes it owns passes the
// install-content rule, because charly's generator installs every composed skill's body
// (opencharly/sdk#598); the same candy owning nothing still fails it, so the exemption is the
// ownership and not a loosened rule.

const installContentFinding = "must have at least one install file"

func contentsFindings(t *testing.T, skills []string) []string {
	t.Helper()
	rp := &spec.ResolvedProject{
		CandyModels: map[string]spec.CandyModel{"carrier": {
			Name:      "carrier",
			SourceDir: t.TempDir(),
			Skills:    skills,
			Plan:      []spec.Step{{Check: "a body is installed", Op: spec.Op{Command: "true"}}},
		}},
		Candies: map[string]spec.CandyView{"carrier": {Name: "carrier", Description: "owns skill nodes"}},
	}
	var e vErr
	validateCandyContents(newVctx(rp), &e)
	var hits []string
	for _, m := range e.msgs {
		if strings.Contains(m, installContentFinding) {
			hits = append(hits, m)
		}
	}
	return hits
}

func TestSkillOwnerIsInstallContent(t *testing.T) {
	if hits := contentsFindings(t, []string{"arch", "arch-coder"}); len(hits) != 0 {
		t.Fatalf("a candy owning skill nodes was refused for having no install content: %q", hits)
	}
}

func TestCandyOwningNothingStillNeedsInstallContent(t *testing.T) {
	hits := contentsFindings(t, nil)
	if len(hits) != 1 || !strings.Contains(hits[0], "an owned skill: node") {
		t.Fatalf("findings = %q, want the one install-content finding naming owned skill nodes as an option", hits)
	}
}
