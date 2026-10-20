package box

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/spec/spec"
)

// validate_skills_test.go — both controls for validateSkillBodies. The tree is identical in each case;
// only the body's location differs. The pre-change code (which called only CollectSkills) reported
// nothing for either, because composition gates the read.

func skillBodiesCfg(t *testing.T, atRoot bool) (*spec.Config, map[string]deploykit.CandyModel) {
	t.Helper()
	dir := t.TempDir()
	tree := filepath.Join(dir, ".agents", "skills", "review")
	if !atRoot {
		tree = filepath.Join(dir, "candy", "plugin-review", ".agents", "skills", "review")
	}
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: review\ndescription: Review a PR\n---\n\nReviews things.\n"
	if err := os.WriteFile(filepath.Join(tree, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dk := map[string]deploykit.CandyModel{
		"plugin-review": deploykit.NewSpecCandyModel(
			spec.CandyModel{Name: "plugin-review", SourceDir: filepath.Join(dir, "candy", "plugin-review")},
			spec.CandyView{Name: "plugin-review", Description: "review"},
		),
	}
	cfg := &spec.Config{
		Skills: map[string]json.RawMessage{
			"review": json.RawMessage(`{"owner":"plugin-review","family":"charly-internals"}`),
		},
	}
	return cfg, dk
}

func TestValidateSkillBodies_BesideTheCandyIsClean(t *testing.T) {
	cfg, dk := skillBodiesCfg(t, false)
	e := &vErr{}
	validateSkillBodies(cfg, dk, e)
	if len(e.msgs) != 0 {
		t.Fatalf("body beside the candy must be clean, got %v", e.msgs)
	}
}

func TestValidateSkillBodies_AtTheRepoRootIsReported(t *testing.T) {
	cfg, dk := skillBodiesCfg(t, true)
	e := &vErr{}
	validateSkillBodies(cfg, dk, e)
	if len(e.msgs) == 0 {
		t.Fatal("body at the repo root must be reported — the loader reads beside the candy, not at the root")
	}
	t.Logf("reported: %v", e.msgs)
}

func TestValidateSkillBodies_NilConfigIsSilent(t *testing.T) {
	e := &vErr{}
	validateSkillBodies(nil, nil, e) // project-less dir: no panic, no finding
	if len(e.msgs) != 0 {
		t.Fatalf("nil config must be silent, got %v", e.msgs)
	}
}
