// Package rule holds rule definitions, their one strict decode, and the
// action handlers that check each action of a rule.
package rule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// Definition is one rule as an author writes it. Build it with
// DecodeDefinition, which refuses keys it does not declare and fills the
// meanings of absent keys.
type Definition struct {
	ID          string                           `json:"id"`
	Type        string                           `json:"type"`
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Enabled     bool                             `json:"enabled"`
	Conditions  []expression.ConditionExpression `json:"conditions"`
	Logic       string                           `json:"logic"`
	Cooldown    string                           `json:"cooldown,omitempty"`
	Entity      EntityConfig                     `json:"entity,omitempty"`

	// Metadata is the author's notes. The engine never reads it.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	OnEnter    []Action `json:"on_enter,omitempty"`    // Fires on false→true transition
	OnExit     []Action `json:"on_exit,omitempty"`     // Fires on true→false transition
	WhileTrue  []Action `json:"while_true,omitempty"`  // Fires on every update while true
	OnRecovery []Action `json:"on_recovery,omitempty"` // Fires once on restart if still matching; falls back to OnEnter

	// RerunOnRecovery opts rules with only OnEnter/WhileTrue defined into the
	// bootstrap recovery path. When false (default), such rules must declare
	// OnRecovery explicitly to re-fire on restart. Rules that already declare
	// OnRecovery always participate regardless of this flag.
	RerunOnRecovery bool `json:"rerun_on_recovery,omitempty"`

	// MaxIterations is the value a condition reads as $state.max_iterations,
	// for retry budgets in When clauses (retry up to 3 times, then escalate).
	// It is not a limit: the engine never stops a rule because of it. An
	// action's own MaxIterations is its firing cap.
	MaxIterations int `json:"max_iterations,omitempty"`

	// FireEveryNEvents makes the rule fire its action only every Nth matching
	// event. N=0 and N=1 both mean "fire on every match". N=2 fires on the
	// 2nd, 4th, 6th matching events; N=10 fires on the 10th, 20th, 30th.
	//
	// The match check still runs on every event; only the action is gated.
	// The count is per rule, not per entity, and starts again when the rule
	// definition is replaced, so a policy change begins with a fresh rhythm.
	//
	// This is a separate dimension from the time-based cooldown; neither
	// replaces the other.
	FireEveryNEvents int `json:"fire_every_n_events,omitempty"`

	// Schedule is the cron expression for `type: "cron"` rules: POSIX
	// 5-field (`min hour dom mon dow`) or the descriptors @hourly, @daily,
	// @weekly, @monthly and @yearly.
	Schedule string `json:"schedule,omitempty"`

	// Actions is the action list for `type: "cron"` rules. Cron rules use
	// this field instead of OnEnter, OnExit and WhileTrue because they have
	// no state transitions: every scheduled tick fires every action here.
	Actions []Action `json:"actions,omitempty"`
}

// EntityConfig selects the entities a rule watches.
type EntityConfig struct {
	Pattern string `json:"pattern"` // Exact six-position EntityState ID pattern
}

// DecodeDefinition decodes one rule object from data. source names where the
// bytes came from (a rules file's path, `inline_rules[<index>]`,
// `rules.<id>@<revision>`) and is named in every error.
//
// It refuses a key that Definition, its entity config, a condition or an
// action's core keys do not declare, and any data after the object. An absent
// `enabled` means enabled, an absent `logic` means `and`, and an absent
// condition `required` means not required; an empty action list or metadata
// object decodes as an absent one. Each error is classified invalid
// and names the rule ID and source.
func DecodeDefinition(data []byte, source string) (Definition, error) {
	// The ID is read first and leniently, only so that a refusal can name the rule.
	var head struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(bytes.NewReader(data)).Decode(&head)

	def := Definition{Enabled: true}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return Definition{}, decodeError(head.ID, source, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Definition{}, decodeError(head.ID, source, errors.New("data after the rule object"))
	}
	if def.Logic == "" {
		def.Logic = "and"
	}
	// An empty list or metadata object means what an absent one means, and a
	// stored definition omits both, so both decode to nil.
	for _, list := range []*[]Action{&def.OnEnter, &def.OnExit, &def.WhileTrue, &def.OnRecovery, &def.Actions} {
		if len(*list) == 0 {
			*list = nil
		}
	}
	if len(def.Metadata) == 0 {
		def.Metadata = nil
	}
	return def, nil
}

func decodeError(ruleID, source string, err error) error {
	return errs.Classified(errs.ErrorInvalid, fmt.Errorf("rule %q (source %s): %w", ruleID, source, err))
}
