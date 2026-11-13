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
// It fails on an ASSERTION without the change only in the sense that matters for the cutover: the
// decode target is the envelope, and a regression to the authored type would break the build once
// the contract leg drops `spec.Local` — while a silent decode failure would make every template
// pass here, which the "missing required field" and "candy not found" cases below catch.

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
