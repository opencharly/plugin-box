package box

import (
	"strings"
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

// TestValidateSingleTaskConfigContentRefs pins the config-content ${VAR} rule:
// content is substituted at generate time, so an unresolved reference must be
// a validate-time error naming the field (write: bodies stay verbatim — no such rule).
func TestValidateSingleTaskConfigContentRefs(t *testing.T) {
	known := map[string]bool{"CBX_CFG_PROVIDER": true}
	e := &vErr{}
	validateSingleTask("cbx", 0, "config",
		&spec.Op{Config: "/etc/crabbox.yaml", Content: "provider: ${CBX_CFG_PROVIDER}\nnoHostname: true\n", Mode: "0600"}, known, e)
	if len(e.msgs) != 0 {
		t.Fatalf("resolved config content must pass, got errors: %v", e.msgs)
	}
	e = &vErr{}
	validateSingleTask("cbx", 1, "config",
		&spec.Op{Config: "/etc/crabbox.yaml", Content: "provider: ${MISSING_VAR}\n"}, known, e)
	if len(e.msgs) != 1 {
		t.Fatalf("unresolved config content must be rejected, got %d errors: %v", len(e.msgs), e.msgs)
	}
	if !strings.Contains(e.msgs[0], "MISSING_VAR") || !strings.Contains(e.msgs[0], "config content") {
		t.Fatalf("error must name the field + the unresolved ref, got: %v", e.msgs)
	}
	// write: content stays verbatim — the same unresolved ref on a write step passes.
	e = &vErr{}
	validateSingleTask("cbx", 2, "write",
		&spec.Op{Write: "/etc/x", Content: "keep ${MISSING_VAR}\n"}, known, e)
	if len(e.msgs) != 0 {
		t.Fatalf("write content must stay verbatim (no ref rule), got errors: %v", e.msgs)
	}
}
