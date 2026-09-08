package box

import (
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
	if class, _, ok := splitPluginCapability("kind:anything"); !ok || class != "kind" {
		t.Fatalf("splitPluginCapability rejects a class from spec.ProviderClasses: %q", "kind:anything")
	}
	if _, _, ok := splitPluginCapability("nope:anything"); ok {
		t.Fatal("splitPluginCapability accepts an unknown class — the closed set is leaking")
	}
}
