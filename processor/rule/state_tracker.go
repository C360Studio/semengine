package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/natsclient"
)

// transition is the change in a rule's match between two evaluations, stored as
// the record's last_transition string.
type transition string

const (
	// transitionNone means the match did not change.
	transitionNone transition = ""
	// transitionEntered means the rule went from not matching to matching.
	transitionEntered transition = "entered"
	// transitionExited means the rule went from matching to not matching.
	transitionExited transition = "exited"
)

// MatchState is one rule's stored state for one entity key: whether the rule
// matched at its last evaluation, and what it saw then.
type MatchState struct {
	// RuleID is the rule's ID, one key token.
	RuleID string `json:"rule_id"`
	// EntityKey is what the rule was evaluated for: an entity ID, or a message ID
	// on the message path.
	EntityKey string `json:"entity_key"`
	// IsMatching is whether the rule matched at its last evaluation.
	IsMatching bool `json:"is_matching"`
	// LastTransition is the last transition, "entered" or "exited", kept through
	// later evaluations without one.
	LastTransition string `json:"last_transition"`
	// TransitionAt is when LastTransition happened.
	TransitionAt time.Time `json:"transition_at,omitempty"`
	// SourceRevision is the revision of the entity or message that was evaluated.
	SourceRevision uint64 `json:"source_revision"`
	// LastChecked is when the rule was last evaluated, set by every evaluation.
	LastChecked time.Time `json:"last_checked"`
	// Iteration counts the rule's evaluations of this key.
	Iteration int `json:"iteration"`
	// MaxIterations is a copy of the rule's max_iterations at that evaluation, so
	// that $state.max_iterations can read it. It is never read back and is not a
	// limit.
	MaxIterations int `json:"max_iterations,omitempty"`
	// FieldValues holds the watched field values seen at that evaluation.
	FieldValues map[string]string `json:"field_values,omitempty"`
}

// StateTracker stores MatchState records in a KV bucket under the key
// `<rule ID>.<entity key>`.
type StateTracker struct {
	kv *natsclient.KVStore
}

// ErrStateNotFound is returned by Get when no record is stored for the pair.
var ErrStateNotFound = errors.New("rule state not found")

// NewStateTracker returns a StateTracker over kv.
func NewStateTracker(kv *natsclient.KVStore) *StateTracker { return &StateTracker{kv: kv} }

// detectTransition gives the transition between the previous and the current match.
func detectTransition(wasMatching, nowMatching bool) transition {
	switch {
	case !wasMatching && nowMatching:
		return transitionEntered
	case wasMatching && !nowMatching:
		return transitionExited
	default:
		return transitionNone
	}
}

// stateKey builds a record's key, refusing a rule ID that is not one literal key
// token and a key that is not a literal key, so no wildcard reaches the bucket.
// The entity key need not be an entity ID: the message path keys by message ID.
func stateKey(ruleID, entityKey string) (string, error) {
	if err := natsclient.ValidateKVLiteralToken(ruleID); err != nil {
		return "", fmt.Errorf("rule state rule ID %q: %w", ruleID, err)
	}
	key := ruleID + "." + entityKey
	if err := natsclient.ValidateKVLiteralKey(key); err != nil {
		return "", fmt.Errorf("rule state key %q: %w", key, err)
	}
	return key, nil
}

// Get reads the record for ruleID and entityKey, or returns ErrStateNotFound.
func (st *StateTracker) Get(ctx context.Context, ruleID, entityKey string) (MatchState, error) {
	key, err := stateKey(ruleID, entityKey)
	if err != nil {
		return MatchState{}, err
	}
	entry, err := st.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, natsclient.ErrKVKeyNotFound) {
			return MatchState{}, ErrStateNotFound
		}
		return MatchState{}, fmt.Errorf("get rule state %s: %w", key, err)
	}
	var state MatchState
	if err := json.Unmarshal(entry.Value, &state); err != nil {
		return MatchState{}, fmt.Errorf("decode rule state %s: %w", key, err)
	}
	return state, nil
}

// Set writes state under its RuleID and EntityKey, replacing any record there.
func (st *StateTracker) Set(ctx context.Context, state MatchState) error {
	key, err := stateKey(state.RuleID, state.EntityKey)
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode rule state %s: %w", key, err)
	}
	if _, err := st.kv.Put(ctx, key, data); err != nil {
		return fmt.Errorf("put rule state %s: %w", key, err)
	}
	return nil
}

// DeleteAllForEntity deletes every rule's record for entityID. It refuses an ID
// that is not a literal key, so a wildcard deletes nothing. It tries every
// record and returns the failures joined, each naming its key; no record is nil.
func (st *StateTracker) DeleteAllForEntity(ctx context.Context, entityID string) error {
	if err := natsclient.ValidateKVLiteralKey(entityID); err != nil {
		return fmt.Errorf("rule state entity ID %q: %w", entityID, err)
	}
	keys, err := st.kv.KeysByFilter(ctx, "*."+entityID)
	if err != nil {
		return fmt.Errorf("list rule state for %s: %w", entityID, err)
	}
	var failures []error
	for _, key := range keys {
		if err := st.kv.Delete(ctx, key); err != nil {
			failures = append(failures, fmt.Errorf("delete rule state %s: %w", key, err))
		}
	}
	return errors.Join(failures...)
}
