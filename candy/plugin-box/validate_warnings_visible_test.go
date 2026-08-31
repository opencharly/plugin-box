package box

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// captureStderr runs fn with os.Stderr redirected, returning what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy: %v", err)
	}
	return buf.String()
}

// Routing the scan's advisories into diagnostics made them COUNTABLE, but the verdict skipped
// warning-severity items on the way out. The result was a run reporting "10 warnings" and never
// saying what any of them were -- strictly worse than the stderr writes it replaced, because a
// count nobody can act on is not a gate. This guard pins that warnings reach the operator.
func TestEmitVerdictPrintsWarnings(t *testing.T) {
	diags := spec.Diagnostics{Items: []spec.Diagnostic{
		{Severity: "warning", Message: `candy "sway-desktop" resolved to multiple versions`},
		{Severity: "warning", Message: `local candy "punktfunk" shadows a pinned ref`},
	}}
	var err error
	out := captureStderr(t, func() {
		err = emitVerdict(diags, validateSummary{Candies: 3, Warnings: 2})
	})
	if err != nil {
		t.Fatalf("warnings alone must not fail the gate, got: %v", err)
	}
	for _, want := range []string{
		`candy "sway-desktop" resolved to multiple versions`,
		`local candy "punktfunk" shadows a pinned ref`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("warning not printed: %q\nstderr was:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "warning: "); n != 2 {
		t.Errorf(`expected 2 "warning: " lines, got %d:\n%s`, n, out)
	}
}

// A count without the messages is the defect this fixes: the number must never be the ONLY
// thing an operator gets. If the summary reports N warnings, N warning lines must precede it.
func TestWarningCountMatchesPrintedLines(t *testing.T) {
	items := make([]spec.Diagnostic, 0, 5)
	for i := 0; i < 5; i++ {
		items = append(items, spec.Diagnostic{Severity: "warning", Message: "advisory"})
	}
	diags := spec.Diagnostics{Items: items}
	summary := summarize(nil, diags)
	if summary.Warnings != 5 {
		t.Fatalf("summarize counted %d, want 5", summary.Warnings)
	}
	out := captureStderr(t, func() { _ = emitVerdict(diags, summary) })
	if n := strings.Count(out, "warning: "); n != summary.Warnings {
		t.Errorf("summary says %d warnings but %d were printed", summary.Warnings, n)
	}
}

// Errors must still surface as the returned verdict, and warnings must not be swallowed when
// errors are also present -- otherwise a failing run hides the advisories that explain it.
func TestWarningsPrintedAlongsideErrors(t *testing.T) {
	diags := spec.Diagnostics{Items: []spec.Diagnostic{
		{Severity: "warning", Message: "an advisory"},
		{Severity: "error", Message: "a real failure"},
	}}
	var err error
	out := captureStderr(t, func() {
		err = emitVerdict(diags, validateSummary{Candies: 1, Warnings: 1})
	})
	if err == nil {
		t.Fatal("an error-severity item must still produce a verdict error")
	}
	if !strings.Contains(err.Error(), "a real failure") {
		t.Errorf("verdict lost the error: %v", err)
	}
	if !strings.Contains(out, "an advisory") {
		t.Errorf("warning swallowed when an error was present; stderr:\n%s", out)
	}
}
