package graphingest

// The time of the statements graph-ingest derives (design D15, #98; owner ruling, #91 comment
// 6066791396): the indexing profile, explicit or the floor, hierarchy statements and a hierarchy
// container's statements carry the latest Timestamp among the write's own statements, on every
// lane, reserved sources included, and never the clock. Each case drives a lane's production
// entry: the consume closure Start wires for the entity_stream port, the extract-then-ingest
// path behind it, the canonical create handler and the in-process create.

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/nats-io/nats.go/jetstream"
)

// Times far from the wall clock, so a statement stamped from the clock cannot equal one. The
// wire carries the envelope's creation time in milliseconds, so each is a whole millisecond.
var (
	derivedEnvelopeTime = time.UnixMilli(1_500_000_000_000)
	derivedLaterTime    = derivedEnvelopeTime.Add(90 * time.Minute)
	derivedEarlierTime  = derivedEnvelopeTime.Add(-90 * time.Minute)
)

// derivedSource is the producer graph-ingest names in a statement it derives with predicate.
func derivedSource(predicate string) string {
	if predicate == vocabulary.EntityIndexingProfile {
		return graph.SourceIndexingProfile
	}
	return graph.SourceHierarchy
}

// assertDerivedStatements checks every stored statement of entity whose predicate is not one of
// own: it names graph-ingest's producer and carries at. It returns how many profile and hierarchy
// statements it checked.
func assertDerivedStatements(t *testing.T, entity *graph.EntityState, at time.Time, own ...string) (profile, hierarchy int) {
	t.Helper()
	for _, triple := range entity.Triples {
		if slices.Contains(own, triple.Predicate) {
			continue
		}
		if triple.Predicate == vocabulary.EntityIndexingProfile {
			profile++
		} else {
			hierarchy++
		}
		if want := derivedSource(triple.Predicate); triple.Source != want || !triple.Timestamp.Equal(at) {
			t.Errorf("%s: derived %s stored with source %q at %s, want %q at %s",
				entity.ID, triple.Predicate, triple.Source, triple.Timestamp, want, at)
		}
	}
	return profile, hierarchy
}

// assertBirthDerivations checks the birth of id with hierarchy on: the entity and every
// container its birth created carry their derived statements at at, the entity has a profile and
// a hierarchy statement, each container has a profile, and no hierarchy inference was refused.
func assertBirthDerivations(t *testing.T, c *Component, logs *bytes.Buffer, id string, at time.Time, own ...string) {
	t.Helper()
	profile, hierarchy := assertDerivedStatements(t, storedEntity(t, c, id), at, own...)
	if profile != 1 || hierarchy == 0 {
		t.Errorf("%s carries %d profile and %d hierarchy statements, want 1 and at least one", id, profile, hierarchy)
	}
	keys, err := c.entityBucket.Keys(context.Background())
	if err != nil {
		t.Fatalf("list stored entities: %v", err)
	}
	containers := 0
	for _, key := range keys {
		if key == id {
			continue
		}
		containers++
		// A container's statements are all graph-ingest's: its type statement and the inverse
		// edges under graph-ingest-hierarchy, and its profile.
		if profile, _ := assertDerivedStatements(t, storedEntity(t, c, key), at); profile != 1 {
			t.Errorf("container %s carries %d profile statements, want 1", key, profile)
		}
	}
	if containers == 0 {
		t.Errorf("the birth of %s created no hierarchy container: stored %v", id, keys)
	}
	if strings.Contains(logs.String(), "Failed to get hierarchy triples") {
		t.Errorf("hierarchy inference was refused during the birth: %s", logs.String())
	}
}

// startHierarchyStreamLane is startStreamLane with hierarchy inference on and graph-ingest's log
// written to the returned buffer.
func startHierarchyStreamLane(t *testing.T) (*Component, func(context.Context, jetstream.Msg), *bytes.Buffer) {
	t.Helper()
	c, handler := startStreamLane(t)
	var logs bytes.Buffer
	c.logger = slog.New(slog.NewTextHandler(&logs, nil))
	c.config.EnableHierarchy = true
	buildHierarchyInference(t, c)
	return c, handler, &logs
}

func TestDerivedStatementsCarryTriggeringTime(t *testing.T) {
	t.Run("stream birth takes the envelope's creation time", func(t *testing.T) {
		c, handler, logs := startHierarchyStreamLane(t)
		const id = "c360.test.robotics.mav1.drone.001"
		payload := &mergeTestGraphable{entityID: id, triples: []message.Triple{
			{Subject: id, Predicate: "robotics.status.armed", Object: true, Confidence: 1},
		}}
		data := streamLaneData(t, payload, "derive-producer", message.WithTime(derivedEnvelopeTime))
		if got := deliverStreamLane(t, handler, data, 1); got != "ack" {
			t.Fatalf("disposition = %s, want ack", got)
		}
		assertBirthDerivations(t, c, logs, id, derivedEnvelopeTime, "robotics.status.armed")
	})

	t.Run("stream birth takes a later statement time", func(t *testing.T) {
		c, handler, logs := startHierarchyStreamLane(t)
		const id = "c360.test.robotics.mav1.drone.002"
		// The later statement is not the last, so neither the first nor the last statement's
		// time is the latest.
		payload := &mergeTestGraphable{entityID: id, triples: []message.Triple{
			{Subject: id, Predicate: "robotics.status.armed", Object: true, Confidence: 1},
			{Subject: id, Predicate: "robotics.status.mode", Object: "auto", Timestamp: derivedLaterTime, Confidence: 1},
			{Subject: id, Predicate: "robotics.status.battery", Object: 87, Timestamp: derivedEarlierTime, Confidence: 1},
		}}
		data := streamLaneData(t, payload, "derive-producer", message.WithTime(derivedEnvelopeTime))
		if got := deliverStreamLane(t, handler, data, 1); got != "ack" {
			t.Fatalf("disposition = %s, want ack", got)
		}
		assertBirthDerivations(t, c, logs, id, derivedLaterTime,
			"robotics.status.armed", "robotics.status.mode", "robotics.status.battery")
	})

	t.Run("stream birth's explicit profile takes the triggering time", func(t *testing.T) {
		c, _, logs := startHierarchyStreamLane(t)
		const id = "c360.test.robotics.mav1.drone.003"
		payload := &testGraphablePayload{
			id: id,
			triples: []message.Triple{
				{Subject: id, Predicate: "robotics.status.mode", Object: "auto", Timestamp: derivedLaterTime, Confidence: 1},
				{Subject: id, Predicate: "robotics.status.armed", Object: true, Confidence: 1},
			},
			profile: vocabulary.IndexingProfileContent,
		}
		msg := message.NewBaseMessage(payload.Schema(), payload, "derive-producer", message.WithTime(derivedEnvelopeTime))
		entity, err := c.extractEntityFromMessage(msg)
		if err != nil {
			t.Fatalf("extractEntityFromMessage: %v", err)
		}
		if err := c.ingestEntity(t.Context(), entity, false); err != nil {
			t.Fatalf("ingestEntity: %v", err)
		}
		stored := storedEntity(t, c, id)
		if got := storedStatement(t, stored, vocabulary.EntityIndexingProfile).Object; got != vocabulary.IndexingProfileContent {
			t.Errorf("profile = %v, want the declared %s", got, vocabulary.IndexingProfileContent)
		}
		assertBirthDerivations(t, c, logs, id, derivedLaterTime, "robotics.status.mode", "robotics.status.armed")
	})

	// A stream arrival for an existing entity with no profile stamps one. It takes the arrival's
	// time, not the stored statements': the stored one is later than the arrival's.
	t.Run("stream merge into an unprofiled entity takes the arrival's time", func(t *testing.T) {
		c, handler := startStreamLane(t)
		const id = "c360.test.robotics.mav1.drone.004"
		seed := graph.EntityState{
			ID:          id,
			MessageType: testEntityType(),
			Triples: []message.Triple{
				{Subject: id, Predicate: "robotics.status.mode", Object: "manual", Source: "other-lane", Timestamp: derivedLaterTime, Confidence: 1},
			},
			UpdatedAt: derivedLaterTime,
		}
		encoded, err := graph.MarshalEntityState(&seed)
		if err != nil {
			t.Fatalf("encode seed entity: %v", err)
		}
		if _, err := c.entityBucket.Put(t.Context(), id, encoded); err != nil {
			t.Fatalf("store seed entity: %v", err)
		}
		payload := &mergeTestGraphable{entityID: id, triples: []message.Triple{
			{Subject: id, Predicate: "robotics.status.armed", Object: true, Confidence: 1},
		}}
		data := streamLaneData(t, payload, "derive-producer", message.WithTime(derivedEnvelopeTime))
		if got := deliverStreamLane(t, handler, data, 1); got != "ack" {
			t.Fatalf("disposition = %s, want ack", got)
		}
		if profile, _ := assertDerivedStatements(t, storedEntity(t, c, id), derivedEnvelopeTime,
			"robotics.status.mode", "robotics.status.armed"); profile != 1 {
			t.Errorf("%s carries %d profile statements, want 1", id, profile)
		}
	})

	t.Run("mutation create takes its latest statement time", func(t *testing.T) {
		c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
		statement := func(id, predicate string, at time.Time) message.Triple {
			return message.Triple{Subject: id, Predicate: predicate, Object: "set", Source: "derive-writer", Timestamp: at}
		}
		for _, tt := range []struct {
			name    string
			id      string
			profile string
		}{
			{"floor profile", "acme.ops.robotics.gcs.drone.010", ""},
			{"explicit profile", "acme.ops.robotics.gcs.drone.011", vocabulary.IndexingProfileSignal},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := c.handleCanonicalCreate(t.Context(), mustCanonicalJSON(t, graph.CreateEntityRequest{
					Entity: canonicalMutationEntity(tt.id),
					Triples: []message.Triple{
						statement(tt.id, "test.state.first", derivedEarlierTime),
						statement(tt.id, "test.state.second", derivedLaterTime),
						statement(tt.id, "test.state.third", derivedEnvelopeTime),
					},
					IndexingProfile: tt.profile,
				}))
				if err != nil {
					t.Fatalf("handleCanonicalCreate: %v", err)
				}
				stored := storedEntity(t, c, tt.id)
				if profile, _ := assertDerivedStatements(t, stored, derivedLaterTime,
					"test.state.first", "test.state.second", "test.state.third"); profile != 1 {
					t.Errorf("%s carries %d profile statements, want 1", tt.id, profile)
				}
			})
		}
	})

	t.Run("lifecycle create takes its own time", func(t *testing.T) {
		// pkg/lifecycle writes on the mutation lane under its reserved source, and its statements
		// are the write's own.
		c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
		const id = "acme.ops.robotics.gcs.drone.020"
		lifecycle := func(predicate string, at time.Time) message.Triple {
			return message.Triple{Subject: id, Predicate: predicate, Object: "active", Source: graph.SourceLifecycle, Timestamp: at}
		}
		if _, err := c.handleCanonicalCreate(t.Context(), mustCanonicalJSON(t, graph.CreateEntityRequest{
			Entity:  canonicalMutationEntity(id),
			Triples: []message.Triple{lifecycle("test.lifecycle.state", derivedLaterTime), lifecycle("test.lifecycle.phase", derivedEarlierTime)},
		})); err != nil {
			t.Fatalf("handleCanonicalCreate: %v", err)
		}
		if profile, _ := assertDerivedStatements(t, storedEntity(t, c, id), derivedLaterTime,
			"test.lifecycle.state", "test.lifecycle.phase"); profile != 1 {
			t.Errorf("%s carries %d profile statements, want 1", id, profile)
		}
	})

	t.Run("in-process create takes its latest statement time", func(t *testing.T) {
		c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
		var logs bytes.Buffer
		c.logger = slog.New(slog.NewTextHandler(&logs, nil))
		c.config.EnableHierarchy = true
		buildHierarchyInference(t, c)
		const id = "acme.ops.robotics.gcs.drone.030"
		entity := canonicalMutationEntity(id)
		entity.Triples = []message.Triple{
			{Subject: id, Predicate: "test.state.first", Object: "a", Source: "derive-writer", Timestamp: derivedLaterTime},
			{Subject: id, Predicate: "test.state.second", Object: "b", Source: "derive-writer", Timestamp: derivedEarlierTime},
		}
		if err := c.CreateEntity(t.Context(), entity); err != nil {
			t.Fatalf("CreateEntity: %v", err)
		}
		assertBirthDerivations(t, c, &logs, id, derivedLaterTime, "test.state.first", "test.state.second")
	})
}
