package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-substitution/"A token ends where its name ends" and "An
// unresolved token stays and is reported", for $entity.id and the six parts.

func TestEntityPartsValidSixPartID(t *testing.T) {
	t.Parallel()

	id := semantictest.EntityID(t, "c360", "osh-demo-001", "agentic-loop", "agent", "execution", "c1e90237-1cd5-4def-99ab-aabbccddeeff")
	ec := &ExecutionContext{EntityID: id}

	in := "org=$entity.org plat=$entity.platform domain=$entity.domain " +
		"system=$entity.system type=$entity.type instance=$entity.instance"
	want := "org=c360 plat=osh-demo-001 domain=agent system=agentic-loop " +
		"type=execution instance=c1e90237-1cd5-4def-99ab-aabbccddeeff"
	assert.Equal(t, want, ec.SubstituteVariables(t.Context(), in))
}

// An entity ID that is not a valid six-part ID resolves no part, so the tokens
// stay and are reported: rendering them empty would hide the mistake and let a
// bad key or subject through.
func TestEntityPartsInvalidIDLeavesTokens(t *testing.T) {
	log := captureWarnings(t)

	for name, id := range map[string]string{
		"empty":                "",
		"three parts":          "a.b.c",
		"five parts":           "a.b.c.d.e",
		"seven parts":          "a.b.c.d.e.f.g",
		"invalid char in part": "c360.osh.agent.loop.execution.uuid$bad",
	} {
		t.Run(name, func(t *testing.T) {
			ec := &ExecutionContext{EntityID: id}
			in := "instance=$entity.instance type=$entity.type"

			assert.Equal(t, in, ec.SubstituteVariables(t.Context(), in))
			assert.Equal(t, []string{"$entity.instance", "$entity.type"}, log.unresolved(t))
		})
	}
}

// A template can carry both the whole ID and one of its parts: a log line
// wants the readable ID, a tool argument the instance.
func TestEntityPartsBesideID(t *testing.T) {
	t.Parallel()

	id := semantictest.EntityID(t, "c360", "osh-demo-001", "agentic-loop", "agent", "execution", "c1e90237")
	ec := &ExecutionContext{EntityID: id}

	assert.Equal(t, "loop=c1e90237 full="+id, ec.SubstituteVariables(t.Context(), "loop=$entity.instance full=$entity.id"))
}

// A part token and a triple whose predicate ends in the same word resolve
// apart, and $entity.id does not take the start of $entity.instance.
func TestEntityPartsBesideIDAndTriple(t *testing.T) {
	t.Parallel()

	id := semantictest.EntityID(t, "acme", "ops", "robotics", "gcs", "drone", "001")
	ec := &ExecutionContext{
		EntityID: id,
		Entity: &gtypes.EntityState{Triples: []message.Triple{
			{Predicate: semantictest.Predicate(t, "test", "fixture", "org"), Object: "triple-derived-org"},
		}},
	}

	in := "part=$entity.org triple=$entity.triple.test.fixture.org id=$entity.id instance=$entity.instance"
	want := "part=acme triple=triple-derived-org id=" + id + " instance=001"
	assert.Equal(t, want, ec.SubstituteVariables(t.Context(), in))
}

// When one predicate extends another (researcher-plan and
// researcher-plan-entity), each reference reads its own predicate, whatever
// order the triples are in.
func TestTriplePredicateExtendingAnother(t *testing.T) {
	t.Parallel()

	planUUID := "78a0e9ed-3da3-4bc4-ae33-1c3c8f4a0001"
	planEntity := semantictest.EntityID(t, "c360", "bootstrap-001", "agentic-loop", "agent", "execution", planUUID)
	short := semantictest.Predicate(t, "test", "lineage", "researcher-plan")
	long := semantictest.Predicate(t, "test", "lineage", "researcher-plan-entity")

	for name, triples := range map[string][]message.Triple{
		"shorter predicate first": {{Predicate: short, Object: planUUID}, {Predicate: long, Object: planEntity}},
		"longer predicate first":  {{Predicate: long, Object: planEntity}, {Predicate: short, Object: planUUID}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ec := &ExecutionContext{
				EntityID: semantictest.EntityID(t, "c360", "bootstrap-001", "agentic-loop", "agent", "execution", "child"),
				Entity:   &gtypes.EntityState{Triples: triples},
			}

			in := "subject=$entity.triple.test.lineage.researcher-plan-entity short=$entity.triple.test.lineage.researcher-plan"
			assert.Equal(t, "subject="+planEntity+" short="+planUUID, ec.SubstituteVariables(t.Context(), in))
		})
	}
}

// $entity.system and $entity.domain are the third and fourth parts by name.
func TestEntityPartsResolveByName(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{EntityID: semantictest.EntityID(t, "acme", "dep1", "src", "git", "commit", "a1")}

	assert.Equal(t, "src=src dom=git id=a1",
		ec.SubstituteVariables(t.Context(), "src=$entity.system dom=$entity.domain id=$entity.instance"))
	assert.Equal(t, "acme/dep1/src/git/commit/a1",
		ec.SubstituteVariables(t.Context(), "$entity.org/$entity.platform/$entity.system/$entity.domain/$entity.type/$entity.instance"))
}

func TestEntityPartsUnresolvedOnFivePartID(t *testing.T) {
	log := captureWarnings(t)
	ec := &ExecutionContext{EntityID: "acme.dep1.src.git.commit"}

	in := "src=$entity.system dom=$entity.domain id=$entity.instance"
	assert.Equal(t, in, ec.SubstituteVariables(t.Context(), in))
	assert.Equal(t, []string{"$entity.system", "$entity.domain", "$entity.instance"}, log.unresolved(t))
}
