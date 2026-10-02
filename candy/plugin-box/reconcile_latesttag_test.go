package box

import "testing"

// TestReconcileTargetVersion_RemoteUsesCachedClient — charly#736: `reconcile --remote` MUST
// resolve the newest tag through the ONE cached client (latestTagFn → gitClient().LatestTag),
// NOT the raw per-call refs.GitLatestTag. This test injects latestTagFn (the cached seam) and
// asserts reconcileTargetVersion routes through it; it FAILS if the raw path is used, because
// then the injected resolver would never be consulted.
func TestReconcileTargetVersion_RemoteUsesCachedClient(t *testing.T) {
	orig := latestTagFn
	defer func() { latestTagFn = orig }()

	called := ""
	latestTagFn = func(repo string) (string, error) {
		called = repo
		return "v2026.274.0000", nil
	}

	got, err := reconcileTargetVersion(true, "opencharly/example", map[string]bool{"v2026.1.0": true})
	if err != nil {
		t.Fatalf("reconcileTargetVersion(remote): %v", err)
	}
	if called != "opencharly/example" {
		t.Fatalf("remote reconcile did not route through the cached client (latestTagFn); called=%q", called)
	}
	if got != "v2026.274.0000" {
		t.Fatalf("remote reconcile = %q, want the cached resolver's answer", got)
	}

	// Offline mode must NOT call the remote resolver at all (uses newest-referenced).
	called = ""
	got, err = reconcileTargetVersion(false, "opencharly/example", map[string]bool{"v2026.1.0": true, "v2026.9.0": true})
	if err != nil {
		t.Fatalf("reconcileTargetVersion(offline): %v", err)
	}
	if called != "" {
		t.Fatalf("offline reconcile must not hit the remote resolver; called=%q", called)
	}
	if got != "v2026.9.0" {
		t.Fatalf("offline reconcile = %q, want the newest referenced v2026.9.0", got)
	}
}
