package box

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// The defect this covers (opencharly/distro-cachyos#163): `box validate` printed
//
//	Warning: box "krun-host" includes candy … packaged-only service …
//	charly box validate: OK — checked … 0 warnings, 0 errors
//
// in ONE run. The rule wrote to stderr; the summary counts warning-severity DIAGNOSTICS, so the
// finding was invisible to the number printed right beneath it — a control reporting green over a
// finding. The fix routes the warning through the same channel the count reads, and this test is
// the one that fails if a rule ever goes back to a bare Fprintf.
func TestWarningFindingIsCounted(t *testing.T) {
	e := &vErr{}
	e.Add("a hard finding")
	e.Warn("box %q includes candy %q with a packaged-only service", "krun-host", "layer-containerd")

	merged := mergePluginFindings(spec.Diagnostics{}, e)

	if len(merged.Items) != 2 {
		t.Fatalf("want 2 diagnostics (1 error, 1 warning), got %d: %+v", len(merged.Items), merged.Items)
	}
	// The ORDER is part of the contract: errors are read before warnings.
	if merged.Items[0].Severity != "error" || merged.Items[1].Severity != "warning" {
		t.Fatalf("want error then warning, got %q then %q", merged.Items[0].Severity, merged.Items[1].Severity)
	}

	// The count the operator reads must name the warning the reader sees.
	warnings := 0
	for _, it := range merged.Items {
		if it.Severity == "warning" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("want 1 counted warning, got %d", warnings)
	}
	s := summarize(&spec.ResolvedProject{}, merged)
	if s.Warnings != warnings {
		t.Errorf("summary says %d warnings but %d were emitted: the number and the message disagree", s.Warnings, warnings)
	}
	if !contains(s.String(), "1 warning") {
		t.Errorf("the verdict line must name the warning, got: %s", s.String())
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
