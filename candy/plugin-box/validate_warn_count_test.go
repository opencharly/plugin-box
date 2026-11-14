package box

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// The defect this covers (opencharly/distro-cachyos#163): `box validate` printed
//
//	Warning: box "krun-host" includes candy … packaged-only service …
//	charly box validate: OK — checked … 0 warnings, 0 errors
//
// in ONE run. The rule wrote to stderr; the summary counts warning-severity DIAGNOSTICS, so the
// finding was invisible to the number printed right beneath it.
//
// This test DRIVES the rule over a fixture, so reverting validatePackagedServices to a bare
// `fmt.Fprintf(os.Stderr, …)` makes it fail: e.warns would be empty and the count would be 0 while
// the reader still saw a warning.
func TestPackagedServiceWarningIsCounted(t *testing.T) {
	packagedOnly := spec.CandyModel{Service: []spec.CandyService{
		{Name: "buildkit", UsePackaged: "buildkit.service"},
	}}

	t.Run("no preserve_user — the warning is emitted AND counted", func(t *testing.T) {
		vc := &vctx{
			models: map[string]spec.CandyModel{"layer-containerd": packagedOnly},
			views:  map[string]spec.CandyView{},
			boxes: map[string]spec.ResolvedBoxView{
				"krun-host": {Candy: []string{"layer-containerd"}},
			},
		}
		e := &vErr{}
		validatePackagedServices(vc, e)

		if len(e.warns) != 1 {
			t.Fatalf("want 1 warning finding, got %d: %v", len(e.warns), e.warns)
		}
		if !strings.Contains(e.warns[0], "krun-host") || !strings.Contains(e.warns[0], "layer-containerd") {
			t.Errorf("the warning must name the box and the candy, got: %s", e.warns[0])
		}

		merged := mergePluginFindings(spec.Diagnostics{}, e)
		s := summarize(&spec.ResolvedProject{}, merged)
		if s.Warnings != 1 {
			t.Fatalf("summary says %d warnings, the rule emitted %d — the number and the message disagree", s.Warnings, len(e.warns))
		}
		if !strings.Contains(s.String(), "1 warning") {
			t.Errorf("the verdict line must name the warning, got: %s", s.String())
		}
	})

	t.Run("preserve_user — silent, and the count agrees", func(t *testing.T) {
		vc := &vctx{
			models: map[string]spec.CandyModel{"layer-containerd": packagedOnly},
			views: map[string]spec.CandyView{
				"layer-containerd": {Capabilities: &spec.CandyCapabilitiesView{PreserveUser: true}},
			},
			boxes: map[string]spec.ResolvedBoxView{
				"krun-host": {Candy: []string{"layer-containerd"}},
			},
		}
		e := &vErr{}
		validatePackagedServices(vc, e)

		if len(e.warns) != 0 {
			t.Fatalf("a preserve_user composition must not warn, got: %v", e.warns)
		}
		if s := summarize(&spec.ResolvedProject{}, mergePluginFindings(spec.Diagnostics{}, e)); s.Warnings != 0 {
			t.Errorf("want 0 warnings, got %d", s.Warnings)
		}
	})
}

// The order contract: errors are read before warnings, whichever order the rules ran in.
func TestMergePluginFindingsOrderAndSeverity(t *testing.T) {
	e := &vErr{}
	e.Warn("a warning")
	e.Add("an error")

	merged := mergePluginFindings(spec.Diagnostics{}, e)
	if len(merged.Items) != 2 {
		t.Fatalf("want 2 diagnostics, got %d", len(merged.Items))
	}
	if merged.Items[0].Severity != "error" || merged.Items[1].Severity != "warning" {
		t.Fatalf("want error then warning, got %q then %q", merged.Items[0].Severity, merged.Items[1].Severity)
	}
}
