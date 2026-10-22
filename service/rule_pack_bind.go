package service

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/projection"
	rulepackcontract "github.com/c360studio/semengine/pkg/rulepack"
)

// ProjectionBinder is implemented by rule processors that use reconciled
// projection writes. Contracts are validated before any client is injected.
type ProjectionBinder interface {
	ProjectionBindings() (packID string, contracts []projection.Contract)
	PreflightProjectionMutations() error
	SetPredicateReconciler(projection.PredicateReconciler) error
}

// ProjectionBinders returns enabled components that require projection clients.
func (m *Manager) ProjectionBinders() []ProjectionBinder {
	return ComponentsImplementing[ProjectionBinder](m)
}

// ComponentsImplementing returns the admitted components that implement T, in
// the component manager's start order. The component set is fixed when the
// component manager is constructed, so a composition root can wire a
// component-facing seam once, after ConfigureFromServices. It returns nil
// before a component manager is composed, or when the one composed does not
// expose its components.
func ComponentsImplementing[T any](m *Manager) []T {
	if m == nil {
		return nil
	}
	service, exists := m.GetService(componentManagerName)
	if !exists {
		return nil
	}
	components, ok := service.(componentStartup)
	if !ok {
		return nil
	}
	var managed []*component.ManagedComponent
	// The callback cannot fail, and withComponents returns only what its
	// callback returns, so there is no error to report.
	_ = components.withComponents(func(byName map[string]*component.ManagedComponent) error {
		for _, mc := range byName {
			managed = append(managed, mc)
		}
		return nil
	})
	// Start order, never the map's: each StartOrder is a distinct position in
	// the fixed start order built at construction, so one sort gives the same
	// order on every run and ConfigureRulePackMutations visits the packs in it.
	slices.SortFunc(managed, func(a, b *component.ManagedComponent) int {
		return cmp.Compare(a.StartOrder, b.StartOrder)
	})
	var result []T
	for _, mc := range managed {
		if match, ok := mc.Component.(T); ok {
			result = append(result, match)
		}
	}
	return result
}

type rulePackMutationPlan struct {
	binder    ProjectionBinder
	packID    string
	contracts []projection.Contract
}

// ConfigureRulePackMutations validates every enabled pack before injecting one
// canonical mutation client per contract-bearing processor. A composition root
// calls it after ConfigureFromServices and before StartAll; without the call no
// binder is handed a reconciler.
func ConfigureRulePackMutations(manager *Manager) error {
	if manager == nil {
		return fmt.Errorf("rule-pack mutation composition requires a service manager")
	}
	manager.mu.RLock()
	client := manager.natsClient
	manager.mu.RUnlock()
	plans, err := preflightRulePackMutations(manager.ProjectionBinders(), client)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		if len(plan.contracts) == 0 {
			continue
		}
		reconciler, err := projection.NewMutationClient(projection.MutationClientConfig{
			NATS: client, Contracts: plan.contracts,
		})
		if err != nil {
			return fmt.Errorf("build mutation client for rule pack %q: %w", plan.packID, err)
		}
		if err := plan.binder.SetPredicateReconciler(reconciler); err != nil {
			return fmt.Errorf("inject predicate reconciler for rule pack %q: %w", plan.packID, err)
		}
	}
	return nil
}

// preflightRulePackMutations runs every binder's preflight before reading any
// binding, then checks every pack, so one refusal leaves every binder without a
// reconciler.
func preflightRulePackMutations(binders []ProjectionBinder, client *natsclient.Client) ([]rulePackMutationPlan, error) {
	plans := make([]rulePackMutationPlan, 0, len(binders))
	seen := make(map[string]struct{}, len(binders))
	for index, binder := range binders {
		if err := binder.PreflightProjectionMutations(); err != nil {
			return nil, fmt.Errorf("preflight enabled rule processor %d: %w", index, err)
		}
	}
	for _, binder := range binders {
		packID, contracts := binder.ProjectionBindings()
		if err := rulepackcontract.ValidateID(packID); err != nil {
			return nil, fmt.Errorf("enabled rule processor has invalid pack_id: %w", err)
		}
		if _, duplicate := seen[packID]; duplicate {
			return nil, fmt.Errorf("duplicate enabled rule pack_id %q in one composition", packID)
		}
		seen[packID] = struct{}{}
		copies := make([]projection.Contract, len(contracts))
		for i, contract := range contracts {
			copies[i] = contract.Clone()
		}
		if len(copies) > 0 {
			if client == nil {
				return nil, fmt.Errorf("rule pack %q mutation client requires NATS", packID)
			}
			if err := projection.ValidateContracts(copies); err != nil {
				return nil, fmt.Errorf("validate contracts for rule pack %q: %w", packID, err)
			}
		}
		plans = append(plans, rulePackMutationPlan{binder: binder, packID: packID, contracts: copies})
	}
	return plans, nil
}
