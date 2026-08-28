package box

import (
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
