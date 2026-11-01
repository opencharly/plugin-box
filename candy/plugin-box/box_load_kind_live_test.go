package box

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// box_load_kind_live_test.go — the LIVE gate for `charly box load --kind <cluster>`: it runs
// the REAL changed code path (dispatchLoad → loadIntoKindNode → deploykit.NewNodeVenue →
// TransferImageToVenue) against a REAL kind cluster — not a hand-run `podman save | ctr import`
// spike. It is the R7/R10 witness for this diff.
//
// It SKIPS cleanly (never fakes) unless a kind cluster named $CHARLY_KIND_BOXLOAD_CLUSTER is
// actually running on the host engine — so `go test ./...` stays green on a machine with no
// cluster, and the PR body pastes the run where one exists. It asserts the node's containerd
// ends up holding the image (probed through the SAME venue seam the code uses).

// TestBoxLoadKindLive drives `charly box load --kind` end to end:
//
//	CHARLY_KIND_BOXLOAD_CLUSTER  the cluster name (its node is <name>-control-plane)
func TestBoxLoadKindLive(t *testing.T) {
	cluster := strings.TrimSpace(os.Getenv("CHARLY_KIND_BOXLOAD_CLUSTER"))
	if cluster == "" {
		t.Skip("CHARLY_KIND_BOXLOAD_CLUSTER unset — no live kind cluster; skipping the box load --kind gate")
	}
	node := kindControlPlaneNode(cluster)
	if out, err := exec.Command("podman", "inspect", "-f", "{{.State.Running}}", node).Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		t.Skipf("kind cluster %q has no running node container %q — skipping", cluster, node)
	}

	const ref = "localhost/charly-boxload-kind-live:latest"

	// Host-side image the node cannot have (unique content per run), as `charly box build` leaves it.
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/Containerfile", []byte("FROM docker.io/library/alpine:latest\nRUN echo charly-boxload-live-$$ > /probe.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("podman", "build", "-q", "-t", ref, dir).CombinedOutput(); err != nil {
		t.Fatalf("podman build: %v\n%s", err, out)
	}
	// The node must START without it, so the test cannot pass on a stale prior load.
	_ = exec.Command("podman", "exec", node, "ctr", "-n", "k8s.io", "images", "rm", ref).Run()
	if nodeHasImage(t, node, ref) {
		t.Fatalf("node %q still holds %s after rm — cannot prove the load", node, ref)
	}

	// THE CHANGED CODE PATH: the SAME dispatch the `charly box load` leaf runs.
	hc := &hostClient{ctx: context.Background()}
	args := []string{"myapp", ref, "--kind", cluster, "--as", "localhost/charly-boxload-kind-live:tagged"}
	if err := dispatchLoad(hc, args); err != nil {
		t.Fatalf("charly box load --kind %s %s: %v", cluster, ref, err)
	}

	if !nodeHasImage(t, node, ref) {
		t.Fatalf("after `box load --kind`, node %q containerd does not hold %s", node, ref)
	}
	t.Logf("PASS: `charly box load --kind %s %s` delivered %s into node %q containerd (probed via ctr)", cluster, ref, ref, node)
}

func nodeHasImage(t *testing.T, node, ref string) bool {
	t.Helper()
	out, err := exec.Command("podman", "exec", node, "ctr", "-n", "k8s.io", "images", "ls", "-q").Output()
	if err != nil {
		t.Fatalf("probe node containerd: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == ref {
			return true
		}
	}
	return false
}
