package box

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// An INFO-tier diagnostic is a RESOLVABLE observation the resolver has already arbitrated —
// sdk/loaderkit emits the candy-version skew at DiagInfo and its own contract says "a resolvable
// skew is INFO ... must not fail on a successful arbitration". spec.SeverityError states the same
// rule from the other side: it is "the only tier a verdict may turn into a non-zero exit".
//
// Before this guard the verdict skipped just SeverityWarning, so SeverityInfo fell through, was
// printed inside "N validation errors:" and made `charly box validate` exit 1 — blocking every
// pin advance that left two candies in one box scope naming different tags of the same repo.
func TestValidateVerdictDoesNotFailOnInfoTier(t *testing.T) {
	diags := spec.Diagnostics{Items: []spec.Diagnostic{
		{Severity: spec.SeverityInfo, Message: "candy X resolved to multiple git tags with differing content within one scope (box=a); using newest referenced v2 (from repo@v2); also referenced v1 (from repo@v1)"},
		{Severity: spec.SeverityInfo, Message: "local candy X shadows remote candy Y"},
	}}

	outR, outW, perr := os.Pipe()
	if perr != nil {
		t.Fatalf("pipe stdout: %v", perr)
	}
	errR, errW, perr := os.Pipe()
	if perr != nil {
		t.Fatalf("pipe stderr: %v", perr)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	verr := emitVerdict(diags, validateSummary{Candies: 1})
	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = origOut, origErr

	var stdout, stderr bytes.Buffer
	if _, cerr := io.Copy(&stdout, outR); cerr != nil {
		t.Fatalf("read stdout: %v", cerr)
	}
	if _, cerr := io.Copy(&stderr, errR); cerr != nil {
		t.Fatalf("read stderr: %v", cerr)
	}

	if verr != nil {
		t.Fatalf("an INFO-tier, already-arbitrated skew must not fail the gate, got: %v", verr)
	}
	if !strings.Contains(stdout.String(), "OK") {
		t.Errorf("success must still print the OK line, got %q", stdout.String())
	}
	if got := strings.Count(stderr.String(), "notice: "); got != 2 {
		t.Errorf("both info diagnostics must be printed so they stay actionable; got %d notice line(s) in %q", got, stderr.String())
	}
	if strings.Contains(stderr.String(), "validation error") {
		t.Errorf("an info-tier run must not report a validation error, got %q", stderr.String())
	}
}

// The widening must not silence a real failure: an ERROR-tier finding still fails, and an
// UNKNOWN severity fails too (Diagnostic.IsError fails closed).
func TestValidateVerdictStillFailsOnErrorTier(t *testing.T) {
	for _, tc := range []struct {
		name string
		sev  string
	}{
		{"error tier", spec.SeverityError},
		{"unknown tier fails closed", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diags := spec.Diagnostics{Items: []spec.Diagnostic{
				{Severity: spec.SeverityInfo, Message: "an observation"},
				{Severity: tc.sev, Message: "a real failure"},
			}}
			verr := emitVerdict(diags, validateSummary{Candies: 1})
			if verr == nil || !strings.Contains(verr.Error(), "a real failure") {
				t.Fatalf("a %q finding must fail the verdict, got: %v", tc.sev, verr)
			}
		})
	}
}
