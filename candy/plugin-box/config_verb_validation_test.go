package box

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestValidateConfigTask pins the config: per-verb validation rules: the
// destination must be absolute (or ~//${HOME}-prefixed) and the content body
// (the rendered file) must be non-empty.
func TestValidateConfigTask(t *testing.T) {
	// Relative destination must be rejected.
	e := &vErr{}
	validateConfigTask("cbx", 0, &spec.Op{Config: "etc/x.yaml", Content: "provider: x\n"}, e)
	if len(e.msgs) != 1 {
		t.Fatalf("relative config destination must be rejected, got %d errors: %v", len(e.msgs), e.msgs)
	}
	// Absolute destination with EMPTY content must be rejected.
	e = &vErr{}
	validateConfigTask("cbx", 1, &spec.Op{Config: "/etc/x.yaml"}, e)
	if len(e.msgs) != 1 {
		t.Fatalf("empty config content must be rejected, got %d errors: %v", len(e.msgs), e.msgs)
	}
	// Valid absolute destination + content passes.
	e = &vErr{}
	validateConfigTask("cbx", 2, &spec.Op{Config: "/etc/crabbox.yaml", Content: "provider: local-container\n"}, e)
	if len(e.msgs) != 0 {
		t.Fatalf("valid config task must pass, got errors: %v", e.msgs)
	}
}
