package box

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// validate_bake_plugin_test.go — the bake_plugin dedup in validateRemoteCandies.
// A candy's bake_plugin: ref pulls an out-of-tree plugin candy into its composing
// images, making the plugin appear under the BAKING repo's path while the standalone
// repo provides it directly. The conflict check must NOT flag the same underlying
// provider twice (issue: the plugin-mcp conflict blocking the Phase-4 distro sweep).

type fakeReader struct {
	spec.CandyReader
	name     string
	repoPath string
	remote   bool
	bake     []string
}

func (f *fakeReader) GetName() string     { return f.name }
func (f *fakeReader) GetRepoPath() string { return f.repoPath }
func (f *fakeReader) GetRemote() bool     { return f.remote }
func (f *fakeReader) GetBakePlugin() []spec.CandyRefEntry {
	var out []spec.CandyRefEntry
	for _, r := range f.bake {
		out = append(out, spec.CandyRefEntry{Raw: r})
	}
	return out
}

func TestBakePluginSibling(t *testing.T) {
	candies := map[string]spec.CandyReader{
		"charly/charly-mcp": &fakeReader{name: "charly-mcp", repoPath: "github.com/opencharly/charly", remote: true,
			bake: []string{"@github.com/opencharly/plugin-mcp/candy/plugin-mcp:v1"}},
		"charly/plugin-mcp": &fakeReader{name: "plugin-mcp", repoPath: "github.com/opencharly/charly", remote: true},
	}
	baked := &fakeReader{name: "plugin-mcp", repoPath: "github.com/opencharly/charly", remote: true}
	other := &fakeReader{name: "plugin-mcp", repoPath: "github.com/opencharly/plugin-mcp", remote: true}
	if !bakePluginSibling(candies, baked, other.GetRepoPath()) {
		t.Fatal("bakePluginSibling should be true: charly-mcp bakes plugin-mcp from the standalone repo")
	}

	// A genuine clash (no bake_plugin relationship) must NOT dedupe.
	unrelated := map[string]spec.CandyReader{
		"a/x": &fakeReader{name: "x", repoPath: "github.com/opencharly/a", remote: true},
	}
	xA := &fakeReader{name: "x", repoPath: "github.com/opencharly/a", remote: true}
	xB := &fakeReader{name: "x", repoPath: "github.com/opencharly/b", remote: true}
	if bakePluginSibling(unrelated, xA, xB.GetRepoPath()) {
		t.Fatal("bakePluginSibling should be false for unrelated repos")
	}
}

func TestBakePluginSibling_ScopedToName(t *testing.T) {
	// A sibling bakes a DIFFERENT plugin from the same other repo — must NOT dedupe.
	candies := map[string]spec.CandyReader{
		"charly/charly-mcp": &fakeReader{name: "charly-mcp", repoPath: "github.com/opencharly/charly", remote: true,
			bake: []string{"@github.com/opencharly/plugin-mcp/candy/plugin-mcp:v1"}},
	}
	conflict := &fakeReader{name: "plugin-record", repoPath: "github.com/opencharly/charly", remote: true}
	other := &fakeReader{name: "plugin-record", repoPath: "github.com/opencharly/plugin-record", remote: true}
	if bakePluginSibling(candies, conflict, other.GetRepoPath()) {
		t.Fatal("bakePluginSibling must be FALSE: the sibling bakes plugin-mcp, not plugin-record")
	}
}

// TestBakedPluginConflictIsSuppressedInBothDirections drives the CONFLICT LOOP, not the
// helper — which is where the defect lived and why the helper's own test could not see it.
//
// validateRemoteCandies compares every remote candy against every other, so it visits a
// clashing pair TWICE: (candy=A, other=B) and (candy=B, other=A). The bake relationship is
// NOT symmetric — only the BAKING repo holds the sibling carrying the `bake_plugin:` ref.
// Testing one direction therefore suppressed the visit where the baker happened to be
// `candy` and let the mirrored visit through, so every baked plugin reported the identical
// conflict twice in opposite orders:
//
//	"plugin-mcp" provided by both …/charly     and …/plugin-mcp
//	"plugin-mcp" provided by both …/plugin-mcp and …/charly
//
// Reverting the fix makes this test fail with exactly one message — the mirrored one.
func TestBakedPluginConflictIsSuppressedInBothDirections(t *testing.T) {
	// charly bakes the standalone plugin-mcp; both therefore provide a candy named
	// "plugin-mcp". That is one provider seen twice, not a genuine clash.
	candies := map[string]spec.CandyReader{
		"charly/charly-mcp": &fakeReader{
			name: "charly-mcp", repoPath: "github.com/opencharly/charly", remote: true,
			bake: []string{"@github.com/opencharly/plugin-mcp/candy/plugin-mcp:v1"},
		},
		"charly/plugin-mcp":     &fakeReader{name: "plugin-mcp", repoPath: "github.com/opencharly/charly", remote: true},
		"standalone/plugin-mcp": &fakeReader{name: "plugin-mcp", repoPath: "github.com/opencharly/plugin-mcp", remote: true},
	}
	var e vErr
	reportRemoteNameConflicts(candies, &e)
	for _, m := range e.msgs {
		if strings.Contains(m, "remote candy name conflict") {
			t.Errorf("baked plugin reported as a conflict: %s", m)
		}
	}
}

// A genuine clash — two unrelated repos providing the same candy name with NO bake_plugin
// ref between them — must still be reported. Without this the fix above could "pass" by
// suppressing every conflict, which is the way a dedupe goes wrong.
func TestGenuineNameConflictIsStillReported(t *testing.T) {
	candies := map[string]spec.CandyReader{
		"a/widget": &fakeReader{name: "widget", repoPath: "github.com/opencharly/repo-a", remote: true},
		"b/widget": &fakeReader{name: "widget", repoPath: "github.com/opencharly/repo-b", remote: true},
	}
	var e vErr
	reportRemoteNameConflicts(candies, &e)
	n := 0
	for _, m := range e.msgs {
		if strings.Contains(m, "remote candy name conflict") {
			n++
		}
	}
	if n == 0 {
		t.Error("a genuine two-repo name clash was suppressed — the dedupe is too broad")
	}
}
