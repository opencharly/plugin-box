package box

import (
	"strings"
	"testing"

	"github.com/opencharly/sdk/rawbody"
	"github.com/opencharly/spec/spec"
)

// TestListVolumesAndAliasesDecodeTheOpaqueViews covers the TWO `list` commands that read a
// `spec.CandyView` directly. They were the untested half of the view conversion: the inspect path's
// fixture encoded a `ResolvedBoxView`, which those two never touch.
//
// Both views carry their lists as OPAQUE BYTES (spec#336), so each command must decode before
// printing. These tests build the bodies with the PRODUCER's encoder and assert the printed rows, so
// a revert to reading the slice prints nothing and fails here.
func TestListVolumesAndAliasesDecodeTheOpaqueViews(t *testing.T) {
	rp := &spec.ResolvedProject{
		Candies: map[string]spec.CandyView{
			"sshd": {
				Name:    "sshd",
				Volumes: rawbody.Encode([]spec.ResolvedCandyVolume{{Name: "sshconf", Path: "/etc/ssh"}}, "volumes"),
				Aliases: rawbody.Encode([]spec.ResolvedCandyAlias{{Name: "sshd-alias", Command: "/usr/sbin/sshd"}}, "aliases"),
			},
		},
	}

	t.Run("box list volumes", func(t *testing.T) {
		out := captureStdout(t, func() { listVolumes(rp) })
		if !strings.Contains(out, "sshd\tsshconf\t/etc/ssh") {
			t.Fatalf("listVolumes printed %q, want the decoded volume row", out)
		}
	})

	t.Run("box list aliases", func(t *testing.T) {
		out := captureStdout(t, func() { listAliases(rp) })
		if !strings.Contains(out, "sshd\tsshd-alias\t/usr/sbin/sshd") {
			t.Fatalf("listAliases printed %q, want the decoded alias row", out)
		}
	})

	t.Run("an empty view prints nothing", func(t *testing.T) {
		empty := &spec.ResolvedProject{Candies: map[string]spec.CandyView{"bare": {Name: "bare"}}}
		if out := captureStdout(t, func() { listVolumes(empty) }); out != "" {
			t.Fatalf("listVolumes printed %q for a candy with no volumes", out)
		}
		if out := captureStdout(t, func() { listAliases(empty) }); out != "" {
			t.Fatalf("listAliases printed %q for a candy with no aliases", out)
		}
	})
}
