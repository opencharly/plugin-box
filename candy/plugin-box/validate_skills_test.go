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

// ---------------------------------------------------------------------------
// The ownership shape (opencharly/sdk#434): an image candy may not own a skill.
// ---------------------------------------------------------------------------

// imageOwnedSkillCfg is the ruled-out shape: `pimage` is an IMAGE (`base:`), it composes the package
// candy `pcandy`, and the skill `psk1` names the IMAGE as its owner. inlineContent selects the arm
// that validated CLEAN before this rule (measured rc=0 on the CLI) — the shape is what is forbidden,
// not the missing body.
func imageOwnedSkillCfg(t *testing.T, inlineContent bool) (*spec.Config, map[string]deploykit.CandyModel) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "candy", "pcandy"), 0o755); err != nil {
		t.Fatal(err)
	}
	dk := map[string]deploykit.CandyModel{
		"pcandy": deploykit.NewSpecCandyModel(
			spec.CandyModel{Name: "pcandy", SourceDir: filepath.Join(dir, "candy", "pcandy")},
			spec.CandyView{Name: "pcandy", Description: "the package candy the image composes"},
		),
	}
	skill := spec.Skill{Family: "test", Owner: "pimage"}
	if inlineContent {
		skill.Content = "Body prose owned by the image.\n"
	}
	raw, err := json.Marshal(skill)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &spec.Config{
		Box: spec.BoxMap{
			"pimage": json.RawMessage(`{"base":"docker.io/library/alpine:3.20","candy":["pcandy"]}`),
		},
		Skills: map[string]json.RawMessage{"psk1": raw},
	}
	return cfg, dk
}

// TestValidateSkills_ImageOwnedSkillIsRejected is the change's own control: it FAILS on the pre-change
// source (which accepted the inline-content arm and reported the other under a message describing a
// file search that never ran), and it pins the exact wording so the fix clause and the issue reference
// cannot be silently dropped.
func TestValidateSkills_ImageOwnedSkillIsRejected(t *testing.T) {
	const want = "skills: skill \"psk1\": owner \"pimage\" is an image, not a candy — a `candy:` node carrying `base:` or `from:` folds into the image set, so it has no source directory and this skill's prose can never be read (it is also silently absent from the ai.opencharly.skill label): move this skill to a package candy that the image composes, and set `owner:` to that candy (opencharly/sdk#434)"

	for _, arm := range []struct {
		name          string
		inlineContent bool
	}{
		{"in-line content", true},
		{"no body", false},
	} {
		t.Run(arm.name, func(t *testing.T) {
			cfg, dk := imageOwnedSkillCfg(t, arm.inlineContent)
			e := &vErr{}
			validateSkills(cfg, dk, e)
			if len(e.msgs) != 1 {
				t.Fatalf("an image-owned skill must produce exactly ONE finding (the shape, never also the body walk's message), got %d: %v", len(e.msgs), e.msgs)
			}
			if e.msgs[0] != want {
				t.Fatalf("message mismatch\n got: %s\nwant: %s", e.msgs[0], want)
			}
		})
	}
}

// TestValidateSkills_ImageAndCandySameNameKeepsTheCandy is the anti-over-rejection control: four repos
// name one entity as BOTH an image and a layer, and there the layer resolves the skill, so the shape is
// legal. The rejected arm must depend on the owner being an image AND NOT a candy.
func TestValidateSkills_ImageAndCandySameNameKeepsTheCandy(t *testing.T) {
	cfg, dk := imageOwnedSkillCfg(t, false)
	// Same name in the candy set: the layer wins, and a body resolves beside it.
	dir := t.TempDir()
	tree := filepath.Join(dir, ".agents", "skills", "psk1")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "SKILL.md"), []byte("---\nname: psk1\ndescription: twin\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dk["pimage"] = deploykit.NewSpecCandyModel(
		spec.CandyModel{Name: "pimage", SourceDir: dir},
		spec.CandyView{Name: "pimage", Description: "the layer twin"},
	)
	e := &vErr{}
	validateSkills(cfg, dk, e)
	if len(e.msgs) != 0 {
		t.Fatalf("a name that is BOTH an image and a candy resolves to the candy and must not be rejected, got %v", e.msgs)
	}
}

// TestValidateSkills_PackageCandyOwnerStaysClean is the second anti-over-rejection control: the shape
// this rule DEMANDS (a package candy the image composes) must validate clean.
func TestValidateSkills_PackageCandyOwnerStaysClean(t *testing.T) {
	cfg, dk := skillBodiesCfg(t, false) // owner `plugin-review` is a candy with its body beside it
	e := &vErr{}
	validateSkills(cfg, dk, e)
	if len(e.msgs) != 0 {
		t.Fatalf("a package-candy owner with a resolvable body must be clean, got %v", e.msgs)
	}
}
