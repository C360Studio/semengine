package kvcatalog

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCatalogReader_ExportedSurfaceIsReadOnly pins the complete exported
// capability. Adding a method is an API change and must re-enter the exported-
// surface gate; in particular, no mutation or raw-handle escape hatch belongs
// here.
func TestCatalogReader_ExportedSurfaceIsReadOnly(t *testing.T) {
	typeOfReader := reflect.TypeOf((*CatalogReader)(nil)).Elem()
	methods := make([]string, 0, typeOfReader.NumMethod())
	for i := 0; i < typeOfReader.NumMethod(); i++ {
		methods = append(methods, typeOfReader.Method(i).Name)
	}
	sort.Strings(methods)

	assert.Equal(t, []string{
		"Get",
		"Keys",
		"ListKeys",
		"ListKeysFiltered",
		"Status",
		"Watch",
		"WatchAll",
	}, methods)
}

// TestOpenCatalogReader_InvalidNamePreservesClassification proves catalog
// resolution still fails before client access and retains the canonical
// invalid outcome shape.
func TestOpenCatalogReader_InvalidNamePreservesClassification(t *testing.T) {
	_, err := OpenCatalogReader(t.Context(), nil, "OUTGOING_INDEX_TYPO")
	require.Error(t, err)

	var classified *errs.ClassifiedError
	require.True(t, errors.As(err, &classified))
	assert.Equal(t, errs.ErrorInvalid, classified.Class)
	assert.Empty(t, classified.Code)
	assert.Nil(t, classified.Detail)
	assert.Equal(t, "graph", classified.Component)
	assert.Equal(t, "OpenCatalogReader", classified.Operation)
	assert.Contains(t, err.Error(), `bucket "OUTGOING_INDEX_TYPO" is not in the framework KV catalog`)
	wrapped := errors.Unwrap(classified)
	require.Error(t, wrapped)
	assert.EqualError(t, errors.Unwrap(wrapped), `bucket "OUTGOING_INDEX_TYPO" is not in the framework KV catalog`)
}

// TestKVCatalog_EveryRowValidates: every shipped descriptor must pass the
// seam's fail-closed validation — a catalog row the seam would refuse is a
// boot-breaking typo caught here instead.
func TestKVCatalog_EveryRowValidates(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range KVCatalog() {
		require.NoError(t, spec.Validate(), "catalog row %q must validate", spec.Name)
		assert.False(t, seen[spec.Name], "catalog row %q must be declared exactly once", spec.Name)
		seen[spec.Name] = true
	}
	assert.Len(t, seen, 18, "the catalog carries the 18 retained framework-guaranteed buckets")
}

// TestKVCatalog_DeclaredPolicies pins the architect-census policy decisions
// that enforcement derives from.
func TestKVCatalog_DeclaredPolicies(t *testing.T) {
	spec := func(name string) natsclient.BucketSpec {
		s, ok := SpecFor(name)
		require.True(t, ok, "catalog must declare %s", name)
		return s
	}

	// Owner decision 2026-07-28: ENTITY_STATES History = 1.
	assert.Equal(t, uint8(1), spec(graph.BucketEntityStates).History)
	assert.Equal(t, natsclient.ClassAuthoritative, spec(graph.BucketEntityStates).Class)
	assert.Equal(t, "graph-ingest", spec(graph.BucketEntityStates).Owner)

	// GRAPH_STATUS keeps its readiness replay depth.
	assert.Equal(t, uint8(3), spec(graph.BucketGraphStatus).History)

	// The shared runtime configuration bucket is catalogued for BOTH guarantees.
	// Owner-only by owner ruling 2026-08-31 (#1168 comment 5479005060): once
	// identity correctness state shares a bucket, letting a generic rule writer
	// onto the rest of the configuration plane is the wrong boundary, and the
	// existing owner predicate closes the whole class without a remembered-key
	// list. Its retention is STRICT — verify and refuse, never reconcile —
	// because the platform identity it holds is create-once and a silent TTL
	// strip would conceal that the identity may already have expired (ADR-104;
	// ADR-102 d7).
	sharedConfig := spec(graph.BucketSemEngineConfig + "_acme_dep")
	assert.Equal(t, natsclient.RetentionNoLifecycleStrict, sharedConfig.Retention.Kind)
	assert.Equal(t, uint8(5), sharedConfig.History)
	assert.Equal(t, natsclient.ClassOperational, sharedConfig.Class)

	// EVERY catalog bucket is owner-only, this one included: a generic update_kv
	// into platform_identity would plain-Put over create-once state that the
	// next boot ADOPTS after checking only grammar and byte budget.
	for _, s := range KVCatalog() {
		assert.Equal(t, natsclient.WriteOwnerOnly, s.Write, "%s must be owner-only", s.Name)
	}

	// Retention: no-lifecycle everywhere, with the shared configuration bucket
	// the ONE strict row (verify and refuse rather than reconcile). The
	// exception is named, not widened — a second strict row still trips this.
	for _, s := range KVCatalog() {
		if s.Name == graph.BucketSemEngineConfig {
			continue
		}
		assert.Equal(t, natsclient.RetentionNoLifecycle, s.Retention.Kind,
			"%s must declare no lifecycle retention", s.Name)
	}
}

// TestIsFrameworkOwnedBucket_ProductionView pins the production derived view:
// every catalog row is owner-only.
func TestIsFrameworkOwnedBucket_ProductionView(t *testing.T) {
	for _, name := range []string{
		graph.BucketEntityStates, graph.BucketPredicateIndex, graph.BucketIncomingIndex, graph.BucketOutgoingIndex,
		graph.BucketAliasIndex, graph.BucketNameIndex, graph.BucketSpatialIndex,
		graph.BucketTemporalIndex, graph.BucketTemporalIndexReverse,
		graph.BucketEmbeddingIndex, graph.BucketEmbeddingDedup, graph.BucketCommunityIndex,
		graph.BucketCommunitySummaries, graph.BucketAnomalyIndex,
		graph.BucketGraphIngestAppliedSeq, graph.BucketGraphStatus, graph.BucketStorageReport,
		// The configuration bucket family joined the guard set with ADR-104: a
		// generic update_kv into platform_identity forges an authority the next
		// boot adopts. Members are per deployment since #1188.
		graph.BucketSemEngineConfig + "_acme_dep",
	} {
		assert.True(t, IsFrameworkOwnedBucket(name), "%s must be framework-owned", name)
	}
	assert.False(t, IsFrameworkOwnedBucket("CONTEXT_INDEX"),
		"the retired provenance-only bucket must not remain in the generic write guard")
	assert.False(t, IsFrameworkOwnedBucket("STRUCTURAL_INDEX"),
		"the retired structural persistence bucket must not remain in the generic write guard")
	assert.False(t, IsFrameworkOwnedBucket("AGENT_LOOPS"),
		"application buckets are outside the catalog by rule")
}

// TestSpecFor_UnknownNameResolvesFalse is the F2 resolution contract: a name
// outside the catalog resolves to nothing, which callers turn into a boot
// failure naming the subject.
func TestSpecFor_UnknownNameResolvesFalse(t *testing.T) {
	_, ok := SpecFor("OUTGOING_INDEX_TYPO")
	assert.False(t, ok)
	assert.Empty(t, OwnerOf("OUTGOING_INDEX_TYPO"))

	spec, ok := SpecFor(graph.BucketOutgoingIndex)
	require.True(t, ok)
	assert.Equal(t, "graph-index", spec.Owner)
	assert.Equal(t, spec.Owner, OwnerOf(graph.BucketOutgoingIndex))
}

// TestErrorCodeBucketNotReady_MatchesIndexNotReady cross-pins the classified
// code the catalog-reader seam emits to the code graph-index emits for a
// not-sound index, so classified consumers can never see the two drift apart.
func TestErrorCodeBucketNotReady_MatchesIndexNotReady(t *testing.T) {
	assert.Equal(t, graph.ErrorCodeIndexNotReady, natsclient.ErrorCodeBucketNotReady)
}

// TestConfigBucketFamilyResolvesEveryMember pins the catalog's one name
// family (#1188): every per-deployment configuration bucket resolves to the
// strict, owner-only descriptor under its OWN name — so acquisition creates
// that member and every update_kv guard refuses it — while the bare prefix,
// the orphaned pre-#1188 bucket, is no longer framework state.
func TestConfigBucketFamilyResolvesEveryMember(t *testing.T) {
	for _, member := range []string{
		graph.BucketSemEngineConfig + "_acme_dep",
		graph.BucketSemEngineConfig + "_c360_semengine-e2e-structural",
		graph.BucketSemEngineConfig + "_a_b_c",
		graph.BucketSemEngineConfig + "_x",
	} {
		got, ok := SpecFor(member)
		require.True(t, ok, "%s must resolve to the configuration family", member)
		assert.Equal(t, member, got.Name, "the descriptor must carry the member's concrete name")
		assert.Equal(t, natsclient.RetentionNoLifecycleStrict, got.Retention.Kind)
		assert.Equal(t, uint8(5), got.History)
		assert.True(t, IsFrameworkOwnedBucket(member), "%s must stay owner-only", member)
		assert.Contains(t, OwnerOf(member), "config.Manager")
	}
	for _, outside := range []string{
		graph.BucketSemEngineConfig,
		graph.BucketSemEngineConfig + "_",
		graph.BucketSemEngineConfig + "x",
		"semengine_configuration",
	} {
		_, ok := SpecFor(outside)
		assert.False(t, ok, "%q must not resolve to the configuration family", outside)
		assert.False(t, IsFrameworkOwnedBucket(outside), "%q must not be framework-owned", outside)
	}
}

// TestEntityStatesKeepsOneRevision: the entity bucket keeps one revision per
// key, so SemEngine offers no point-in-time read of an entity's state; an
// entity's history is its input stream (#99).
//
// Requirement: graph-entity-writes/One stored revision per entity
func TestEntityStatesKeepsOneRevision(t *testing.T) {
	spec, ok := SpecFor(graph.BucketEntityStates)
	require.True(t, ok, "the catalog must declare %s", graph.BucketEntityStates)
	if spec.History != 1 {
		t.Fatalf("%s declares History %d; one stored revision per entity requires 1", graph.BucketEntityStates, spec.History)
	}
}
