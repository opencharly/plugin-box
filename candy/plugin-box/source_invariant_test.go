package box

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestThisPackageDoesNotNameTheAuthoredCandyDefs is the test that FAILS without this change, and it
// is a SOURCE invariant rather than a behavioural one — deliberately, and here is why.
//
// The resolved twins (`spec.ResolvedCandyAlias`, `spec.ResolvedCandyVolume`) are WIRE-COMPATIBLE with
// the authored defs by design: same field names, same JSON, because they answer the same decoding
// question about different inputs (one validated, one not). A round-trip therefore passes
// identically whichever type a site names — measured: reverting a decode target to
// `spec.CandyAlias` leaves every behavioural test in this package GREEN.
//
// So "coverage that fails without the change" has to test the thing that actually changed: WHICH
// TYPE THE CODE NAMES. That is a property of the source, and this test reads the source.
//
// Why it matters beyond this file: the authored defs move to plugin-candy-kind (DESIGN.md 3.1). A
// decode target still naming them would travel with them, and no behavioural test would notice
// until the move broke the build somewhere else.
func TestThisPackageDoesNotNameTheAuthoredCandyDefs(t *testing.T) {
	forbidden := []string{
		"spec.CandyAlias",  // the authored shapes — the resolved twins are what this package decodes
		"spec.CandyVolume", //
		"spec.AliasYAML",   //
		"spec.VolumeYAML",  //
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}

	var checked int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		checked++
		for _, bad := range forbidden {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s names %s — this package decodes the RESOLVED twins; naming the authored def "+
					"keeps it referenced by a consumer and stops it moving to plugin-candy-kind", name, bad)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no .go files scanned — this test would pass over an empty set")
	}
}
