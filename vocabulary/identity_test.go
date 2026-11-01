package vocabulary

import "testing"

// TestCoreIdentityAliasIsDeclared proves the framework alias predicate reaches
// the alias index through discovery, as every other alias predicate does
// (ADR-114 §6). The expected spelling is written out here, not read from the
// constant, so a renamed constant fails the test.
func TestCoreIdentityAliasIsDeclared(t *testing.T) {
	const want = "core.identity.alias"

	if _, ok := DiscoverAliasPredicates()[want]; !ok {
		t.Fatalf("DiscoverAliasPredicates() does not contain %q", want)
	}
	meta := GetPredicateMetadata(want)
	if meta == nil {
		t.Fatalf("GetPredicateMetadata(%q) = nil", want)
	}
	if meta.AliasType != AliasTypeIdentity {
		t.Errorf("%q alias type = %q, want %q", want, meta.AliasType, AliasTypeIdentity)
	}
	if meta.DataType != DataTypeString {
		t.Errorf("%q data type = %q, want %q", want, meta.DataType, DataTypeString)
	}
}
