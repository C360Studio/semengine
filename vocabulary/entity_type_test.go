package vocabulary

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEntityTypeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		key        string
		wantDomain string
		wantType   string
		wantErr    bool
	}{
		{
			name:       "canonical simple",
			key:        "uxs.uas",
			wantDomain: "uxs",
			wantType:   "uas",
		},
		{
			name:       "canonical kebab",
			key:        "air-defense.radar-track",
			wantDomain: "air-defense",
			wantType:   "radar-track",
		},
		{name: "empty", key: "", wantErr: true},
		{name: "one segment", key: "onlyone", wantErr: true},
		{name: "three segments", key: "three.part.key", wantErr: true},
		{name: "uppercase", key: "Upper.Case", wantErr: true},
		{name: "empty segment", key: "bad..key", wantErr: true},
		{name: "leading dot", key: ".leading", wantErr: true},
		{name: "trailing dot", key: "trailing.", wantErr: true},
		{name: "underscore", key: "under_score.type", wantErr: true},
		{name: "leading hyphen", key: "bad.-hyphen", wantErr: true},
		{name: "trailing hyphen", key: "bad.hyphen-", wantErr: true},
		{name: "double hyphen", key: "bad.double--hyphen", wantErr: true},
		{name: "leading digit", key: "bad.1type", wantErr: true},
		{
			name:    "segment too long",
			key:     "uxs." + strings.Repeat("a", MaxPredicateSegmentBytes+1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			domain, entityType, err := ParseEntityTypeKey(tt.key)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDomain, domain)
			assert.Equal(t, tt.wantType, entityType)
		})
	}
}

func TestRegisterEntityType(t *testing.T) {
	defer SnapshotRegistry()()
	ClearRegistry()

	assert.Nil(t, GetEntityTypeMetadata("uxs.uas"))
	assert.Empty(t, ListRegisteredEntityTypes())

	RegisterEntityType("uxs.uas",
		WithEntityTypeDescription("Unmanned aerial system"),
		WithClassIRI("https://confirm.mil/diekm/UnmannedAerialSystem"),
		WithSuperclass("http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft"),
		WithSuperclass("http://purl.obolibrary.org/obo/BFO_0000030"))

	meta := GetEntityTypeMetadata("uxs.uas")
	require.NotNil(t, meta)
	assert.Equal(t, "uxs.uas", meta.DomainType)
	assert.Equal(t, "uxs", meta.Domain)
	assert.Equal(t, "uas", meta.Type)
	assert.Equal(t, "Unmanned aerial system", meta.Description)
	assert.Equal(t, "https://confirm.mil/diekm/UnmannedAerialSystem", meta.ClassIRI)
	assert.Equal(t, []string{
		"http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft",
		"http://purl.obolibrary.org/obo/BFO_0000030",
	}, meta.Superclasses)

	// Defensive copy check on GetEntityTypeMetadata.
	meta.Superclasses[0] = "http://example.org/mutated"
	fresh := GetEntityTypeMetadata("uxs.uas")
	require.NotNil(t, fresh)
	assert.Equal(t, "http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft", fresh.Superclasses[0])

	// Amend preserves omitted fields and deduplicates Superclasses in registration order.
	RegisterEntityType("uxs.uas",
		WithEntityTypeDescription("Amended UAS description"),
		WithSuperclass("http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft"),
		WithSuperclass("https://confirm.mil/diekm/Platform"))

	amended := GetEntityTypeMetadata("uxs.uas")
	require.NotNil(t, amended)
	assert.Equal(t, "Amended UAS description", amended.Description)
	assert.Equal(t, "https://confirm.mil/diekm/UnmannedAerialSystem", amended.ClassIRI)
	assert.Equal(t, []string{
		"http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft",
		"http://purl.obolibrary.org/obo/BFO_0000030",
		"https://confirm.mil/diekm/Platform",
	}, amended.Superclasses)

	// Invalid registrations panic without mutating the registry.
	assert.Panics(t, func() {
		RegisterEntityType("Invalid.Key", WithClassIRI("http://example.org/Class"))
	})
	assert.Panics(t, func() {
		RegisterEntityType("uxs.ugv", WithClassIRI(""))
	})
	assert.Panics(t, func() {
		RegisterEntityType("uxs.ugv", WithClassIRI("http://example.org/has space"))
	})
	assert.Panics(t, func() {
		RegisterEntityType("uxs.ugv", WithSuperclass("   "))
	})
	assert.Nil(t, GetEntityTypeMetadata("uxs.ugv"))
}

func TestLookupEntityTypesByClassIRI(t *testing.T) {
	defer SnapshotRegistry()()
	ClearRegistry()

	const (
		diekmUAS       = "https://confirm.mil/diekm/UnmannedAerialSystem"
		diekmFixedWing = "https://confirm.mil/diekm/FixedWingUAS"
		ccoAircraft    = "http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft"
	)

	RegisterEntityType("uxs.uas",
		WithClassIRI(diekmUAS),
		WithSuperclass(ccoAircraft))
	RegisterEntityType("air.fixed-wing",
		WithClassIRI(diekmFixedWing),
		WithSuperclass(diekmUAS),
		WithSuperclass(ccoAircraft))

	assert.Equal(t, []string{"air.fixed-wing", "uxs.uas"}, ListRegisteredEntityTypes())
	assert.Equal(t, []string{"air.fixed-wing"}, LookupEntityTypesByClassIRI(diekmFixedWing))
	assert.Equal(t, []string{"air.fixed-wing", "uxs.uas"}, LookupEntityTypesByClassIRI(diekmUAS))
	assert.Equal(t, []string{"air.fixed-wing", "uxs.uas"}, LookupEntityTypesByClassIRI(ccoAircraft))
	assert.Nil(t, LookupEntityTypesByClassIRI(""))
	assert.Nil(t, LookupEntityTypesByClassIRI("http://example.org/unknown"))

	// Returned slice is a defensive copy.
	got := LookupEntityTypesByClassIRI(ccoAircraft)
	got[0] = "mutated.type"
	assert.Equal(t, []string{"air.fixed-wing", "uxs.uas"}, LookupEntityTypesByClassIRI(ccoAircraft))

	// SnapshotRegistry and ClearRegistry include entity types.
	restore := SnapshotRegistry()
	ClearRegistry()
	assert.Empty(t, ListRegisteredEntityTypes())
	assert.Nil(t, LookupEntityTypesByClassIRI(ccoAircraft))

	restore()
	assert.Equal(t, []string{"air.fixed-wing", "uxs.uas"}, LookupEntityTypesByClassIRI(ccoAircraft))
}

func FuzzParseEntityTypeKey(f *testing.F) {
	for _, seed := range []string{
		"uxs.uas",
		"air-defense.radar-track",
		"",
		"onlyone",
		"three.part.key",
		"Upper.Case",
		"bad..key",
		"a.-b",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		domain, entityType, err := ParseEntityTypeKey(key)
		if err != nil {
			return
		}
		if domain+"."+entityType != key {
			t.Fatalf("ParseEntityTypeKey(%q) = (%q, %q), does not reconstruct key", key, domain, entityType)
		}
		if _, predErr := ParsePredicate(domain + "." + entityType + ".probe"); predErr != nil {
			t.Fatalf("ParseEntityTypeKey(%q) accepted segment rejected by ParsePredicate: %v", key, predErr)
		}
	})
}
