package box

// validate_candy_manifest_source_test.go — the per-candy manifest verdict candyManifestVerdict
// applies inside validateCandyCUESchemas. opencharly/plugin-box#95: `charly box validate`
// answered "0 errors" for a project whose IMPORTED namespace carried a document the same binary
// refuses, because a candy whose manifest could not be read was skipped silently. The remote arms
// below are the ones that go red on that code: an imported candy whose manifest the rule cannot
// read must be an ERROR naming the candy and the path, never a pass.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// manifestCandy is a spec.CandyReader carrying only what candyManifestVerdict reads.
type manifestCandy struct {
	spec.CandyReader
	name      string
	sourceDir string
	remote    bool
}

func (c *manifestCandy) GetName() string      { return c.name }
func (c *manifestCandy) GetSourceDir() string { return c.sourceDir }
func (c *manifestCandy) GetRemote() bool      { return c.remote }

func TestCandyManifestVerdict(t *testing.T) {
	// The threaded snapshot carries the registry-derived kind set; "candy" must be recognized for
	// the node to be an entity at all.
	threaded := spec.Threaded{Kinds: map[string]bool{"candy": true}, Primaries: map[string]string{"command": "command"}}

	// A valid candy manifest: the positive control, so the error arms cannot pass by always failing.
	validDir := t.TempDir()
	writeManifest(t, validDir, `probe:
    candy:
        description: a valid layer candy
        plan:
            - check: loadable
              id: probe-ok
              command: "true"
`)
	if err := candyManifestVerdict(&manifestCandy{name: "probe", sourceDir: validDir, remote: true}, "probe", threaded); err != nil {
		t.Fatalf("a readable, schema-valid remote manifest must pass: %v", err)
	}

	// A candy node whose body the schema refuses (a missing description) — the imported-namespace
	// case the filed finding measured: the verdict must name the file.
	badDir := t.TempDir()
	writeManifest(t, badDir, `bad:
    candy:
        plan:
            - check: loadable
              id: bad-ok
              command: "true"
`)
	err := candyManifestVerdict(&manifestCandy{name: "bad", sourceDir: badDir, remote: true}, "bad", threaded)
	if err == nil {
		t.Fatal("a remote candy whose manifest the schema refuses must be reported, not skipped")
	}
	if !strings.Contains(err.Error(), filepath.Join(badDir, spec.UnifiedFileName)) {
		t.Errorf("the verdict must name the manifest file it read, got: %v", err)
	}

	// A REMOTE candy whose namespace never materialized (no source dir): the silent skip that made
	// `box validate` report OK for an unread manifest (opencharly/plugin-box#95).
	err = candyManifestVerdict(&manifestCandy{name: "ghost", remote: true}, "ghost", threaded)
	if err == nil || !strings.Contains(err.Error(), "no materialized source directory") {
		t.Fatalf("a remote candy with no source directory must be an error naming it, got: %v", err)
	}

	// A REMOTE candy whose source dir exists but carries no manifest: same obligation.
	err = candyManifestVerdict(&manifestCandy{name: "moved", sourceDir: t.TempDir(), remote: true}, "moved", threaded)
	if err == nil || !strings.Contains(err.Error(), "manifest unreadable") {
		t.Fatalf("a remote candy with no readable manifest must be an error naming the path, got: %v", err)
	}

	// An INLINE candy synthesized by the engine has no manifest of its own; its body is gated at
	// load, so the verdict must stay nil (this arm keeps the fix from over-correcting).
	if err := candyManifestVerdict(&manifestCandy{name: "inline"}, "inline", threaded); err != nil {
		t.Fatalf("an inline candy without a manifest must stay skipped, got: %v", err)
	}
	if err := candyManifestVerdict(&manifestCandy{name: "inline-moved", sourceDir: t.TempDir()}, "inline-moved", threaded); err != nil {
		t.Fatalf("an inline candy declared in another document must stay skipped, got: %v", err)
	}
}

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, spec.UnifiedFileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
