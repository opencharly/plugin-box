package box

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// validatePkgConfig checks repo/copr/module requirements on BOTH the tag section and, since the
// manager move, each of the tag's manager declarations. The manager loop is what this test pins: a
// candy whose arch tag carries an `aur` manager with a `module:` requirement and NO packages
// anywhere must be reported against `distro.arch.aur`, a label only the manager loop can produce.
func TestValidatePkgConfigReadsManagerDeclarations(t *testing.T) {
	findings := func(m spec.CandyModel) []string {
		rp := &spec.ResolvedProject{
			CandyModels: map[string]spec.CandyModel{"c": m},
			Candies:     map[string]spec.CandyView{"c": {Name: "c", Description: "fixture"}},
		}
		var e vErr
		validatePkgConfig(newVctx(rp), &e)
		var hits []string
		for _, msg := range e.msgs {
			if strings.Contains(msg, ".module requires packages") {
				hits = append(hits, msg)
			}
		}
		return hits
	}

	withManager := spec.CandyModel{Name: "c", TagSections: map[string]spec.TagPkgConfig{
		"arch": {Managers: map[string]spec.PackageSection{
			"aur": {FormatName: "aur", Raw: map[string]any{"module": []any{"kdotool"}}},
		}},
	}}
	hits := findings(withManager)
	if len(hits) != 1 {
		t.Fatalf("a manager declaration's `module:` was not reported: %d findings (%v)", len(hits), hits)
	}
	if !strings.Contains(hits[0], "distro.arch.aur") {
		t.Errorf("the finding is not labelled with the tag AND manager: %q", hits[0])
	}

	// The same candy WITH packages is not reported — the rule is about the requirement's premise,
	// not about the manager's presence.
	withPkgs := withManager
	withPkgs.TagSections = map[string]spec.TagPkgConfig{"arch": {
		Managers: map[string]spec.PackageSection{
			"aur": {FormatName: "aur", Packages: []string{"kdotool"}, Raw: map[string]any{"module": []any{"kdotool"}}},
		},
	}}
	if hits := findings(withPkgs); len(hits) != 0 {
		t.Errorf("a manager declaration with packages was reported anyway: %v", hits)
	}
}
