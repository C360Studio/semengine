package service

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/natsclient/storagereport"
)

// The storage observability service's health rendering (design D5, "Health"; D7, U-d). Health
// returns the base service's status with the storage picture appended to its message and nothing
// else changed, so pressure, an over-committed tier, an unread report or a failed collection make
// nothing unhealthy, degraded or not ready. The rendering is the plain function
// describeStorageHealth, which these tests call with the base status, snapshot and inventory each
// one builds. Carried from the pin: storage_observability_test.go:249-283 and :346-483, and
// storage_observability_tier_metrics_test.go:151-222; the pin's storageHealthSurface and its
// staticSnapshot reader become the snapshot and inventory values passed in (O4).

// TestStorageObservabilityHealth_CriticalPressureFailsNoGate is the report-only guarantee stated
// where it would break first. Readiness reads Status() and IsHealthy(); /health aggregates
// Health(). None of them may move because a resource is under pressure.
func TestStorageObservabilityHealth_CriticalPressureFailsNoGate(t *testing.T) {
	snapshot := storagereport.Snapshot{
		Synced: true,
		Resources: []storagereport.ResourceReport{
			withPressure(resourceRow("DOOMED", storagereport.TierFile), storagereport.PressureCritical),
			withPressure(resourceRow("FINE", storagereport.TierFile), storagereport.PressureNormal),
		},
		PressureCounts: map[storagereport.PressureState]int{
			storagereport.PressureCritical: 1,
			storagereport.PressureNormal:   1,
		},
		WorstPressure: storagereport.PressureCritical,
	}

	base := health.NewHealthy(StorageObservabilityServiceName, "Service operating normally")
	status := describeStorageHealth(base, snapshot, storagereport.StorageInventory{})

	assert.True(t, status.Healthy, "pressure is report-only; it cannot make a service unhealthy")
	assert.Equal(t, "healthy", status.Status)
	assert.False(t, status.IsDegraded())
	assert.False(t, status.IsUnhealthy())

	// And the same status aggregated the way /health aggregates it.
	system := health.Aggregate("system", []health.Status{status})
	assert.True(t, system.Healthy, "a critical resource must not degrade system health")

	assert.Contains(t, status.Message, "critical=1", "the state is still visible in health STATUS")
	assert.Contains(t, status.Message, "report-only")
}

// TestStorageObservabilityHealth_NeverCarriesANonHealthySubStatus keeps a pressure verdict from
// arriving as a sub-status: the base has none, so the rendered status has none either.
func TestStorageObservabilityHealth_NeverCarriesANonHealthySubStatus(t *testing.T) {
	snapshot := storagereport.Snapshot{
		Synced: true,
		Resources: []storagereport.ResourceReport{
			withPressure(resourceRow("DOOMED", storagereport.TierFile), storagereport.PressureCritical),
		},
		PressureCounts: map[storagereport.PressureState]int{storagereport.PressureCritical: 1},
		WorstPressure:  storagereport.PressureCritical,
	}

	status := describeStorageHealth(health.NewHealthy(StorageObservabilityServiceName, "ok"), snapshot,
		storagereport.StorageInventory{})
	assert.Empty(t, status.SubStatuses, "no pressure verdict may be expressed as a health verdict")
}

// TestStorageObservabilityHealth_NamesUnevaluatedResources keeps unbounded and unreadable
// resources from being the ones nobody sees: they carry no pressure state at all, so a summary
// that only counted states would omit them.
func TestStorageObservabilityHealth_NamesUnevaluatedResources(t *testing.T) {
	snapshot := storagereport.Snapshot{
		Synced: true,
		Resources: []storagereport.ResourceReport{
			unboundedRow("FREE", storagereport.TierFile),
			withPressure(resourceRow("FINE", storagereport.TierFile), storagereport.PressureNormal),
		},
		PressureCounts: map[storagereport.PressureState]int{storagereport.PressureNormal: 1},
		NotEvaluated:   1,
		WorstPressure:  storagereport.PressureNormal,
	}

	status := describeStorageHealth(health.NewHealthy(StorageObservabilityServiceName, "ok"), snapshot,
		storagereport.StorageInventory{})
	assert.Contains(t, status.Message, "not-evaluated=1")
	assert.Contains(t, status.Message, "FREE not-evaluated (unbounded)")
	assert.True(t, status.Healthy)
}

// TestStorageObservabilityHealth_ReportsWhenNothingHasBeenReadYet keeps an unread report from
// looking like an empty account.
func TestStorageObservabilityHealth_ReportsWhenNothingHasBeenReadYet(t *testing.T) {
	status := describeStorageHealth(health.NewHealthy(StorageObservabilityServiceName, "ok"),
		storagereport.Snapshot{}, storagereport.StorageInventory{})
	assert.True(t, status.Healthy, "a report nobody has read yet is not a failure")
	assert.Contains(t, strings.ToLower(status.Message), "not been read")
}

// TestStorageObservabilityHealth_PreservesTheBaseVerdictsReason pins the one axis the storage
// picture must never overwrite. BaseService carries the reason for a non-healthy lifecycle verdict
// only in Message ("Service is stopped"), so replacing it would leave an operator reading
// `status: unhealthy` next to a pressure line and no explanation.
func TestStorageObservabilityHealth_PreservesTheBaseVerdictsReason(t *testing.T) {
	base := health.NewUnhealthy(StorageObservabilityServiceName, "Service is stopped")
	status := describeStorageHealth(base, storagereport.Snapshot{Synced: true}, storagereport.StorageInventory{})

	assert.Contains(t, status.Message, "Service is stopped",
		"the reason for the verdict must survive the storage picture")
	assert.Contains(t, strings.ToLower(status.Message), "storage pressure",
		"the storage picture is still appended, not dropped")
	assert.False(t, status.Healthy, "the rendering must not touch the status axis")
}

// TestStorageObservabilityHealth_ReportsCollectionFreshness puts staleness on the answer rather
// than in a decision to withhold it. The message rounds the age to the second, so the assertion
// holds unless the call takes longer than half a second after collectedAt is taken.
func TestStorageObservabilityHealth_ReportsCollectionFreshness(t *testing.T) {
	collectedAt := time.Now().Add(-90 * time.Second)
	inventory := storagereport.StorageInventory{ProducedBy: "collector-1", CollectedAt: collectedAt}

	status := describeStorageHealth(health.NewHealthy(StorageObservabilityServiceName, "ok"),
		storagereport.Snapshot{Synced: true, PressureCounts: map[storagereport.PressureState]int{}}, inventory)
	assert.Contains(t, status.Message, "collected 1m30s ago by collector-1")
}

// TestStorageObservabilityHealth_FailedCollectionIsReportedNotGated is the other half of the
// monitoring-safety rule: a collector that cannot read the account says so in its message and
// still fails nothing.
func TestStorageObservabilityHealth_FailedCollectionIsReportedNotGated(t *testing.T) {
	failedAt := time.Now()
	inventory := storagereport.StorageInventory{
		ProducedBy:  "collector-1",
		Stale:       true,
		StaleSince:  &failedAt,
		StaleReason: "jetstream unavailable",
	}

	status := describeStorageHealth(health.NewHealthy(StorageObservabilityServiceName, "ok"),
		storagereport.Snapshot{Synced: true, PressureCounts: map[storagereport.PressureState]int{}}, inventory)
	assert.True(t, status.Healthy, "an unreadable account is a finding, not a verdict")
	assert.Equal(t, "healthy", status.Status)
	assert.Contains(t, status.Message, "last collection did not succeed")
	assert.Contains(t, status.Message, "jetstream unavailable")
	assert.Contains(t, status.Message, "serving last known result")
}

// TestStorageObservabilityHealth_ReportsAnOverCommittedTier surfaces the account finding where an
// operator already looks.
func TestStorageObservabilityHealth_ReportsAnOverCommittedTier(t *testing.T) {
	snapshot := storagereport.Snapshot{
		Synced:       true,
		AccountKnown: true,
		Account: storagereport.AccountReport{Tiers: []storagereport.TierComparison{
			{Tier: storagereport.TierFile, State: storagereport.OvercommitmentOver, DeclaredBytes: 100},
			{Tier: storagereport.TierMemory, State: storagereport.OvercommitmentNotApplicable,
				Unavailable: storagereport.OvercommitmentUnavailableUnboundedLimit},
		}},
		PressureCounts: map[storagereport.PressureState]int{},
	}

	status := describeStorageHealth(health.NewHealthy(StorageObservabilityServiceName, "ok"), snapshot,
		storagereport.StorageInventory{})
	assert.True(t, status.Healthy, "over-commitment is a finding, not a gate")
	assert.Contains(t, status.Message, "file over-committed")
}

// TestInheritedPressure_HealthMessageNamesTheCeilingItCameFrom qualifies a state inherited from
// the account tier, and only that state: the two findings have different fixes.
func TestInheritedPressure_HealthMessageNamesTheCeilingItCameFrom(t *testing.T) {
	snapshot := storagereport.Snapshot{
		Synced: true,
		Resources: []storagereport.ResourceReport{
			inheritedPressureRow("CAMPAIGN_LEDGER", storagereport.TierFile, storagereport.PressureCritical),
			withPressure(resourceRow("EVENTS", storagereport.TierFile), storagereport.PressureHigh),
		},
		PressureCounts: map[storagereport.PressureState]int{
			storagereport.PressureCritical: 1,
			storagereport.PressureHigh:     1,
		},
		WorstPressure: storagereport.PressureCritical,
	}

	status := describeStorageHealth(
		health.NewHealthy(StorageObservabilityServiceName, "Service operating normally"), snapshot,
		storagereport.StorageInventory{})

	assert.Contains(t, status.Message, "CAMPAIGN_LEDGER critical (time-to-threshold) vs file account tier")
	assert.Contains(t, status.Message, "EVENTS high (headroom)")
	assert.NotContains(t, status.Message, "EVENTS high (headroom) vs",
		"a resource evaluated against its own bound must not be qualified as an account-tier finding")
	assert.True(t, status.Healthy, "pressure remains report-only however it was derived")
}

// TestInheritedPressure_DoesNotEvictOwnBoundFindingsFromTheHealthMessage is the amplification
// guard. Every framework KV bucket is unbounded on bytes, so one tier crossing a band makes many
// resources inherit the same state. The message names at most five; sorted on severity and name
// alone, the inherited rows would push the one resource whose bound an operator can raise into
// "and N more".
func TestInheritedPressure_DoesNotEvictOwnBoundFindingsFromTheHealthMessage(t *testing.T) {
	rows := []storagereport.ResourceReport{
		// Alphabetically ahead of the own-bound row, which is what makes name order the wrong
		// tiebreak.
		inheritedPressureRow("KV_AGENT_LOOPS", storagereport.TierFile, storagereport.PressureCritical),
		inheritedPressureRow("KV_ALIAS_INDEX", storagereport.TierFile, storagereport.PressureCritical),
		inheritedPressureRow("KV_COMMUNITY_INDEX", storagereport.TierFile, storagereport.PressureCritical),
		inheritedPressureRow("KV_ENTITY_STATES", storagereport.TierFile, storagereport.PressureCritical),
		inheritedPressureRow("KV_GRAPH_STATUS", storagereport.TierFile, storagereport.PressureCritical),
		inheritedPressureRow("KV_STORAGE_REPORT", storagereport.TierFile, storagereport.PressureCritical),
		withPressure(resourceRow("ZEBRA_EVENTS", storagereport.TierFile), storagereport.PressureCritical),
	}
	snapshot := storagereport.Snapshot{
		Synced:         true,
		Resources:      rows,
		PressureCounts: map[storagereport.PressureState]int{storagereport.PressureCritical: len(rows)},
		WorstPressure:  storagereport.PressureCritical,
	}

	message := describeStorageHealth(
		health.NewHealthy(StorageObservabilityServiceName, "Service operating normally"), snapshot,
		storagereport.StorageInventory{}).Message

	assert.Contains(t, message, "ZEBRA_EVENTS critical",
		"the finding with a bound an operator can raise must survive the five-slot budget")
	zebra := strings.Index(message, "ZEBRA_EVENTS")
	inherited := strings.Index(message, "KV_AGENT_LOOPS")
	require.Positive(t, zebra)
	require.Positive(t, inherited)
	assert.Less(t, zebra, inherited,
		"own-bound findings must precede states inherited from a shared tier ceiling")

	// The inherited rows are not hidden: they are just not first, and each still says which
	// ceiling it came from.
	assert.Contains(t, message, "vs file account tier")
	assert.Contains(t, message, "and 2 more")
}

// TestPropStorageHealthKeepsTheBaseVerdict checks the stated law of design D5: the rendering
// never moves the base verdict. The oracle is the base status itself: Healthy, Status and the
// sub-statuses equal the base's, and the message begins with the base message when that is not
// empty. The generator (design D7, "Generated checks") draws the base from all three constructors
// with an empty or any message and zero to three sub-statuses; a snapshot synced or not whose rows
// take every pressure state, evaluated or not, against their own bound or the account tier; an
// account known or not whose tiers take every over-commitment state; and an inventory stale or
// not, collected or not. Each case runs every assertion; the empty-message branch runs whenever
// the drawn message is "", which the generator draws by construction. Seed and completed checks
// are recorded in the notes when the test runs (task 2.1, U-d).
func TestPropStorageHealthKeepsTheBaseVerdict(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		base := drawBaseStatus(rt, "base")
		baseSubStatuses := slices.Clone(base.SubStatuses)
		snapshot := drawStorageSnapshot(rt)
		inventory := drawStorageInventory(rt)

		status := describeStorageHealth(base, snapshot, inventory)

		if status.Healthy != base.Healthy {
			rt.Fatalf("Healthy = %v, base %v", status.Healthy, base.Healthy)
		}
		if status.Status != base.Status {
			rt.Fatalf("Status = %q, base %q", status.Status, base.Status)
		}
		if !assert.ObjectsAreEqual(baseSubStatuses, status.SubStatuses) {
			rt.Fatalf("SubStatuses = %+v, base %+v", status.SubStatuses, baseSubStatuses)
		}
		if base.Message != "" && !strings.HasPrefix(status.Message, base.Message) {
			rt.Fatalf("Message %q does not begin with the base message %q", status.Message, base.Message)
		}
	})
}

// drawBaseStatus draws a status from one of the three health constructors, with an empty or any
// message and, for the top-level base, zero to three sub-statuses.
func drawBaseStatus(rt *rapid.T, label string) health.Status {
	verdict := rapid.SampledFrom([]string{"healthy", "degraded", "unhealthy"}).Draw(rt, label+"-verdict")
	message := rapid.OneOf(rapid.Just(""), rapid.String()).Draw(rt, label+"-message")
	var status health.Status
	switch verdict {
	case "healthy":
		status = health.NewHealthy(StorageObservabilityServiceName, message)
	case "degraded":
		status = health.NewDegraded(StorageObservabilityServiceName, message)
	default:
		status = health.NewUnhealthy(StorageObservabilityServiceName, message)
	}
	if label != "base" {
		return status
	}
	for i := range rapid.IntRange(0, 3).Draw(rt, "sub-statuses") {
		status.SubStatuses = append(status.SubStatuses, drawBaseStatus(rt, "sub-"+strconv.Itoa(i)))
	}
	return status
}

var (
	propPressureStates = []storagereport.PressureState{
		storagereport.PressureNormal, storagereport.PressureWarning,
		storagereport.PressureHigh, storagereport.PressureCritical,
	}
	propTiers = []storagereport.StorageTier{
		storagereport.TierFile, storagereport.TierMemory, storagereport.TierUnknown,
	}
)

// drawStorageSnapshot draws a snapshot as the consumer would hand it over: empty when not synced,
// otherwise rows with counts that tally them.
func drawStorageSnapshot(rt *rapid.T) storagereport.Snapshot {
	if !rapid.Bool().Draw(rt, "synced") {
		return storagereport.Snapshot{}
	}
	snapshot := storagereport.Snapshot{
		Synced:         true,
		PressureCounts: map[storagereport.PressureState]int{},
	}
	for range rapid.IntRange(0, 8).Draw(rt, "rows") {
		name := rapid.StringMatching(`[A-Z][A-Z_]{0,11}`).Draw(rt, "name")
		tier := rapid.SampledFrom(propTiers).Draw(rt, "tier")
		var row storagereport.ResourceReport
		switch rapid.IntRange(0, 3).Draw(rt, "row-shape") {
		case 0:
			row = withPressure(resourceRow(name, tier), rapid.SampledFrom(propPressureStates).Draw(rt, "own-state"))
		case 1:
			row = inheritedPressureRow(name, tier, rapid.SampledFrom(propPressureStates).Draw(rt, "inherited-state"))
		case 2:
			row = unboundedRow(name, tier)
		default:
			row = unknownRow(name)
		}
		snapshot.Resources = append(snapshot.Resources, row)
		if row.Pressure.Evaluated {
			snapshot.PressureCounts[row.Pressure.State]++
		} else {
			snapshot.NotEvaluated++
		}
	}
	if rapid.Bool().Draw(rt, "account-known") {
		snapshot.AccountKnown = true
		for range rapid.IntRange(0, 3).Draw(rt, "tiers") {
			snapshot.Account.Tiers = append(snapshot.Account.Tiers, storagereport.TierComparison{
				Tier: rapid.SampledFrom(propTiers).Draw(rt, "account-tier"),
				State: rapid.SampledFrom([]storagereport.OvercommitmentState{
					storagereport.OvercommitmentWithin, storagereport.OvercommitmentOver,
					storagereport.OvercommitmentNotApplicable,
				}).Draw(rt, "overcommitment"),
				Unavailable: rapid.SampledFrom([]string{
					"", storagereport.OvercommitmentUnavailableUnboundedLimit,
					storagereport.OvercommitmentUnavailableUnknownLimit,
				}).Draw(rt, "overcommitment-unavailable"),
				DeclaredBytes: rapid.Int64Min(0).Draw(rt, "declared-bytes"),
			})
		}
	}
	return snapshot
}

// drawStorageInventory draws an inventory stale or not, collected or not.
func drawStorageInventory(rt *rapid.T) storagereport.StorageInventory {
	inventory := storagereport.StorageInventory{
		ProducedBy: rapid.StringMatching(`[a-z][a-z0-9-]{0,15}`).Draw(rt, "produced-by"),
	}
	if rapid.Bool().Draw(rt, "collected") {
		age := time.Duration(rapid.Int64Range(0, int64(24*time.Hour)).Draw(rt, "age"))
		inventory.CollectedAt = time.Now().Add(-age)
	}
	if rapid.Bool().Draw(rt, "stale") {
		failedAt := time.Now()
		inventory.Stale = true
		inventory.StaleSince = &failedAt
		inventory.StaleReason = rapid.String().Draw(rt, "stale-reason")
	}
	return inventory
}
