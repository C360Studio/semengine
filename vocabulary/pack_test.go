package vocabulary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

func TestLoadPackJSON(t *testing.T) {
	t.Parallel()

	const validJSON = `{
		"name": "diekm-core",
		"version": "1.0.0",
		"prefixes": [
			{"prefix": "bfo", "namespace": "http://purl.obolibrary.org/obo/"},
			{"prefix": "cco", "namespace": "http://www.ontologyrepository.com/CommonCoreOntologies/"},
			{"prefix": "diekm", "namespace": "https://confirm.mil/diekm/"}
		],
		"predicates": [
			{
				"name": "uxs.telemetry.altitude",
				"description": "Barometric altitude in meters",
				"data_type": "float",
				"units": "meters",
				"iri": "https://confirm.mil/diekm/hasAltitude",
				"projection": {
					"steps": [
						{
							"predicate_iri": "http://purl.obolibrary.org/obo/RO_0000056",
							"node_class_iri": "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring",
							"value_predicate_iri": "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value"
						}
					]
				}
			}
		],
		"entity_types": [
			{
				"domain_type": "uxs.uas",
				"description": "Unmanned Aerial System",
				"class_iri": "https://confirm.mil/diekm/UnmannedAerialSystem",
				"superclasses": [
					"http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft"
				]
			}
		]
	}`

	pack, err := LoadPackJSON(strings.NewReader(validJSON))
	require.NoError(t, err)
	assert.Equal(t, "diekm-core", pack.Name)
	assert.Equal(t, "1.0.0", pack.Version)
	require.Len(t, pack.Prefixes, 3)
	require.Len(t, pack.Predicates, 1)
	require.NotNil(t, pack.Predicates[0].Projection)
	require.Len(t, pack.EntityTypes, 1)

	// Unknown field is refused.
	_, err = LoadPackJSON(strings.NewReader(`{"name": "bad", "unknown_field": true}`))
	require.Error(t, err)

	// Trailing JSON value is refused.
	_, err = LoadPackJSON(strings.NewReader(`{"name": "one"}{"name": "two"}`))
	require.Error(t, err)

	// Empty input is refused.
	_, err = LoadPackJSON(strings.NewReader(""))
	require.Error(t, err)
}

func TestLoadPackYAML(t *testing.T) {
	t.Parallel()

	const validYAML = `
name: diekm-core
version: 1.0.0
prefixes:
  - prefix: bfo
    namespace: http://purl.obolibrary.org/obo/
  - prefix: diekm
    namespace: https://confirm.mil/diekm/
predicates:
  - name: uxs.identity.callsign
    description: Tactical callsign
    data_type: string
    iri: https://confirm.mil/diekm/hasCallsign
    alias_type: communication
    alias_priority: 1
entity_types:
  - domain_type: uxs.uas
    description: Unmanned Aerial System
    class_iri: https://confirm.mil/diekm/UnmannedAerialSystem
    superclasses:
      - http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft
`

	pack, err := LoadPackYAML(strings.NewReader(validYAML))
	require.NoError(t, err)
	assert.Equal(t, "diekm-core", pack.Name)
	require.Len(t, pack.Prefixes, 2)
	require.Len(t, pack.Predicates, 1)
	assert.Equal(t, AliasTypeCommunication, pack.Predicates[0].AliasType)
	require.Len(t, pack.EntityTypes, 1)

	// Unknown field is refused.
	_, err = LoadPackYAML(strings.NewReader("name: bad\nunknown_field: true\n"))
	require.Error(t, err)

	// Trailing YAML document is refused.
	_, err = LoadPackYAML(strings.NewReader("name: one\n---\nname: two\n"))
	require.Error(t, err)

	// Empty input is refused.
	_, err = LoadPackYAML(strings.NewReader(""))
	require.Error(t, err)
}

func TestRegisterPackAndAtomicOnFailure(t *testing.T) {
	defer SnapshotRegistry()()
	ClearRegistry()

	registeredPrefixes := make(map[string]string)
	recordPrefix := func(prefix, namespace string) error {
		if existing, ok := registeredPrefixes[prefix]; ok && existing != namespace {
			return fmt.Errorf("prefix %q conflict", prefix)
		}
		registeredPrefixes[prefix] = namespace
		return nil
	}

	validPack := PackSpec{
		Name:    "diekm-test",
		Version: "1.0.0",
		Prefixes: []PrefixSpec{
			{Prefix: "diekm", Namespace: "https://confirm.mil/diekm/"},
		},
		Predicates: []PredicateSpec{
			{
				Name:        "uxs.telemetry.altitude",
				Description: "Altitude",
				DataType:    DataTypeFloat,
				IRI:         "https://confirm.mil/diekm/hasAltitude",
				Projection: &ProjectionSpec{
					Steps: []ProjectionStep{
						{
							PredicateIRI:      "http://purl.obolibrary.org/obo/RO_0000056",
							NodeClassIRI:      "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring",
							ValuePredicateIRI: "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value",
						},
					},
				},
			},
		},
		EntityTypes: []EntityTypeSpec{
			{
				DomainType:   "uxs.uas",
				Description:  "Unmanned Aerial System",
				ClassIRI:     "https://confirm.mil/diekm/UnmannedAerialSystem",
				Superclasses: []string{"http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft"},
			},
		},
	}

	require.NoError(t, RegisterPack(validPack, WithPrefixRegistrar(recordPrefix)))
	assert.Equal(t, "https://confirm.mil/diekm/", registeredPrefixes["diekm"])
	assert.Equal(t, []string{"uxs.telemetry.altitude"}, LookupByIRI("https://confirm.mil/diekm/hasAltitude"))
	assert.Equal(t, []string{"uxs.uas"}, LookupEntityTypesByClassIRI("https://confirm.mil/diekm/UnmannedAerialSystem"))

	// Atomic failure check: if the second predicate or entity type is invalid,
	// neither the first predicate nor the new prefix is registered.
	badPack := PackSpec{
		Name:    "bad-pack",
		Version: "1.0.0",
		Prefixes: []PrefixSpec{
			{Prefix: "newpfx", Namespace: "https://example.org/new/"},
		},
		Predicates: []PredicateSpec{
			{
				Name:     "uxs.telemetry.speed",
				DataType: DataTypeFloat,
				IRI:      "https://confirm.mil/diekm/hasSpeed",
			},
			{
				Name:     "Invalid.Predicate.Name",
				DataType: DataTypeFloat,
			},
		},
		EntityTypes: []EntityTypeSpec{
			{
				DomainType: "uxs.ugv",
				ClassIRI:   "https://confirm.mil/diekm/UnmannedGroundVehicle",
			},
		},
	}

	err := RegisterPack(badPack, WithPrefixRegistrar(recordPrefix))
	require.Error(t, err)
	assert.Nil(t, GetPredicateMetadata("uxs.telemetry.speed"), "valid entry before invalid predicate must not be committed")
	assert.Nil(t, GetEntityTypeMetadata("uxs.ugv"), "entity type in rejected pack must not be committed")
	assert.NotContains(t, registeredPrefixes, "newpfx", "prefix in rejected pack must not be committed")
}

func TestPropPackRoundTrip(t *testing.T) {
	defer SnapshotRegistry()()

	segmentGen := rapid.StringMatching(`[a-z][a-z0-9]{2,5}`)
	predicateGen := rapid.Custom(func(t *rapid.T) string {
		return fmt.Sprintf("%s.%s.%s",
			segmentGen.Draw(t, "domain"),
			segmentGen.Draw(t, "category"),
			segmentGen.Draw(t, "property"))
	})
	entityTypeGen := rapid.Custom(func(t *rapid.T) string {
		return fmt.Sprintf("%s.%s",
			segmentGen.Draw(t, "domain"),
			segmentGen.Draw(t, "type"))
	})

	rapid.Check(t, func(t *rapid.T) {
		preds := rapid.SliceOfNDistinct(predicateGen, 1, 4, func(s string) string { return s }).Draw(t, "preds")
		etypes := rapid.SliceOfNDistinct(entityTypeGen, 1, 4, func(s string) string { return s }).Draw(t, "etypes")

		pack := PackSpec{
			Name:    "prop-" + segmentGen.Draw(t, "pack_name"),
			Version: "1.0.0",
		}
		for _, p := range preds {
			pack.Predicates = append(pack.Predicates, PredicateSpec{
				Name:        p,
				Description: "desc " + p,
				DataType:    DataTypeString,
				IRI:         "https://example.org/pred/" + p,
			})
		}
		for _, et := range etypes {
			pack.EntityTypes = append(pack.EntityTypes, EntityTypeSpec{
				DomainType:   et,
				Description:  "desc " + et,
				ClassIRI:     "https://example.org/class/" + et,
				Superclasses: []string{"https://example.org/class/Thing"},
			})
		}

		// Round-trip through JSON.
		jsonBytes, err := json.Marshal(pack)
		require.NoError(t, err)
		fromJSON, err := LoadPackJSON(bytes.NewReader(jsonBytes))
		require.NoError(t, err)

		ClearRegistry()
		require.NoError(t, RegisterPack(fromJSON))
		for _, p := range pack.Predicates {
			assert.Contains(t, LookupByIRI(p.IRI), p.Name)
		}
		for _, et := range pack.EntityTypes {
			assert.Contains(t, LookupEntityTypesByClassIRI(et.ClassIRI), et.DomainType)
		}

		// Round-trip through YAML.
		yamlBytes, err := yaml.Marshal(pack)
		require.NoError(t, err)
		fromYAML, err := LoadPackYAML(bytes.NewReader(yamlBytes))
		require.NoError(t, err)

		ClearRegistry()
		require.NoError(t, RegisterPack(fromYAML))
		for _, p := range pack.Predicates {
			assert.Contains(t, LookupByIRI(p.IRI), p.Name)
		}
		for _, et := range pack.EntityTypes {
			assert.Contains(t, LookupEntityTypesByClassIRI(et.ClassIRI), et.DomainType)
		}
	})
}

func FuzzLoadPackJSON(f *testing.F) {
	for _, seed := range []string{
		`{"name":"bfo","version":"2020","predicates":[{"name":"bfo.rel.part-of","iri":"http://purl.obolibrary.org/obo/BFO_0000050"}]}`,
		`{"name":"bad","unknown":1}`,
		`{}`,
		``,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(_ *testing.T, input string) {
		pack, err := LoadPackJSON(strings.NewReader(input))
		if err != nil {
			return
		}
		_ = ValidatePack(pack)
	})
}

func FuzzLoadPackYAML(f *testing.F) {
	for _, seed := range []string{
		"name: bfo\nversion: \"2020\"\npredicates:\n  - name: bfo.rel.part-of\n    iri: http://purl.obolibrary.org/obo/BFO_0000050\n",
		"name: bad\nunknown: 1\n",
		"{}",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(_ *testing.T, input string) {
		pack, err := LoadPackYAML(strings.NewReader(input))
		if err != nil {
			return
		}
		_ = ValidatePack(pack)
	})
}
