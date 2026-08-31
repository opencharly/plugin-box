package box

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// A gate that prints NOTHING on success cannot be used as evidence: a PR body can only assert
// "it passed", and a reviewer asking for pasted output has nothing to read. These guards pin the
// success line's shape so it stays quotable.
func TestValidateSummaryLine(t *testing.T) {
	s := validateSummary{Candies: 42, Boxes: 7, Deploys: 3, Distros: 2, Builders: 1, Warnings: 0}
	got := s.String()
	want := "charly box validate: OK — checked 42 candies, 7 boxes, 3 deploys, 2 distros, 1 builder; 0 warnings, 0 errors"
	if got != want {
		t.Errorf("summary line\n got: %q\nwant: %q", got, want)
	}
}

// Zero-valued collections are omitted so a single-candy repo does not read as
// "1 candy, 0 boxes, 0 deploys, 0 distros, 0 builders". Warnings are the exception: "0 warnings"
// is the load-bearing half of the claim, and omitting it would read as though none were counted.
func TestValidateSummaryOmitsEmptyButAlwaysShowsWarnings(t *testing.T) {
	got := validateSummary{Candies: 1}.String()
	want := "charly box validate: OK — checked 1 candy; 0 warnings, 0 errors"
	if got != want {
		t.Errorf("single-candy summary\n got: %q\nwant: %q", got, want)
	}
	if !strings.Contains(validateSummary{}.String(), "nothing to check") {
		t.Errorf("an empty project must say so, got %q", validateSummary{}.String())
	}
}

// The warning count must reflect the SAME diagnostics the error verdict filtered out, not a
// separate tally. Scan advisories used to bypass diagnostics entirely (stderr writes in
// sdk/loaderkit), so an early version of this line printed "0 warnings" on a run that had just
// emitted two; they now arrive as warning-severity items via spec.ScanSeams.Warn.
func TestValidateSummaryCountsWarningsFromTheSameDiagnostics(t *testing.T) {
	diags := spec.Diagnostics{Items: []spec.Diagnostic{
		{Severity: "warning", Message: "candy X resolved to multiple versions"},
		{Severity: "warning", Message: "local candy shadows remote"},
		{Severity: "error", Message: "a real failure"},
	}}
	s := summarize(&spec.ResolvedProject{}, diags)
	if s.Warnings != 2 {
		t.Errorf("Warnings = %d, want 2 (errors must not be counted as warnings)", s.Warnings)
	}
	if !strings.Contains(s.String(), "2 warnings") {
		t.Errorf("the line must report the real count, got %q", s.String())
	}
}

// The success line must be printed ONLY on success — a run with errors returns the verdict error
// and must not also emit an "OK" line a reader could quote.
func TestValidateSummaryNotEmittedWhenErrorsExist(t *testing.T) {
	err := emitVerdict(spec.Diagnostics{Items: []spec.Diagnostic{
		{Severity: "error", Message: "boom"},
	}}, validateSummary{Candies: 3})
	if err == nil {
		t.Fatal("expected a verdict error")
	}
	if strings.Contains(err.Error(), "OK") {
		t.Errorf("verdict must not carry the OK line, got %q", err.Error())
	}
}

// The point of the summary is that it REACHES stdout on success — a body can only paste what the
// command actually printed. Asserting the string's shape is not enough: deleting the Println left
// every other guard here green, so this captures stdout and proves the line is emitted.
func TestValidateSummaryIsActuallyPrintedOnSuccess(t *testing.T) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	verr := emitVerdict(spec.Diagnostics{}, validateSummary{Candies: 2, Boxes: 1})
	w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	if _, cerr := io.Copy(&buf, r); cerr != nil {
		t.Fatalf("read: %v", cerr)
	}
	if verr != nil {
		t.Fatalf("clean diagnostics must not error: %v", verr)
	}
	got := strings.TrimSpace(buf.String())
	want := "charly box validate: OK — checked 2 candies, 1 box; 0 warnings, 0 errors"
	if got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

// And it must stay silent when the run failed, so a failing gate cannot be quoted as passing.
func TestValidateSummaryPrintsNothingOnFailure(t *testing.T) {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	_ = emitVerdict(spec.Diagnostics{Items: []spec.Diagnostic{{Severity: "error", Message: "boom"}}},
		validateSummary{Candies: 2})
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("a failing validate must print no OK line, got %q", buf.String())
	}
}
