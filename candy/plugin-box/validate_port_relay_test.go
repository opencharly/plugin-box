package box

// validate_port_relay_test.go — the port_relay/socat soundness rule.
//
// The rule asks "is the candy that ships the relay wrapper composed into this box?" and
// answered it by comparing a resolved candy's name to the literal "socat". That is
// pre-cutover vocabulary: socat used to live inside charly as candy/socat, where it really
// was named "socat". Post-cutover it is the standalone opencharly/layer-socat, and a remote
// candy is named by its REPO — so nothing in a resolved set is ever named "socat" again and
// the rule fired on every box that composes the relay correctly.
//
// These are unit tests on the predicate rather than fixture builds, because reproducing the
// failure through `charly box validate` needs a REMOTE candy (the name only degrades on a
// remote scan), which a hermetic test cannot fetch.

import "testing"

func TestProvidesSocatRelay_AcceptsThePostCutoverRepoName(t *testing.T) {
	// The defect: the standalone repo resolves under its repo name, and the rule missed it.
	if !providesSocatRelay("layer-socat") {
		t.Error(`"layer-socat" must satisfy the port_relay rule: post-cutover a remote candy ` +
			`is named by its repo, so opencharly/layer-socat resolves as "layer-socat" and ` +
			`never as "socat"`)
	}
}

func TestProvidesSocatRelay_StillAcceptsALocalSocatCandy(t *testing.T) {
	// A project carrying its own socat candy declares it as `socat:` and keeps that name,
	// so the original spelling must keep working — this is an addition, not a replacement.
	if !providesSocatRelay("socat") {
		t.Error(`a local candy named "socat" must still satisfy the port_relay rule`)
	}
}

func TestProvidesSocatRelay_RejectsUnrelatedCandies(t *testing.T) {
	// The predicate must stay narrow: it is a soundness check, and matching anything that
	// merely CONTAINS "socat" would let a box ship a broken relay silently.
	for _, name := range []string{
		"pod-openclaw",     // the relay CONSUMER, not the provider
		"layer-nodejs",     // an unrelated sibling in the same resolved set
		"socat-skill",      // layer-socat's OTHER entity — a skill, ships no wrapper
		"layer-socat-fork", // a lookalike repo name
		"",
	} {
		if providesSocatRelay(name) {
			t.Errorf("%q must not satisfy the port_relay rule", name)
		}
	}
}
