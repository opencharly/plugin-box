package box

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/spec/spec"
)

// validateBuilders asks THE question the build asks (deploykit.NeedsBuilder, opencharly/sdk#608):
// a candy that declares aur packages under `arch` needs an aur route on an arch box, and on a
// fedora box the declaration does not apply, so no route is demanded. Before, validate kept its own
// trigger with a `build:` format gate, so validate and build could disagree about a builder.
func TestValidateBuildersUsesTheBuildTrigger(t *testing.T) {
	rp := func(distro string, route map[string]string) *spec.ResolvedProject {
		return &spec.ResolvedProject{
			CandyModels: map[string]spec.CandyModel{"shell": {Name: "shell", TagSections: map[string]spec.TagPkgConfig{
				"arch": {Managers: map[string]spec.PackageSection{"aur": {FormatName: "aur", Packages: []string{"kdotool"}}}},
			}}},
			Candies: map[string]spec.CandyView{"shell": {Name: "shell"}},
			Boxes:   map[string]spec.ResolvedBoxView{"b": {Name: "b", Distro: []string{distro}, Candy: []string{"shell"}, Builder: route}},
			Builder: map[string]*spec.Builder{"aur": {}},
		}
	}
	run := func(p *spec.ResolvedProject) string {
		e := &vErr{}
		validateBuilders(newVctx(p), e)
		return strings.Join(e.msgs, "\n")
	}
	if got := run(rp("arch", nil)); !strings.Contains(got, `candy "shell" needs builder "aur"`) {
		t.Errorf("an arch box without an aur route was not refused: %q", got)
	}
	if got := run(rp("arch", map[string]string{"aur": "arch-builder"})); got != "" {
		t.Errorf("an arch box with an aur route was refused: %q", got)
	}
	if got := run(rp("fedora", nil)); got != "" {
		t.Errorf("a fedora box was asked for an aur route an arch-tagged declaration does not reach: %q", got)
	}
}

// A candy whose only content is a builder manifest the VOCABULARY detects (mise.toml) has install
// files: the check reads the builders' detect_file data, not a hand-kept list of three manifests
// that left mise out (opencharly/sdk#608). Without a builder that detects it, it is still empty.
func TestBuilderManifestIsInstallContentFromTheVocabulary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mise.toml"), []byte("[tools]\njq = \"1.7.1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dk := deploykit.NewSpecCandyModel(spec.CandyModel{Name: "tools", SourceDir: dir}, spec.CandyView{Name: "tools"})
	mise := map[string]*spec.Builder{"mise": {DetectFiles: []string{"mise.toml", ".tool-versions"}}}
	if !candyHasInstallFiles(spec.CandyModel{}, dk, mise) {
		t.Error("a mise.toml candy is not install content although the vocabulary's mise builder detects it")
	}
	if candyHasInstallFiles(spec.CandyModel{}, dk, map[string]*spec.Builder{"npm": {DetectFiles: []string{"package.json"}}}) {
		t.Error("a mise.toml candy counts as install content with no builder that detects it")
	}
}

// The install-content RULE (not only its helper) reads the project's builder vocabulary: a candy
// whose only content is a builder manifest passes `charly box validate` when the project's
// vocabulary has a builder that detects it, and is still refused when none does. The helper test
// above cannot see the rule's wiring, so a rule that stopped passing the vocabulary would slip by
// it and reject every mise-only candy.
func TestInstallContentRuleReadsTheProjectBuilderVocabulary(t *testing.T) {
	findings := func(vocab map[string]*spec.Builder) []string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "mise.toml"), []byte("[tools]\njq = \"1.7.1\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		rp := &spec.ResolvedProject{
			CandyModels: map[string]spec.CandyModel{"tools": {
				Name:      "tools",
				SourceDir: dir,
				Plan:      []spec.Step{{Check: "jq runs", Op: spec.Op{Command: "jq --version"}}},
			}},
			Candies: map[string]spec.CandyView{"tools": {Name: "tools", Description: "ships jq through mise"}},
			Builder: vocab,
		}
		var e vErr
		validateCandyContents(newVctx(rp), &e)
		var hits []string
		for _, m := range e.msgs {
			if strings.Contains(m, installContentFinding) {
				hits = append(hits, m)
			}
		}
		return hits
	}
	if hits := findings(map[string]*spec.Builder{"mise": {DetectFiles: []string{"mise.toml"}}}); len(hits) != 0 {
		t.Errorf("a mise.toml candy is refused although the project's mise builder detects it: %q", hits)
	}
	if hits := findings(map[string]*spec.Builder{"npm": {DetectFiles: []string{"package.json"}}}); len(hits) != 1 {
		t.Errorf("a mise.toml candy with no builder that detects it: %d install-content findings, want 1", len(hits))
	}
}
