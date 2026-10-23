package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
)

// Rule-pack binding (design D6; the pin's rule_pack_bind.go had no test of its own). A composition
// root calls ConfigureRulePackMutations after ConfigureFromServices and before StartAll: it finds
// the admitted components that implement ProjectionBinder, checks every pack before it injects
// anything, and hands each pack that declares contracts one mutation client of its own. The
// binders here are doubles that record what they were asked and what they were handed.
//
// These tests do not call t.Parallel: rulePackBindContract registers predicates in the
// process-wide vocabulary and restores it when the test ends.

// errRulePackBindPreflight is what a failing binder double's preflight returns.
var errRulePackBindPreflight = errors.New("rule pack preflight failed")

// rulePackBinder is a ProjectionBinder double: a lifecycle component declaring packID and
// contracts whose preflight fails with preflightErr when it is set. It counts the preflights and
// binding reads it was asked for and keeps every reconciler it was handed. ConfigureRulePackMutations
// runs on the caller's goroutine, so the fields need no lock.
type rulePackBinder struct {
	baseDiscoverable
	packID       string
	contracts    []projection.Contract
	preflightErr error

	preflights  int
	bindings    int
	reconcilers []projection.PredicateReconciler
}

var _ ProjectionBinder = (*rulePackBinder)(nil)

func (*rulePackBinder) Initialize() error           { return nil }
func (*rulePackBinder) Start(context.Context) error { return nil }
func (*rulePackBinder) Stop(context.Context) error  { return nil }

func (b *rulePackBinder) ProjectionBindings() (string, []projection.Contract) {
	b.bindings++
	return b.packID, b.contracts
}

func (b *rulePackBinder) PreflightProjectionMutations() error {
	b.preflights++
	return b.preflightErr
}

func (b *rulePackBinder) SetPredicateReconciler(reconciler projection.PredicateReconciler) error {
	b.reconcilers = append(b.reconcilers, reconciler)
	return nil
}

// rulePackBindContract returns a valid contract named name, with one reconcile group "state"
// over a predicate registered in the vocabulary for this test only.
func rulePackBindContract(t *testing.T, name string) projection.Contract {
	t.Helper()
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register("test.rulepack.value")
	return projection.Contract{
		Name:          name,
		EntityPattern: "*.*.test.system.widget.*",
		Groups: []projection.PredicateGroup{
			{Name: "state", Mode: projection.ModeReconcile, Predicates: []string{"test.rulepack.value"}},
		},
	}
}

// newRulePackBindManager composes components as a composition root does: ConfigureFromServices
// builds the component manager from its registered constructor, NewComponentManager, over a
// component registry holding one factory per component, and keeps the dependencies' NATS client.
// The client is not connected; building a mutation client does not reach a broker.
func newRulePackBindManager(t *testing.T, components ...testComponent) *Manager {
	t.Helper()
	registry := component.NewRegistry()
	names := make([]string, 0, len(components))
	for _, c := range components {
		admitTestRegistryComponent(t, registry, c.name, c.instance)
		names = append(names, c.name)
	}
	services := NewServiceRegistry()
	require.NoError(t, services.Register(componentManagerName, NewComponentManager))
	manager := NewServiceManager(services)
	// metrics is disabled: its default port is fixed.
	require.NoError(t, manager.ConfigureFromServices(types.ServiceConfigs{
		"metrics": {Enabled: false, Config: json.RawMessage(`{}`)},
	}, testComponentDependencies(t, registry, names...)))
	return manager
}

// newRulePackBindManagerWithoutNATS composes the component manager as a prebuilt instance and never
// calls ConfigureFromServices, so the service manager holds no NATS client.
func newRulePackBindManagerWithoutNATS(t *testing.T, components ...testComponent) *Manager {
	t.Helper()
	manager := NewServiceManager(NewServiceRegistry())
	require.NoError(t, manager.RegisterInstance(componentManagerName, newTestComponentManager(t, components...)))
	return manager
}

// requireNoRulePackInjection fails the test if any of binders was handed a reconciler.
func requireNoRulePackInjection(t *testing.T, binders ...*rulePackBinder) {
	t.Helper()
	for _, binder := range binders {
		require.Empty(t, binder.reconcilers, "pack %q was handed a reconciler", binder.packID)
	}
}

// TestConfigureRulePackMutationsInjectsOneClientPerPack: each pack that declares contracts is
// handed exactly one mutation client, built over that pack's contracts and no other's. A component
// that is not a binder is passed over.
func TestConfigureRulePackMutationsInjectsOneClientPerPack(t *testing.T) {
	alpha := &rulePackBinder{packID: "alpha", contracts: []projection.Contract{rulePackBindContract(t, "alpha-contract")}}
	beta := &rulePackBinder{packID: "beta", contracts: []projection.Contract{rulePackBindContract(t, "beta-contract")}}
	manager := newRulePackBindManager(t,
		testComponent{name: "alpha-rules", instance: alpha},
		testComponent{name: "beta-rules", instance: beta},
		testComponent{name: "plain", instance: &plainComponent{}},
	)

	require.NoError(t, ConfigureRulePackMutations(manager))

	for _, tc := range []struct {
		binder       *rulePackBinder
		own, foreign string
	}{
		{binder: alpha, own: "alpha-contract", foreign: "beta-contract"},
		{binder: beta, own: "beta-contract", foreign: "alpha-contract"},
	} {
		require.Equal(t, 1, tc.binder.preflights, "pack %q preflights", tc.binder.packID)
		require.Len(t, tc.binder.reconcilers, 1, "pack %q reconcilers", tc.binder.packID)
		client, ok := tc.binder.reconcilers[0].(*projection.MutationClient)
		require.True(t, ok, "pack %q was handed %T, want *projection.MutationClient",
			tc.binder.packID, tc.binder.reconcilers[0])
		require.NotNil(t, client, "pack %q was handed a nil client", tc.binder.packID)

		// The client knows this pack's contract and not the other pack's. Both requests name an
		// entity outside every pattern, so the client refuses them itself and sends nothing; only
		// the foreign one is refused as an unknown contract.
		_, err := client.Reconcile(t.Context(), projection.ReconcileMutation{
			Contract: tc.foreign, Group: "state", EntityID: "not-an-entity",
		})
		require.ErrorContains(t, err, `unknown projection contract "`+tc.foreign+`"`)
		_, err = client.Reconcile(t.Context(), projection.ReconcileMutation{
			Contract: tc.own, Group: "state", EntityID: "not-an-entity",
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "unknown projection contract",
			"pack %q's client does not hold its own contract", tc.binder.packID)
	}
}

// configureRulePackRefusal composes a healthy pack beside failing, runs ConfigureRulePackMutations,
// requires it to fail with neither pack handed a reconciler, and returns the healthy pack and the
// error. The instance names put the healthy pack first in name order.
func configureRulePackRefusal(t *testing.T, failing *rulePackBinder) (*rulePackBinder, error) {
	t.Helper()
	healthy := &rulePackBinder{packID: "healthy", contracts: []projection.Contract{rulePackBindContract(t, "healthy-contract")}}
	manager := newRulePackBindManager(t,
		testComponent{name: "a-healthy-rules", instance: healthy},
		testComponent{name: "z-failing-rules", instance: failing},
	)
	err := ConfigureRulePackMutations(manager)
	require.Error(t, err)
	requireNoRulePackInjection(t, healthy, failing)
	return healthy, err
}

// TestConfigureRulePackMutationsRefusesBeforeAnyInjection: every pack is checked before any is
// handed a client, so one failing pack refuses the composition and leaves every pack, the healthy
// one too, without a reconciler. The preflights run before anything else: when one fails, no pack's
// bindings were read.
func TestConfigureRulePackMutationsRefusesBeforeAnyInjection(t *testing.T) {
	t.Run("NilManager", func(t *testing.T) {
		require.Error(t, ConfigureRulePackMutations(nil))
	})

	t.Run("PreflightFails", func(t *testing.T) {
		failing := &rulePackBinder{
			packID: "failing", contracts: []projection.Contract{rulePackBindContract(t, "failing-contract")},
			preflightErr: errRulePackBindPreflight,
		}
		healthy, err := configureRulePackRefusal(t, failing)
		require.ErrorIs(t, err, errRulePackBindPreflight)
		require.Zero(t, healthy.bindings, "a pack's bindings were read before every preflight ran")
		require.Zero(t, failing.bindings, "a pack's bindings were read before every preflight ran")
	})

	t.Run("InvalidPackID", func(t *testing.T) {
		// A pack without contracts is still held to the pack-ID grammar.
		_, err := configureRulePackRefusal(t, &rulePackBinder{packID: "bad.pack"})
		require.ErrorContains(t, err, "pack_id")
		require.ErrorContains(t, err, `"bad.pack"`)
	})

	t.Run("InvalidContract", func(t *testing.T) {
		_, err := configureRulePackRefusal(t, &rulePackBinder{
			packID: "failing",
			contracts: []projection.Contract{{
				Name: "no-predicates", EntityPattern: "*.*.test.system.widget.*",
			}},
		})
		require.ErrorIs(t, err, projection.ErrInvalidContract)
		require.ErrorContains(t, err, `"failing"`)
	})
}

// TestConfigureRulePackMutationsRefusesADuplicatePackID: two packs in one composition may not share
// a pack ID; the refusal names it and neither pack is handed a client.
func TestConfigureRulePackMutationsRefusesADuplicatePackID(t *testing.T) {
	first := &rulePackBinder{packID: "shared", contracts: []projection.Contract{rulePackBindContract(t, "first-contract")}}
	second := &rulePackBinder{packID: "shared", contracts: []projection.Contract{rulePackBindContract(t, "second-contract")}}
	manager := newRulePackBindManager(t,
		testComponent{name: "first-rules", instance: first},
		testComponent{name: "second-rules", instance: second},
	)

	err := ConfigureRulePackMutations(manager)
	require.ErrorContains(t, err, "duplicate")
	require.ErrorContains(t, err, `"shared"`)
	requireNoRulePackInjection(t, first, second)
}

// TestConfigureRulePackMutationsRequiresNATSForContracts: a pack with contracts needs the service
// manager's NATS client and is refused, naming the pack, by the check before injection (the
// message is the check's, not the mutation client's "NATS client is required"). A pack without
// contracts needs none.
func TestConfigureRulePackMutationsRequiresNATSForContracts(t *testing.T) {
	t.Run("PackWithContracts", func(t *testing.T) {
		alpha := &rulePackBinder{packID: "alpha", contracts: []projection.Contract{rulePackBindContract(t, "alpha-contract")}}
		manager := newRulePackBindManagerWithoutNATS(t, testComponent{name: "alpha-rules", instance: alpha})

		err := ConfigureRulePackMutations(manager)
		require.ErrorContains(t, err, "requires NATS")
		require.ErrorContains(t, err, `"alpha"`)
		requireNoRulePackInjection(t, alpha)
	})

	t.Run("PackWithoutContracts", func(t *testing.T) {
		bare := &rulePackBinder{packID: "bare"}
		manager := newRulePackBindManagerWithoutNATS(t, testComponent{name: "bare-rules", instance: bare})

		require.NoError(t, ConfigureRulePackMutations(manager))
		require.Equal(t, 1, bare.preflights)
		requireNoRulePackInjection(t, bare)
	})
}

// TestConfigureRulePackMutationsSkipsAPackWithoutContracts: a pack that declares no contracts is
// preflighted and its pack ID read, but it is handed no client; the pack beside it that declares
// contracts still gets its own.
func TestConfigureRulePackMutationsSkipsAPackWithoutContracts(t *testing.T) {
	alpha := &rulePackBinder{packID: "alpha", contracts: []projection.Contract{rulePackBindContract(t, "alpha-contract")}}
	bare := &rulePackBinder{packID: "bare"}
	manager := newRulePackBindManager(t,
		testComponent{name: "alpha-rules", instance: alpha},
		testComponent{name: "bare-rules", instance: bare},
	)

	require.NoError(t, ConfigureRulePackMutations(manager))
	require.Equal(t, 1, bare.preflights)
	require.Equal(t, 1, bare.bindings)
	requireNoRulePackInjection(t, bare)
	require.Len(t, alpha.reconcilers, 1)
}
