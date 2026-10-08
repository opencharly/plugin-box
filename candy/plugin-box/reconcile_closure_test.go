package box

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// closureFixture writes a manifest at dir/<rel> and returns its path.
func closureFixture(t *testing.T, dir, rel, body string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestClosureScan_SeesARefOnlyTheClosureNames is the R7 gate on opencharly/charly#737's closure
// mode. The project pins repo-b at v1; a repo it pins (repo-a@v1) pins repo-b at v2. Only a walk
// that descends into the CLOSURE sees v2, so only a closure-wide target bumps the local pin —
// which is exactly the partial-repin-increases-skew defect: a per-project view reads "already
// reconciled" while the composed tree is skewed.
func TestClosureScan_SeesARefOnlyTheClosureNames(t *testing.T) {
	proj := t.TempDir()
	local := closureFixture(t, proj, "charly.yml", `discover:
    - path: candy
      recursive: true
`)
	f := closureFixture(t, proj, "candy/x/charly.yml", `x:
    candy:
        require:
            - '@github.com/opencharly/repo-a/candy/a:v1'
            - '@github.com/opencharly/repo-b/candy/b:v1'
        plan:
            - check: fixture
              id: fixture-ok
              context:
                  - build
              command: "true"
`)

	// The fetched closure: repo-a@v1's own manifest names repo-b at v2.
	exportA := t.TempDir()
	closureFixture(t, exportA, "candy/a/charly.yml", `a:
    candy:
        require:
            - '@github.com/opencharly/repo-b/candy/b:v2'
        plan:
            - check: fixture
              id: fixture-ok
              context:
                  - build
              command: "true"
`)

	orig := closureFetchFn
	defer func() { closureFetchFn = orig }()
	closureFetchFn = func(repo, version string) (string, error) {
		if repo == "github.com/opencharly/repo-a" && version == "v1" {
			return exportA, nil
		}
		return "", os.ErrNotExist
	}

	versions, _, err := closureScan([]string{local, f})
	if err != nil {
		t.Fatalf("closureScan: %v", err)
	}
	got := versions["github.com/opencharly/repo-b"]
	if !got["v2"] {
		t.Fatalf("the closure-wide version set for repo-b is %v — a ref only the CLOSURE names "+
			"(v2, from repo-a@v1's own manifest) was not seen, so a partial repin can never be "+
			"aligned (opencharly/charly#737)", got)
	}
	// The project's OWN files still contribute: v1 is there too, so the target is the newest
	// (v2) and the local pin is the thing that gets rewritten.
	if !got["v1"] {
		t.Fatalf("the project's own repo-b:v1 must still be in the set, got %v", got)
	}
	if newest, terr := reconcileTargetVersion(false, "github.com/opencharly/repo-b", got); terr != nil || newest != "v2" {
		t.Fatalf("closure-wide target for repo-b = %q (err %v), want v2 (the newest referenced "+
			"across the closure)", newest, terr)
	}
}

// TestClosureScan_ReportsForeignSkewItCannotRepair — the third leg. A pin that lives in ANOTHER
// repo of the closure is that repo's to advance; the project must name it rather than let its own
// "already reconciled" stand for the whole tree.
func TestClosureScan_ReportsForeignSkewItCannotRepair(t *testing.T) {
	proj := t.TempDir()
	local := closureFixture(t, proj, "charly.yml", `discover:
    - path: candy
      recursive: true
`)
	f := closureFixture(t, proj, "candy/x/charly.yml", `x:
    candy:
        require:
            - '@github.com/opencharly/repo-a/candy/a:v1'
            - '@github.com/opencharly/repo-c/candy/c:v2'
        plan:
            - check: fixture
              id: fixture-ok
              context:
                  - build
              command: "true"
`)

	// repo-a@v1's manifest pins repo-c at v1 — BEHIND the v2 the project itself composes.
	exportA := t.TempDir()
	closureFixture(t, exportA, "candy/a/charly.yml", `a:
    candy:
        require:
            - '@github.com/opencharly/repo-c/candy/c:v1'
        plan:
            - check: fixture
              id: fixture-ok
              context:
                  - build
              command: "true"
`)

	orig := closureFetchFn
	defer func() { closureFetchFn = orig }()
	closureFetchFn = func(repo, version string) (string, error) {
		if repo == "github.com/opencharly/repo-a" && version == "v1" {
			return exportA, nil
		}
		return "", os.ErrNotExist
	}

	versions, foreign, err := closureScan([]string{local, f})
	if err != nil {
		t.Fatalf("closureScan: %v", err)
	}
	if newest, terr := reconcileTargetVersion(false, "github.com/opencharly/repo-c", versions["github.com/opencharly/repo-c"]); terr != nil || newest != "v2" {
		t.Fatalf("repo-c target = %q (err %v), want v2", newest, terr)
	}
	var reported bool
	for _, fp := range foreign {
		if fp.repo == "github.com/opencharly/repo-c" && fp.version == "v1" && strings.Contains(fp.file, "candy/a") {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("the FOREIGN pin repo-c:v1 (in repo-a@v1's manifest) must be reported as skew this "+
			"project cannot repair — got %+v", foreign)
	}
}

// TestClosureScan_ReportsAnUnfetchableRef — a ref the walk cannot fetch is a fact the reader needs,
// never a silent skip: a half-known closure must not read as a fully-reconciled one.
func TestClosureScan_ReportsAnUnfetchableRef(t *testing.T) {
	proj := t.TempDir()
	local := closureFixture(t, proj, "charly.yml", `discover:
    - path: candy
      recursive: true
`)
	f := closureFixture(t, proj, "candy/x/charly.yml", `x:
    candy:
        require:
            - '@github.com/opencharly/repo-missing/candy/m:v9'
        plan:
            - check: fixture
              id: fixture-ok
              context:
                  - build
              command: "true"
`)

	orig := closureFetchFn
	defer func() { closureFetchFn = orig }()
	closureFetchFn = func(repo, version string) (string, error) { return "", os.ErrNotExist }

	_, foreign, err := closureScan([]string{local, f})
	if err != nil {
		t.Fatalf("closureScan must report, not fail: %v", err)
	}
	var reported bool
	for _, fp := range foreign {
		if fp.source == "github.com/opencharly/repo-missing@v9" {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("an unfetchable closure ref must be REPORTED, got %+v", foreign)
	}
}
