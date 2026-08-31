package box

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A realistic candy file: authored prose in a FOLDED block scalar (`>-`), then pins. yaml.v3
// re-emits a folded scalar as one long line and drops the blank lines around it, which is
// exactly the damage a reconcile inflicted on a 160KB charly.yml — 4 pins changed, ~400 lines
// of diff. A literal scalar (`|-`) round-trips cleanly, so the fixture must be folded to
// reproduce the hazard at all.
const reconcileFixture = `demo:
    candy:
        version: 2026.243.1632
        description: >-
            Sole proof of the punktfunk CONTAINER venue and of the punktfunk check verb
            against a live host. Builds punktfunk-host on cachyos, deploys it with a headless
            sway compositor, and runs the candy's baked package/key/host.env/wrapper checks
            plus the verb's own health, status, compositors and clients probes.

        candy:
            - '@github.com/opencharly/pod-pipewire:v2026.239.1555'
            - '@github.com/opencharly/layer-ffmpeg:v2026.239.1614'
            - '@github.com/opencharly/layer-xdg-portal:v2026.239.1637'
`

// The whole point of the fix: a pin rewrite must change the pin lines and NOTHING else.
// Re-serializing the parsed document reflows every block scalar in the file, which turned a
// 4-pin reconcile into a ~400-line diff across unrelated entities' prose.
func TestApplyPinEdits_TouchesOnlyThePinLines(t *testing.T) {
	src := []byte(reconcileFixture)
	lineOf := func(sub string) int {
		for i, l := range strings.Split(reconcileFixture, "\n") {
			if strings.Contains(l, sub) {
				return i + 1
			}
		}
		t.Fatalf("fixture has no line containing %q", sub)
		return 0
	}
	pipewireLine, portalLine := lineOf("pod-pipewire:v2026.239.1555"), lineOf("layer-xdg-portal:v2026.239.1637")
	edits := []pinEdit{
		{line: pipewireLine, from: "@github.com/opencharly/pod-pipewire:v2026.239.1555", to: "@github.com/opencharly/pod-pipewire:v2026.243.0508"},
		{line: portalLine, from: "@github.com/opencharly/layer-xdg-portal:v2026.239.1637", to: "@github.com/opencharly/layer-xdg-portal:v2026.243.1016"},
	}
	out, err := applyPinEdits(src, edits)
	if err != nil {
		t.Fatalf("applyPinEdits: %v", err)
	}
	in, got := strings.Split(string(src), "\n"), strings.Split(string(out), "\n")
	if len(in) != len(got) {
		t.Fatalf("line count changed: %d -> %d (a pin rewrite must not add or remove lines)", len(in), len(got))
	}
	for i := range in {
		changed := in[i] != got[i]
		isPinLine := i == pipewireLine-1 || i == portalLine-1
		if changed && !isPinLine {
			t.Errorf("line %d changed but carries no edited pin:\n  before: %q\n  after:  %q", i+1, in[i], got[i])
		}
		if !changed && isPinLine {
			t.Errorf("line %d carries an edited pin but did not change: %q", i+1, in[i])
		}
	}
	if !strings.Contains(string(out), "pod-pipewire:v2026.243.0508") {
		t.Error("the new pin is missing from the output")
	}
}

// Negative control, and the reason this code exists: the yaml.Marshal round-trip that
// applyPinEdits replaces does NOT preserve the file. If this ever starts passing, yaml.v3
// gained faithful round-tripping and the line-local rewrite could be reconsidered.
func TestYAMLRoundTripReflowsAuthoredProse(t *testing.T) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(reconcileFixture), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) == reconcileFixture {
		t.Skip("yaml.v3 now round-trips this document byte-exactly; the reflow hazard is gone")
	}
	// Demonstrate the specific damage: the authored line breaks inside the block scalar are
	// not reproduced, so unrelated prose lines show up in the diff.
	if strings.Contains(string(out), "against a live host. Builds punktfunk-host on cachyos, deploys it with a headless\n") {
		t.Error("expected the round trip to reflow the folded scalar; it did not — re-check the hazard")
	}
	t.Logf("round trip rewrote the document: %d bytes -> %d bytes", len(reconcileFixture), len(out))
}

// Repairing a file on a position the parser and the bytes disagree about would corrupt it, so
// the rewrite refuses rather than replacing the value somewhere else.
func TestApplyPinEdits_RefusesPositionMismatch(t *testing.T) {
	cases := []struct {
		name string
		edit pinEdit
	}{
		{"line past the end", pinEdit{line: 999, from: "@github.com/opencharly/pod-pipewire:v2026.239.1555", to: "x"}},
		{"line zero", pinEdit{line: 0, from: "@github.com/opencharly/pod-pipewire:v2026.239.1555", to: "x"}},
		{"value not on that line", pinEdit{line: 1, from: "@github.com/opencharly/pod-pipewire:v2026.239.1555", to: "x"}},
	}
	for _, c := range cases {
		out, err := applyPinEdits([]byte(reconcileFixture), []pinEdit{c.edit})
		if err == nil {
			t.Errorf("%s: expected an error, got a rewritten file", c.name)
		}
		if out != nil {
			t.Errorf("%s: expected no output on failure, got %d bytes", c.name, len(out))
		}
	}
}
