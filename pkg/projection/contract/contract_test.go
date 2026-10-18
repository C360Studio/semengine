package contract

import (
	"errors"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/vocabulary"
)

func TestContractValidateUsesVocabularyProfiles(t *testing.T) {
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register("test.value.name")
	base := Contract{
		Name: "test", EntityPattern: "*.*.test.system.widget.*",
		BirthPredicates: []string{"test.value.name"},
	}
	for _, profile := range []string{
		vocabulary.IndexingProfileContent, vocabulary.IndexingProfileControl,
		vocabulary.IndexingProfileSignal, vocabulary.IndexingProfileTrace,
	} {
		c := base
		c.IndexingProfile = profile
		if err := c.Validate(); err != nil {
			t.Errorf("Validate() with profile %q = %v, want nil", profile, err)
		}
		if err := c.ValidateShape(); err != nil {
			t.Errorf("ValidateShape() with profile %q = %v, want nil", profile, err)
		}
	}
	c := base
	c.IndexingProfile = "prose"
	if err := c.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Validate() with profile \"prose\" = %v, want invalid contract", err)
	}
	if err := c.ValidateShape(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("ValidateShape() with profile \"prose\" = %v, want invalid contract", err)
	}
}

// ValidateShape is everything Validate does except predicate declaration, so a
// registration can validate a contract before the vocabulary is populated.
func TestValidateShapeSkipsPredicateDeclaration(t *testing.T) {
	t.Cleanup(vocabulary.SnapshotRegistry())
	c := Contract{
		Name: "test", EntityPattern: "*.*.test.system.widget.*",
		BirthPredicates: []string{"test.undeclared.name"},
	}
	if err := c.ValidateShape(); err != nil {
		t.Fatalf("ValidateShape() = %v, want nil for an undeclared but well-formed predicate", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Validate() = %v, want invalid contract for an undeclared predicate", err)
	}
}

// A clone shares no slice with its original: changing any element of the clone's groups, a
// group's predicates or its birth predicates leaves the original as it was built.
func TestContractCloneIsIndependent(t *testing.T) {
	build := func() Contract {
		return Contract{
			Name: "widget", EntityPattern: "*.*.test.system.widget.*",
			Groups: []PredicateGroup{
				{Name: "status", Mode: ModeReconcile, Predicates: []string{"test.status.phase", "test.status.owner"}},
				{Name: "notes", Mode: ModeAppend, Predicates: []string{"test.note.text"}},
			},
			BirthPredicates: []string{"test.value.name"},
			IndexingProfile: vocabulary.IndexingProfileContent,
		}
	}
	original := build()
	clone := original.Clone()
	if !reflect.DeepEqual(clone, build()) {
		t.Fatalf("Clone() = %+v, want %+v", clone, build())
	}

	clone.Groups[0].Name = "changed"
	clone.Groups[0].Predicates[0] = "test.changed.phase"
	clone.Groups[1].Predicates[0] = "test.changed.text"
	clone.BirthPredicates[0] = "test.changed.name"
	if !reflect.DeepEqual(original, build()) {
		t.Fatalf("original after changing its clone = %+v, want %+v", original, build())
	}

	sparse := Contract{Name: "sparse", EntityPattern: "*.*.test.system.widget.*", BirthPredicates: []string{"test.value.name"}}
	if got := sparse.Clone(); !reflect.DeepEqual(got, sparse) {
		t.Fatalf("Clone() of a contract with no groups = %+v, want %+v", got, sparse)
	}
}
