package box

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/spec/spec"
)

// validate_owned_content_test.go — the engine's dispatch to the content kinds (opencharly/sdk#598). The
// rules themselves are kind:skill's (opencharly/plugin-harness-kind, tested there); this pins that the
// engine asks every content kind in PROJECT scope, reports every diagnostic, and never reads "0
// errors" because a kind could not be asked.

func ownedContentFixture(t *testing.T) (*spec.Config, map[string]deploykit.CandyModel) {
	t.Helper()
	repo := t.TempDir()
	dk := map[string]deploykit.CandyModel{
		"carrier": deploykit.NewSpecCandyModel(spec.CandyModel{Name: "carrier", SourceDir: filepath.Join(repo, "candy", "carrier")}, spec.CandyView{Name: "carrier"}),
		"unused":  deploykit.NewSpecCandyModel(spec.CandyModel{Name: "unused", SourceDir: filepath.Join(repo, "candy", "unused")}, spec.CandyView{Name: "unused"}),
	}
	cfg := &spec.Config{
		Box: spec.BoxMap{"img": spec.EncodeBox(spec.BoxConfig{Candy: []string{"carrier"}})},
		PluginKinds: map[string]map[string]json.RawMessage{
			"skill": {"s": json.RawMessage(`{"owner":"carrier"}`)},
		},
	}
	return cfg, dk
}

// The kind is asked once, in project scope: no box, no image user, EVERY candy (composed or not) and
// every image — and each of its diagnostics becomes one finding naming the kind.
func TestValidateOwnedContent_AsksInProjectScopeAndReportsEveryDiagnostic(t *testing.T) {
	cfg, dk := ownedContentFixture(t)
	var got []spec.OwnedContentRequest
	ask := func(kind string, req spec.OwnedContentRequest) (*spec.OwnedContentReply, error) {
		got = append(got, req)
		return &spec.OwnedContentReply{Diagnostics: []string{`skill "a": first`, `skill "b": second`}}, nil
	}
	e := &vErr{}
	validateOwnedContent(cfg, dk, []string{"skill"}, ask, e)
	if len(got) != 1 {
		t.Fatalf("the kind was asked %d times, want once", len(got))
	}
	req := got[0]
	if req.Box != "" || req.Home != "" || len(req.Candies) != 2 || strings.Join(req.Images, ",") != "img" || len(req.Nodes) != 1 {
		t.Fatalf("request = %+v, want project scope: no box/user, every candy, every image, the kind's nodes", req)
	}
	if len(e.msgs) != 2 || e.msgs[0] != `kind "skill": skill "a": first` || e.msgs[1] != `kind "skill": skill "b": second` {
		t.Fatalf("findings = %q, want each diagnostic, prefixed with its kind", e.msgs)
	}
}

// A kind that cannot be asked is a finding, never a silent pass; a kind with no nodes is not asked; a
// project with no config checks nothing.
func TestValidateOwnedContent_AnUnaskableKindIsAFinding(t *testing.T) {
	cfg, dk := ownedContentFixture(t)
	e := &vErr{}
	validateOwnedContent(cfg, dk, []string{"skill"}, func(string, spec.OwnedContentRequest) (*spec.OwnedContentReply, error) {
		return nil, errors.New("provider not connected")
	}, e)
	if len(e.msgs) != 1 || !strings.Contains(e.msgs[0], "could not check its nodes: provider not connected") {
		t.Fatalf("findings = %q, want the failed ask reported", e.msgs)
	}

	asked := false
	none := &vErr{}
	validateOwnedContent(cfg, dk, []string{"hook"}, func(string, spec.OwnedContentRequest) (*spec.OwnedContentReply, error) {
		asked = true
		return nil, nil
	}, none)
	if asked || len(none.msgs) != 0 {
		t.Fatalf("a content kind with no nodes was asked (%v) or reported %q", asked, none.msgs)
	}
	validateOwnedContent(nil, dk, []string{"skill"}, nil, none)
}
