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
	recordPrefixes := func(prefixes []PrefixSpec) error {
		staged := make(map[string]string, len(prefixes))
		for _, pfx := range prefixes {
			if err := pfx.Validate(); err != nil {
				return err
			}
			if existing, ok := registeredPrefixes[pfx.Prefix]; ok && existing != pfx.Namespace {
				return fmt.Errorf("prefix %q conflict", pfx.Prefix)
			}
			if existing, ok := staged[pfx.Prefix]; ok && existing != pfx.Namespace {
				return fmt.Errorf("prefix %q intra-batch conflict", pfx.Prefix)
			}
			staged[pfx.Prefix] = pfx.Namespace
		}
		for k, v := range staged {
			registeredPrefixes[k] = v
		}
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

	require.NoError(t, RegisterPack(validPack, WithPrefixRegistrar(recordPrefixes)))
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

	err := RegisterPack(badPack, WithPrefixRegistrar(recordPrefixes))
	require.Error(t, err)
	assert.Nil(t, GetPredicateMetadata("uxs.telemetry.speed"), "valid entry before invalid predicate must not be committed")
	assert.Nil(t, GetEntityTypeMetadata("uxs.ugv"), "entity type in rejected pack must not be committed")
	assert.NotContains(t, registeredPrefixes, "newpfx", "prefix in rejected pack must not be committed")

	// Atomic failure on late prefix conflict: first prefix in pack is new,
	// second conflicts with existing "diekm" binding.
	lateConflictPack := PackSpec{
		Name: "late-prefix-conflict",
		Prefixes: []PrefixSpec{
			{Prefix: "review235", Namespace: "https://example.org/first/"},
			{Prefix: "diekm", Namespace: "https://example.org/conflict/"},
		},
		Predicates: []PredicateSpec{
			{Name: "review.pack.first"},
		},
	}
	require.Error(t, RegisterPack(lateConflictPack, WithPrefixRegistrar(recordPrefixes)))
	assert.NotContains(t, registeredPrefixes, "review235", "earlier prefix in rejected batch must not remain installed")
	assert.Nil(t, GetPredicateMetadata("review.pack.first"))

	// Malformed Turtle PN_PREFIX labels are rejected before any state changes.
	for _, badPrefix := range []string{"", "bad:", ":lead", ".dot", "dot.", "1digit", "-dash", "_under", "has space"} {
		err := RegisterPack(PackSpec{
			Name:     "bad-prefix-pack",
			Prefixes: []PrefixSpec{{Prefix: badPrefix, Namespace: "https://example.org/ns/"}},
			Predicates: []PredicateSpec{
				{Name: "review.prefix.pred"},
			},
		}, WithPrefixRegistrar(recordPrefixes))
		require.Error(t, err, "expected error for invalid prefix %q", badPrefix)
		assert.Nil(t, GetPredicateMetadata("review.prefix.pred"))
	}

	// Malformed Turtle IRIREF namespaces and predicate IRIs are rejected (#237).
	for _, badNS := range []string{"", "https://bad.example/>", "https://bad.example/<", `https://bad.example/"`, "not-an-iri", ":noscheme"} {
		require.Error(t, (PrefixSpec{Prefix: "badns", Namespace: badNS}).Validate(), "expected error for invalid namespace %q", badNS)
		err := RegisterPack(PackSpec{
			Name:     "bad-ns-pack",
			Prefixes: []PrefixSpec{{Prefix: "badns", Namespace: badNS}},
			Predicates: []PredicateSpec{
				{Name: "review.bad.namespace", IRI: "https://example.org/valid"},
			},
		}, WithPrefixRegistrar(recordPrefixes))
		require.Error(t, err, "expected error for invalid pack namespace %q", badNS)
		assert.NotContains(t, registeredPrefixes, "badns")
		assert.Nil(t, GetPredicateMetadata("review.bad.namespace"))
	}

	// Incompatible supplied alias_priority without is_alias or alias_type is rejected (#251).
	aliasPriorityPack, err := LoadPackJSON(strings.NewReader(`{"name":"alias-pack","predicates":[{"name":"review.alias.priority","alias_priority":42}]}`))
	require.NoError(t, err)
	require.Error(t, RegisterPack(aliasPriorityPack))
	assert.Nil(t, GetPredicateMetadata("review.alias.priority"))

	// Intra-pack asymmetric inverse declarations are rejected regardless of order.
	inconsistentInversePack := PackSpec{
		Name: "inconsistent-inverse",
		Predicates: []PredicateSpec{
			{Name: "review.rel.first", InverseOf: "review.rel.second"},
			{Name: "review.rel.second", InverseOf: "review.rel.third"},
		},
	}
	require.Error(t, RegisterPack(inconsistentInversePack))
	assert.Nil(t, GetPredicateMetadata("review.rel.first"))
	assert.Nil(t, GetPredicateMetadata("review.rel.second"))
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
		prefixes := rapid.SliceOfNDistinct(segmentGen, 1, 3, func(s string) string { return s }).Draw(t, "prefixes")
		preds := rapid.SliceOfNDistinct(predicateGen, 1, 4, func(s string) string { return s }).Draw(t, "preds")
		etypes := rapid.SliceOfNDistinct(entityTypeGen, 1, 4, func(s string) string { return s }).Draw(t, "etypes")
		withProj := rapid.Bool().Draw(t, "with_proj")

		pack := PackSpec{
			Name:    "prop-" + segmentGen.Draw(t, "pack_name"),
			Version: "1.0.0",
		}
		for _, pfx := range prefixes {
			pack.Prefixes = append(pack.Prefixes, PrefixSpec{
				Prefix:    pfx,
				Namespace: "https://example.org/ns/" + pfx + "/",
			})
		}
		for _, p := range preds {
			spec := PredicateSpec{
				Name:        p,
				Description: "desc " + p,
				DataType:    DataTypeString,
				IRI:         "https://example.org/pred/" + p,
			}
			if withProj {
				spec.Projection = &ProjectionSpec{
					Steps: []ProjectionStep{
						{
							PredicateIRI:      "https://example.org/proj/step/" + p,
							NodeClassIRI:      "https://example.org/proj/Node",
							ValuePredicateIRI: "https://example.org/proj/value",
						},
					},
				}
			}
			pack.Predicates = append(pack.Predicates, spec)
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
		var gotJSONPrefixes []PrefixSpec
		require.NoError(t, RegisterPack(fromJSON, WithPrefixRegistrar(func(pfxs []PrefixSpec) error {
			gotJSONPrefixes = pfxs
			return nil
		})))
		assert.Equal(t, pack.Prefixes, gotJSONPrefixes)
		for _, p := range pack.Predicates {
			assert.Contains(t, LookupByIRI(p.IRI), p.Name)
			meta := GetPredicateMetadata(p.Name)
			require.NotNil(t, meta)
			assert.Equal(t, p.Projection, meta.Projection)
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
		var gotYAMLPrefixes []PrefixSpec
		require.NoError(t, RegisterPack(fromYAML, WithPrefixRegistrar(func(pfxs []PrefixSpec) error {
			gotYAMLPrefixes = pfxs
			return nil
		})))
		assert.Equal(t, pack.Prefixes, gotYAMLPrefixes)
		for _, p := range pack.Predicates {
			assert.Contains(t, LookupByIRI(p.IRI), p.Name)
			meta := GetPredicateMetadata(p.Name)
			require.NotNil(t, meta)
			assert.Equal(t, p.Projection, meta.Projection)
		}
		for _, et := range pack.EntityTypes {
			assert.Contains(t, LookupEntityTypesByClassIRI(et.ClassIRI), et.DomainType)
		}
	})
}

func FuzzLoadPackJSON(f *testing.F) {
	for _, seed := range []string{
		`{"name":"bfo","version":"2020","prefixes":[{"prefix":"bfo","namespace":"http://purl.obolibrary.org/obo/BFO_"}],"predicates":[{"name":"bfo.rel.part-of","iri":"http://purl.obolibrary.org/obo/BFO_0000050","projection":{"steps":[{"predicate_iri":"http://purl.obolibrary.org/obo/RO_0000056","node_class_iri":"http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring","value_predicate_iri":"http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value"}]}}]}`,
		`{"name":"alias-pack","predicates":[{"name":"review.alias.priority","alias_priority":42}]}`,
		`{"name":"bad-ns","prefixes":[{"prefix":"badns","namespace":"https://bad.example/>"}]}`,
		`{"name":"bad","unknown":1}`,
		`{}`,
		``,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		pack, err := LoadPackJSON(strings.NewReader(input))
		if err != nil {
			return
		}
		defer SnapshotRegistry()()
		ClearRegistry()
		if err := RegisterPack(pack); err != nil {
			assert.Empty(t, ListRegisteredPredicates(), "rejected pack must leave predicate registry unchanged")
			assert.Empty(t, ListRegisteredEntityTypes(), "rejected pack must leave entity-type registry unchanged")
			return
		}
		for _, pfx := range pack.Prefixes {
			require.NoError(t, pfx.Validate())
		}
		for _, p := range pack.Predicates {
			require.NotNil(t, GetPredicateMetadata(p.Name))
		}
		for _, et := range pack.EntityTypes {
			require.NotNil(t, GetEntityTypeMetadata(et.DomainType))
		}
	})
}

func FuzzLoadPackYAML(f *testing.F) {
	for _, seed := range []string{
		"name: bfo\nversion: \"2020\"\nprefixes:\n  - prefix: bfo\n    namespace: http://purl.obolibrary.org/obo/BFO_\npredicates:\n  - name: bfo.rel.part-of\n    iri: http://purl.obolibrary.org/obo/BFO_0000050\n",
		"name: alias-pack\npredicates:\n  - name: review.alias.priority\n    alias_priority: 42\n",
		"name: bad-ns\nprefixes:\n  - prefix: badns\n    namespace: https://bad.example/>\n",
		"name: bad\nunknown: 1\n",
		"{}",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		pack, err := LoadPackYAML(strings.NewReader(input))
		if err != nil {
			return
		}
		defer SnapshotRegistry()()
		ClearRegistry()
		if err := RegisterPack(pack); err != nil {
			assert.Empty(t, ListRegisteredPredicates(), "rejected pack must leave predicate registry unchanged")
			assert.Empty(t, ListRegisteredEntityTypes(), "rejected pack must leave entity-type registry unchanged")
			return
		}
		for _, pfx := range pack.Prefixes {
			require.NoError(t, pfx.Validate())
		}
		for _, p := range pack.Predicates {
			require.NotNil(t, GetPredicateMetadata(p.Name))
		}
		for _, et := range pack.EntityTypes {
			require.NotNil(t, GetEntityTypeMetadata(et.DomainType))
		}
	})
}
