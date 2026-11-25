package rule

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/c360studio/semengine/vocabulary"
)

// reservedUserResponseSubjectFamily is the subject family reserved for typed
// user-response messages. A rule's publish action may not declare a subject
// in it (ADR-093).
const reservedUserResponseSubjectFamily = "user.response.>"

// coreActionKeys are the body keys the engine reads for every action type.
var coreActionKeys = []string{"type", "when", "id", "max_iterations"}

// builtinBodyKeys lists, for each built-in type, the body keys its handler
// reads beside the core keys. A built-in refuses any other key, so a
// misspelt setting fails at load instead of being ignored when the action
// runs.
var builtinBodyKeys = map[string][]string{
	ActionTypePublish:             {"subject", "properties"},
	ActionTypeAddTriple:           {"subject", "predicate", "object", "ttl"},
	ActionTypeRemoveTriple:        {"subject", "predicate", "object"},
	ActionTypeUpdateTriple:        {"subject", "predicate", "object", "ttl", "properties"},
	ActionTypeReconcilePredicates: {"subject", "predicate", "object", "ttl", "projection_contract", "projection_group"},
	ActionTypeUpdateKV:            {"bucket", "key", "payload", "merge"},
	ActionTypeLifecycleTransition: {"workflow", "phase", "reason", "set"},
	ActionTypeLifecycleComplete:   {"workflow"},
	ActionTypeLifecycleFail:       {"workflow", "reason"},
}

// builtinSettings holds the values of every built-in type's body keys, so
// that a value of the wrong JSON type is refused at load. A key a type does
// not read is refused before this decode.
type builtinSettings struct {
	Subject            string         `json:"subject"`
	Predicate          string         `json:"predicate"`
	Object             string         `json:"object"`
	TTL                string         `json:"ttl"`
	Properties         map[string]any `json:"properties"`
	ProjectionContract string         `json:"projection_contract"`
	ProjectionGroup    string         `json:"projection_group"`
	Bucket             string         `json:"bucket"`
	Key                string         `json:"key"`
	Payload            map[string]any `json:"payload"`
	Merge              bool           `json:"merge"`
	Workflow           string         `json:"workflow"`
	Phase              string         `json:"phase"`
	Reason             string         `json:"reason"`
	Set                map[string]any `json:"set"`
}

// builtinHandler checks the actions of one built-in type.
type builtinHandler struct {
	actionType string
}

// Check refuses a body key that neither the core nor the type reads, naming
// the key, and a value of the wrong JSON type. A publish action may not
// declare a subject in user.response.>, and a triple action's predicate
// must be a declared literal.
func (h builtinHandler) Check(a Action) error {
	if err := checkBodyKeys(h.actionType, a.Body, builtinBodyKeys[h.actionType]); err != nil {
		return err
	}
	var settings builtinSettings
	if err := json.Unmarshal(a.Body, &settings); err != nil {
		return fmt.Errorf("%s action: %w", h.actionType, err)
	}
	switch h.actionType {
	case ActionTypePublish:
		if inReservedUserResponseFamily(settings.Subject) {
			return fmt.Errorf("publish action subject %q is in the reserved family %q",
				settings.Subject, reservedUserResponseSubjectFamily)
		}
	case ActionTypeAddTriple, ActionTypeRemoveTriple, ActionTypeUpdateTriple:
		if strings.Contains(settings.Predicate, "$") {
			return fmt.Errorf("%s action predicate %q must be a declared literal; a predicate is never substituted",
				h.actionType, settings.Predicate)
		}
		if err := vocabulary.RequireDeclaredPredicate(settings.Predicate); err != nil {
			return fmt.Errorf("%s action predicate: %w", h.actionType, err)
		}
	}
	return nil
}

// checkBodyKeys refuses a key of an actionType action's body that is
// neither a core key nor in typeKeys. Keys match exactly as written, and the first refused key in
// sorted order is named, so the error is the same on every run.
func checkBodyKeys(actionType string, body json.RawMessage, typeKeys []string) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return fmt.Errorf("action body: %w", err)
	}
	read := make(map[string]bool, len(coreActionKeys)+len(typeKeys))
	for _, key := range append(append([]string(nil), coreActionKeys...), typeKeys...) {
		read[key] = true
	}
	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		if !read[key] {
			return fmt.Errorf("action body key %q is not read by %s actions", key, actionType)
		}
	}
	return nil
}

// inReservedUserResponseFamily reports whether subject is inside
// user.response.>: three or more tokens, the first two `user` and
// `response`.
func inReservedUserResponseFamily(subject string) bool {
	tokens := strings.Split(subject, ".")
	return len(tokens) >= 3 && tokens[0] == "user" && tokens[1] == "response"
}
