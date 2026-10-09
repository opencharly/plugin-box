package box

import (
	"slices"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestValidPluginClassesParity (F4.2) proves the plugin-box closed provider-class set is
// DERIVED from spec.ProviderClasses (the ONE CUE-owned vocabulary) — every CUE class is
// accepted, and no extra class leaks in.
func TestValidPluginClassesParity(t *testing.T) {
	if len(validPluginClasses) != len(spec.ProviderClasses) {
		t.Fatalf("validPluginClasses has %d entries, spec.ProviderClasses has %d — they must match (one CUE source, F4.2)",
			len(validPluginClasses), len(spec.ProviderClasses))
	}
	for _, c := range spec.ProviderClasses {
		if !validPluginClasses[c] {
			t.Errorf("spec.ProviderClasses contains %q but the plugin closed set does not", c)
		}
	}
	if class, _, fault := splitPluginCapability("kind:anything"); fault != capFaultNone || class != "kind" {
		t.Fatalf("splitPluginCapability rejects a class from spec.ProviderClasses: %q", "kind:anything")
	}
	if class, _, fault := splitPluginCapability("nope:anything"); fault != capFaultUnknownClass || class != "nope" {
		t.Fatalf("splitPluginCapability must report an unknown CLASS (not a malformed pair) for %q; got fault=%v class=%q",
			"nope:anything", fault, class)
	}
}

// TestPluginCapabilityFaultsAreDistinct (F4.2 + the pb#24 diagnostic fix) pins that the two
// rejection reasons stay distinguishable — a well-formed pair whose class is unknown is a
// CONTRACT mismatch, not a grammar error, and the caller must be able to word them differently.
// Collapsing them back into one `ok bool` would re-introduce the false "is malformed" message.
func TestPluginCapabilityFaultsAreDistinct(t *testing.T) {
	malformed := []string{"colonless", ":word", "kind:", ""}
	for _, s := range malformed {
		if _, _, fault := splitPluginCapability(s); fault != capFaultMalformed {
			t.Errorf("splitPluginCapability(%q) = fault %v, want capFaultMalformed", s, fault)
		}
	}
	// The whole point: an unknown class with a perfectly good word is NOT malformed.
	if _, word, fault := splitPluginCapability("workflow:lobster"); fault != capFaultUnknownClass || word != "lobster" {
		t.Errorf("splitPluginCapability(%q) = fault %v word %q, want capFaultUnknownClass / \"lobster\"",
			"workflow:lobster", fault, word)
	}
}

// TestKnownProviderClassesMessage pins the enumerated class list the unknown-class diagnostic
// prints: it must list EVERY spec.ProviderClasses entry, add none, and be sorted (so the message
// is stable rather than reordering with the CUE declaration order).
func TestKnownProviderClassesMessage(t *testing.T) {
	got := strings.Split(knownProviderClasses, ", ")
	if len(got) != len(spec.ProviderClasses) {
		t.Fatalf("knownProviderClasses lists %d classes, spec.ProviderClasses has %d",
			len(got), len(spec.ProviderClasses))
	}
	if !slices.IsSorted(got) {
		t.Errorf("knownProviderClasses is not sorted: %q", knownProviderClasses)
	}
	for _, c := range spec.ProviderClasses {
		if !slices.Contains(got, c) {
			t.Errorf("knownProviderClasses omits spec.ProviderClasses entry %q: %q", c, knownProviderClasses)
		}
	}
}

// pluginCandyFaults runs the REAL engine entry (validateCandyContents — the rule that owns the
// plugin capability check) over a one-candy envelope declaring the given providers with an
// EXTERNAL source (so the builtin-compiled-in arm is skipped and only the capability fault is
// under test) and returns the joined verdict.
func pluginCandyFaults(providers ...string) string {
	rp := &spec.ResolvedProject{
		CandyModels: map[string]spec.CandyModel{"myplugin": {Name: "myplugin"}},
		Candies: map[string]spec.CandyView{
			"myplugin": {
				IsPlugin:        true,
				PluginSource:    "github.com/opencharly/plugin-x/candy/plugin-x",
				PluginProviders: providers,
			},
		},
	}
	e := &vErr{}
	validateCandyContents(newVctx(rp), e)
	return strings.Join(e.msgs, "\n")
}

// TestPluginCapabilityDiagnosticWording (pb#24) drives the rule end-to-end: an unknown-class
// capability must produce the ACCURATE contract-mismatch message naming the class and the known
// set, and must NOT say "malformed"; a genuinely malformed pair must still say "malformed" and
// must NOT claim an unknown class. Without the fix both faults collapse into one false
// "is malformed" line, which is exactly what this test fails on.
func TestPluginCapabilityDiagnosticWording(t *testing.T) {
	got := pluginCandyFaults("workflow:lobster")
	if !strings.Contains(got, `unknown provider class "workflow"`) {
		t.Errorf("an unknown class must be reported as such; got: %s", got)
	}
	if !strings.Contains(got, "known: ") || !strings.Contains(got, "agent-runtime") {
		t.Errorf("the unknown-class message must enumerate the known classes; got: %s", got)
	}
	if strings.Contains(got, "is malformed") {
		t.Errorf("a well-formed pair with an unknown class must NOT be called malformed; got: %s", got)
	}

	gotMalformed := pluginCandyFaults("colonless")
	if !strings.Contains(gotMalformed, "is malformed (want <class>:<word>)") {
		t.Errorf("a truly malformed pair must keep the grammar message; got: %s", gotMalformed)
	}
	if strings.Contains(gotMalformed, "unknown provider class") {
		t.Errorf("a malformed pair must NOT claim an unknown class; got: %s", gotMalformed)
	}
}

// pluginRequiresFaults mirrors pluginCandyFaults, putting the capability in `requires:` instead of
// `providers:` so the two paths can be asserted independently.
func pluginRequiresFaults(reqs ...spec.PluginRequirement) string {
	rp := &spec.ResolvedProject{
		CandyModels: map[string]spec.CandyModel{"myplugin": {Name: "myplugin"}},
		Candies: map[string]spec.CandyView{
			"myplugin": {
				IsPlugin:        true,
				PluginSource:    "github.com/opencharly/plugin-x/candy/plugin-x",
				PluginProviders: []string{"verb:x"},
				PluginRequires:  reqs,
			},
		},
	}
	e := &vErr{}
	validateCandyContents(newVctx(rp), e)
	return strings.Join(e.msgs, "\n")
}

// charly#853 / plugin-box#29: the class vocabulary used to be checked by the CUE pattern for BOTH
// paths, because `providers:` and `requires:` both name a #PluginCapability. The pattern's class
// segment is now structural so this rule can NAME an undeclared class, which means the vocabulary
// check for `requires:` has to live here - otherwise widening the CUE would leave that path with no
// check at all. The spec tests for it moved here (plugin_requires_test.go, plugin_test.go).
func TestPluginRequiresClassVocabulary(t *testing.T) {
	// (1) an UNDECLARED class in requires: must be reported, naming the class and the set.
	got := pluginRequiresFaults(spec.PluginRequirement{Capability: "zzznotaclass:lobster"})
	if !strings.Contains(got, "plugin_requires") || !strings.Contains(got, `unknown provider class "zzznotaclass"`) {
		t.Errorf("an undeclared class in requires: must be named; got: %s", got)
	}
	if !strings.Contains(got, "known: ") {
		t.Errorf("the requires: message must enumerate the known classes; got: %s", got)
	}
	if strings.Contains(got, "is malformed") {
		t.Errorf("a well-formed requires: pair with an unknown class must NOT be called malformed; got: %s", got)
	}
	// (2) a DECLARED class must pass untouched.
	ok := pluginRequiresFaults(spec.PluginRequirement{Capability: "verb:enc"})
	if strings.Contains(ok, "unknown provider class") || strings.Contains(ok, "plugin_requires") {
		t.Errorf("a declared class in requires: must be accepted; got: %s", ok)
	}
	// (3) a truly MALFORMED pair keeps the grammar message and does not claim an unknown class.
	bad := pluginRequiresFaults(spec.PluginRequirement{Capability: "colonless"})
	if !strings.Contains(bad, "is malformed (want <class>:<word>)") || strings.Contains(bad, "unknown provider class") {
		t.Errorf("a malformed requires: capability must keep the grammar message; got: %s", bad)
	}
}
