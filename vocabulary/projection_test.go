package vocabulary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterWithProjection(t *testing.T) {
	defer SnapshotRegistry()()
	ClearRegistry()

	spec := ProjectionSpec{
		Steps: []ProjectionStep{
			{
				PredicateIRI: "http://purl.obolibrary.org/obo/RO_0000056",
				NodeClassIRI: "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring",
			},
			{
				PredicateIRI:      "http://www.ontologyrepository.com/CommonCoreOntologies/has_output",
				NodeClassIRI:      "http://www.ontologyrepository.com/CommonCoreOntologies/TemperatureMeasurementInformationContentEntity",
				ValuePredicateIRI: "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value",
			},
		},
	}

	Register("sensor.temperature.celsius",
		WithDescription("Ambient temperature in degrees Celsius"),
		WithDataType(DataTypeFloat),
		WithIRI("http://www.ontologyrepository.com/CommonCoreOntologies/has_temperature"),
		WithProjection(spec))

	// Mutating the caller's spec after Register must not affect the stored projection.
	spec.Steps[0].PredicateIRI = "http://example.org/mutated-caller"

	meta := GetPredicateMetadata("sensor.temperature.celsius")
	require.NotNil(t, meta)
	require.NotNil(t, meta.Projection)
	require.Len(t, meta.Projection.Steps, 2)
	assert.Equal(t, "http://purl.obolibrary.org/obo/RO_0000056", meta.Projection.Steps[0].PredicateIRI)
	assert.Equal(t, "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring", meta.Projection.Steps[0].NodeClassIRI)
	assert.Equal(t, "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value", meta.Projection.Steps[1].ValuePredicateIRI)

	// Mutating the returned metadata's Projection slice must not affect registry state.
	meta.Projection.Steps[0].PredicateIRI = "http://example.org/mutated-returned"
	again := GetPredicateMetadata("sensor.temperature.celsius")
	require.NotNil(t, again)
	require.NotNil(t, again.Projection)
	assert.Equal(t, "http://purl.obolibrary.org/obo/RO_0000056", again.Projection.Steps[0].PredicateIRI)

	// Amending without WithProjection preserves the existing Projection.
	Register("sensor.temperature.celsius", WithDescription("Updated temperature description"))
	amended := GetPredicateMetadata("sensor.temperature.celsius")
	require.NotNil(t, amended)
	assert.Equal(t, "Updated temperature description", amended.Description)
	require.NotNil(t, amended.Projection)
	assert.Len(t, amended.Projection.Steps, 2)

	// Invalid projections panic at registration and leave the registry clean.
	assert.Panics(t, func() {
		Register("sensor.temperature.empty-proj", WithProjection(ProjectionSpec{}))
	})
	assert.Panics(t, func() {
		Register("sensor.temperature.blank-pred", WithProjection(ProjectionSpec{
			Steps: []ProjectionStep{{PredicateIRI: "   "}},
		}))
	})
	assert.Panics(t, func() {
		Register("sensor.temperature.space-iri", WithProjection(ProjectionSpec{
			Steps: []ProjectionStep{{PredicateIRI: "http://example.org/bad iri"}},
		}))
	})
	assert.Panics(t, func() {
		Register("sensor.temperature.missing-intermediate-class", WithProjection(ProjectionSpec{
			Steps: []ProjectionStep{
				{PredicateIRI: "http://example.org/step1"},
				{PredicateIRI: "http://example.org/step2", ValuePredicateIRI: "http://example.org/val"},
			},
		}))
	})
	assert.Nil(t, GetPredicateMetadata("sensor.temperature.empty-proj"))
}

func TestProjectionSpecValidate(t *testing.T) {
	t.Parallel()

	validOneStep := ProjectionSpec{
		Steps: []ProjectionStep{
			{
				PredicateIRI:      "http://purl.obolibrary.org/obo/RO_0000056",
				NodeClassIRI:      "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring",
				ValuePredicateIRI: "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value",
			},
		},
	}
	require.NoError(t, validOneStep.Validate())

	validTwoStep := ProjectionSpec{
		Steps: []ProjectionStep{
			{
				PredicateIRI: "http://purl.obolibrary.org/obo/RO_0000056",
				NodeClassIRI: "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring",
			},
			{
				PredicateIRI:      "https://confirm.mil/diekm/hasOutput",
				ValuePredicateIRI: "https://confirm.mil/diekm/hasDecimalValue",
			},
		},
	}
	require.NoError(t, validTwoStep.Validate())

	invalidCases := []struct {
		name string
		spec ProjectionSpec
	}{
		{
			name: "empty steps",
			spec: ProjectionSpec{},
		},
		{
			name: "empty predicate_iri",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{PredicateIRI: "", ValuePredicateIRI: "http://example.org/val"},
				},
			},
		},
		{
			name: "non-http predicate_iri scheme",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{PredicateIRI: "urn:example:pred", ValuePredicateIRI: "http://example.org/val"},
				},
			},
		},
		{
			name: "non-http node_class_iri scheme",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{
						PredicateIRI:      "http://example.org/pred",
						NodeClassIRI:      "urn:example:Class",
						ValuePredicateIRI: "http://example.org/val",
					},
				},
			},
		},
		{
			name: "intermediate step missing node_class_iri",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{PredicateIRI: "http://example.org/step1"},
					{PredicateIRI: "http://example.org/step2", ValuePredicateIRI: "http://example.org/val"},
				},
			},
		},
		{
			name: "intermediate step with value_predicate_iri",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{
						PredicateIRI:      "http://example.org/step1",
						NodeClassIRI:      "http://example.org/Class1",
						ValuePredicateIRI: "http://example.org/earlyVal",
					},
					{
						PredicateIRI:      "http://example.org/step2",
						ValuePredicateIRI: "http://example.org/val",
					},
				},
			},
		},
		{
			name: "terminal step missing value_predicate_iri",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{
						PredicateIRI: "http://example.org/step1",
						NodeClassIRI: "http://example.org/Class1",
					},
				},
			},
		},
		{
			name: "terminal step non-http value_predicate_iri",
			spec: ProjectionSpec{
				Steps: []ProjectionStep{
					{
						PredicateIRI:      "http://example.org/step1",
						NodeClassIRI:      "http://example.org/Class1",
						ValuePredicateIRI: "urn:example:val",
					},
				},
			},
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.spec.Validate())
		})
	}
}
