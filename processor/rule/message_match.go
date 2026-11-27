package rule

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// matchMessage is the message match the rule processor fires on. It answers
// on the conditions alone, as the rule processor picks the rules a message
// reaches before calling it. A disabled rule, or one with no conditions, does
// not match.
//
// Conditions read the fields the payload declares (ruleFields). A payload
// that declares none is no match, with no error: the rule can never fire for
// that payload type, which is reported once (reportUnreadable). Condition
// values are filled from those fields under ctx, and one whose tokens do not
// resolve is an error. An absent required field is no match, with no error;
// any other evaluation error is returned with the rule ID.
//
// It is safe for concurrent use; the report is the only state it writes.
func (r *Rule) matchMessage(ctx context.Context, msg message.Message) (bool, error) {
	if !r.def.Enabled || len(r.def.Conditions) == 0 {
		return false, nil
	}

	data, readable := ruleFields(msg)
	if !readable {
		r.reportUnreadable(ctx, msg)
		return false, nil
	}

	ec := &ExecutionContext{MessageData: data}
	conditions, err := substituteConditionValues(ctx, r.def.Conditions, ec)
	if err != nil {
		return false, fmt.Errorf("rule %q: %w", r.def.ID, err)
	}

	matched, err := expression.NewExpressionEvaluator().EvaluateWithStateAndMessage(nil, nil,
		expression.MessageFields(data), expression.LogicalExpression{Conditions: conditions, Logic: r.def.Logic})
	switch {
	case err == nil:
		return matched, nil
	case errors.Is(err, expression.ErrRequiredFieldMissing):
		// Rules mark a field required to mean "match only when it is
		// present", so its absence is the ordinary no match.
		return false, nil
	}
	return false, fmt.Errorf("rule %q: %w", r.def.ID, err)
}

// reportUnreadable logs at Warn, once for this loaded rule and the message's
// payload type, that the rule cannot read the payload. The fact does not
// change between messages, so one report per pair keeps a busy subject from
// flooding the log; a rule loaded again reports again, as its definition may
// be the fix. The set is bounded by the registered payload types.
func (r *Rule) reportUnreadable(ctx context.Context, msg message.Message) {
	// shortcut: logged only; a per-rule, per-type metric comes with the rule processor's metrics
	payloadType := payloadTypeLabel(msg)
	if _, reported := r.unreadableReported.LoadOrStore(payloadType, struct{}{}); reported {
		return
	}
	slog.Default().WarnContext(ctx,
		"rule cannot read this payload type: its message conditions never hold and the rule never fires on it",
		slog.String("rule_id", r.def.ID),
		slog.String("payload_type", payloadType),
		slog.String("remedy", "implement message.RuleReadable on the payload type"))
}

// ruleFields returns the fields a message's payload lets rules read: those
// its message.RuleReadable implementation declares, which a
// message.GenericJSONPayload answers with its data map. It is the one reading
// of a payload for rules: whatever gives a rule a payload's fields reads them
// here, so all of it sees the same fields. Nothing reads the payload's
// structure.
//
// The bool tells two answers apart. (nil or empty, true): readable, with
// nothing to match on, so a condition over it is false in the ordinary way.
// (nil, false): the payload declares no fields at all, so no message
// condition can ever hold for it, which the caller reports.
func ruleFields(msg message.Message) (map[string]any, bool) {
	if msg == nil {
		return nil, false
	}
	payload := msg.Payload()
	if payload == nil {
		return nil, false
	}
	if readable, ok := payload.(message.RuleReadable); ok {
		return readable.RuleFields(), true
	}
	return nil, false
}

// payloadTypeLabel names a message's payload in the unreadable report and
// keys the report's once-per-type set. The registered message type is the
// name a rule author recognises and is bounded by the payload registry; a
// message with no valid type, built by hand or decoded in part, falls back to
// the payload's Go type.
func payloadTypeLabel(msg message.Message) string {
	if msg == nil {
		return "none"
	}
	if msgType := msg.Type(); msgType.IsValid() {
		return msgType.String()
	}
	if payload := msg.Payload(); payload != nil {
		return fmt.Sprintf("%T", payload)
	}
	return "none"
}
