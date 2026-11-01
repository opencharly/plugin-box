package box

import (
	"reflect"
	"testing"
)

// box_load_kind_test.go — coverage for `charly box load --kind <cluster>`, the node-venue
// binding (the mechanism opencharly/charly#809 charters). The full path is
// `podman save <ref> | <engine> exec -i <cluster>-control-plane ctr -n k8s.io images import -`;
// the two contract-bearing pieces are pinned here as pure functions, and the load side is
// proven live against a real kind cluster in the PR body.

// TestKindControlPlaneNode pins kind's node-naming convention: a cluster's node is
// `<cluster>-control-plane` (the name kind provisions and plugin-kube's kindNodeHasImage
// probes). Getting this wrong addresses a nonexistent container.
func TestKindControlPlaneNode(t *testing.T) {
	for _, tc := range []struct{ cluster, want string }{
		{"spike", "spike-control-plane"},
		{"check-boxload", "check-boxload-control-plane"},
		{"a-b-c", "a-b-c-control-plane"},
	} {
		if got := kindControlPlaneNode(tc.cluster); got != tc.want {
			t.Errorf("kindControlPlaneNode(%q) = %q, want %q", tc.cluster, got, tc.want)
		}
	}
}

// TestKindCtrImportArgv pins the in-node import argv: engine `exec` of `ctr -n k8s.io images
// import -`. The `-n k8s.io` namespace is what kubelet reads (so the image is visible to the
// cluster); the trailing `-` makes ctr read the archive on STDIN, so the host `save` stream
// pipes straight through and no tar ever lands on the node. Losing either breaks the seam.
func TestKindCtrImportArgv(t *testing.T) {
	got := kindCtrImportArgv("spike-control-plane")
	want := []string{"exec", "-i", "spike-control-plane", "ctr", "-n", "k8s.io", "images", "import", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kindCtrImportArgv = %q, want %q", got, want)
	}
	// The k8s.io namespace must be present — without it the image lands in a namespace the
	// kubelet never reads and the cluster cannot start the pod.
	has := func(s string) bool {
		for _, a := range got {
			if a == s {
				return true
			}
		}
		return false
	}
	if !has("k8s.io") {
		t.Errorf("import argv omits the k8s.io namespace: %q", got)
	}
	if got[len(got)-1] != "-" {
		t.Errorf("import argv must read stdin (trailing -): %q", got)
	}
}

// TestLoadGrammarKindFlag pins the CLI surface: `--kind` carries the cluster name and is
// distinct from the pod-path flags, so the two venues cannot be confused.
func TestLoadGrammarKindFlag(t *testing.T) {
	var g loadGrammar
	if done, err := parseLeaf("load", &g, []string{"mytarget", "quay.io/x/y:v1", "--kind", "spike"}); err != nil || done {
		t.Fatalf("parse: done=%v err=%v", done, err)
	}
	if g.Kind != "spike" || g.Target != "mytarget" || g.Image != "quay.io/x/y:v1" {
		t.Fatalf("parsed grammar wrong: %+v", g)
	}
	// No --kind → the pod path (Kind empty).
	var g2 loadGrammar
	if done, err := parseLeaf("load", &g2, []string{"mytarget", "quay.io/x/y:v1"}); err != nil || done {
		t.Fatalf("parse: done=%v err=%v", done, err)
	}
	if g2.Kind != "" {
		t.Fatalf("without --kind the pod path must be selected, got Kind=%q", g2.Kind)
	}
}
