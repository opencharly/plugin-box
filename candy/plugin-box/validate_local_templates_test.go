package box

// validate_local_templates_test.go — the kind:local template rule's own coverage.
//
// opencharly/spec#304 unit 4: the authored `#Local` body (and its Go type `spec.Local`) moves into
// the plugin that SERVES the kind, so this package decodes the authored template into the
// contract's `spec.ResolvedLocal` envelope instead. This test pins that the rule still judges the
// authored `candy:` stack through the envelope: a missing stack, an unknown bare candy, a remote
// candy ref that does not resolve, and a known candy are four distinct outcomes, and a malformed
// body is a host/load concern this rule stays silent about.
//
// WHAT THIS TEST PINS, AND WHAT IT DOES NOT. It pins the RULE's judgement through the envelope —
// three distinct findings and two silent cases — and it fails on an assertion if the decode silently
// stops carrying a field: every template would then pass and the count assertion catches it.
//
// It does NOT fail on the decode-TARGET swap itself, and the review was right to say so: the
// authored type and the envelope carry the same JSON shape, so the swap is behaviour-preserving by
// construction. The witness that fails without the swap is a BUILD-level one and arrives with the
// contract leg that deletes the authored type (opencharly/spec#304 unit 4); until that leg lands,
// this change's tier is `analysed on a live system`, never `fully tested and validated`.

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

func TestValidateLocalTemplatesJudgesTheAuthoredCandyStack(t *testing.T) {
	rp := &spec.ResolvedProject{Templates: &spec.ProjectTemplates{Local: map[string]spec.RawBody{
		"known":     []byte(`{"candy":["present"]}`),
		"absent":    []byte(`{"candy":["missing-candy"]}`),
		"remote":    []byte(`{"candy":["@github.com/opencharly/nope:v1"]}`),
		"nocandy":   []byte(`{"plan":[{"run":"true"}]}`),
		"malformed": []byte(`{`),
	}}}
	vc := &vctx{env: rp, models: map[string]spec.CandyModel{"present": {}}}
	var e vErr
	validateLocalTemplates(vc, &e)

	if len(e.msgs) != 3 {
		t.Fatalf("validateLocalTemplates produced %d findings, want 3 (absent, remote, nocandy): %v", len(e.msgs), e.msgs)
	}
	for _, want := range []string{
		`kind:local "absent": candy "missing-candy" not found`,
		`kind:local "nocandy": missing required field ` + "`candy:`",
		`kind:local "remote": remote candy "@github.com/opencharly/nope:v1" not found`,
	} {
		found := false
		for _, m := range e.msgs {
			if strings.HasPrefix(m, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing finding with prefix %q; got %v", want, e.msgs)
		}
	}
	// The known candy and the malformed body produce NO finding: the first is valid, and a
	// body that will not decode is a host/load concern, not a validate finding.
	for _, m := range e.msgs {
		if strings.Contains(m, `"known"`) || strings.Contains(m, `"malformed"`) {
			t.Errorf("unexpected finding %q", m)
		}
	}
}
