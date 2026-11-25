package rule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

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
// action's core keys do not declare, a key of the rule, its entity config or
// a condition spelt with another case than declared (`"ID"` for `"id"`,
// `"Field"` for `"field"`), a key repeated in any object at any depth, and
// any data after the object. An absent
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

	// Checked before the struct decode, so that a repeated key inside an
	// action is named by the action's position in the rule.
	if err := checkNoDuplicateKeys(data, "rule"); err != nil {
		return Definition{}, decodeError(head.ID, source, err)
	}
	def := Definition{Enabled: true}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return Definition{}, decodeError(head.ID, source, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Definition{}, decodeError(head.ID, source, errors.New("data after the rule object"))
	}
	if err := checkDefinitionKeys(data); err != nil {
		return Definition{}, decodeError(head.ID, source, err)
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

// The keys Definition, EntityConfig and a condition declare, read from their
// JSON tags so that they cannot drift from the structs.
var (
	definitionKeys = jsonKeys(reflect.TypeOf(Definition{}))
	entityKeys     = jsonKeys(reflect.TypeOf(EntityConfig{}))
	conditionKeys  = jsonKeys(reflect.TypeOf(expression.ConditionExpression{}))
)

// jsonKeys returns the JSON key of every field of t that has one.
func jsonKeys(t reflect.Type) map[string]bool {
	keys := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}

// checkDefinitionKeys refuses a key of the rule, its entity config or a
// condition that is not spelt exactly as the struct declares it.
// encoding/json matches struct keys without regard to case, so "ID" or
// "Field" passes DisallowUnknownFields; a rule source that names a key two
// ways must not have one silently win. data has already decoded strictly, so
// each part has the shape the struct gives it.
func checkDefinitionKeys(data []byte) error {
	var rule map[string]json.RawMessage
	if err := json.Unmarshal(data, &rule); err != nil {
		return err
	}
	if err := checkExactKeys(rule, definitionKeys); err != nil {
		return err
	}
	if raw, ok := rule["entity"]; ok {
		if err := checkObjectKeys(raw, entityKeys); err != nil {
			return fmt.Errorf("entity: %w", err)
		}
	}
	if raw, ok := rule["conditions"]; ok {
		var conditions []json.RawMessage
		if err := json.Unmarshal(raw, &conditions); err != nil {
			return fmt.Errorf("conditions: %w", err)
		}
		for i, condition := range conditions {
			if err := checkObjectKeys(condition, conditionKeys); err != nil {
				return fmt.Errorf("conditions[%d]: %w", i, err)
			}
		}
	}
	return nil
}

// checkObjectKeys refuses a key of the JSON object raw that is not in
// declared, matched exactly. A null object has no keys.
func checkObjectKeys(raw json.RawMessage, declared map[string]bool) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	return checkExactKeys(object, declared)
}

// checkExactKeys refuses the first key of object, in sorted order, that is
// not in declared, so the error is the same on every run.
func checkExactKeys(object map[string]json.RawMessage, declared map[string]bool) error {
	names := make([]string, 0, len(object))
	for key := range object {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		if !declared[key] {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	return nil
}

func decodeError(ruleID, source string, err error) error {
	return errs.Classified(errs.ErrorInvalid, fmt.Errorf("rule %q (source %s): %w", ruleID, source, err))
}

// checkNoDuplicateKeys refuses a JSON object, at any depth in data, that
// names one key twice, naming the key and the path of the object holding it
// (root, then `.key` and `[index]` steps). encoding/json keeps the last of
// two equal keys when it decodes into a map and merges them when it decodes
// into a struct, so a repeated key would let two bodies a handler reads
// differently share one canonical form and one firing-cap key. Keys compare
// as decoded, so an escaped spelling of a key repeats it. Malformed JSON is
// left for the decode that follows to report.
func checkNoDuplicateKeys(data []byte, root string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err := checkValueKeys(decoder, root)
	var dup duplicateKeyError
	if errors.As(err, &dup) {
		return dup
	}
	return nil
}

// duplicateKeyError is a repeated key that checkNoDuplicateKeys found.
type duplicateKeyError struct {
	path, key string
}

func (e duplicateKeyError) Error() string {
	return fmt.Sprintf("%s: duplicate key %q", e.path, e.key)
}

// checkValueKeys reads one JSON value from decoder and checks every object in
// it for a repeated key. path names the value.
func checkValueKeys(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, _ := keyToken.(string)
			if seen[key] {
				return duplicateKeyError{path: path, key: key}
			}
			seen[key] = true
			if err := checkValueKeys(decoder, path+"."+key); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case json.Delim('['):
		for i := 0; decoder.More(); i++ {
			if err := checkValueKeys(decoder, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	}
	return nil
}
