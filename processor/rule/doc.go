// Package rule holds rule definitions and the one step that checks them
// before they run.
//
// # Definitions
//
// A Definition is one rule as an author writes it in JSON: an ID, a type
// (`expression`, or `cron`), conditions combined by `logic`, the entity
// pattern it watches, and lists of actions to run when its conditions start
// to match (`on_enter`), stop matching (`on_exit`), keep matching
// (`while_true`) or still match after a restart (`on_recovery`). A cron rule
// lists its actions under `actions`.
//
// DecodeDefinition is the one decode of a rule's JSON, used for every source:
// a rules file, an inline rule in a processor's configuration, a stored rule.
// It refuses a key the rule, its entity config or a condition does not
// declare, a declared key spelt with another case (`"Field"` for
// `"field"`), and a key repeated in any object, so a typo or a stray copy
// fails instead of being ignored or silently winning. An absent `enabled`
// means enabled, an absent `logic` means `and`, and an absent condition
// `required` means not required.
//
// # Actions
//
// An Action keeps its JSON object as written in Body; the action's type,
// `when` guard, `id` and `max_iterations` are copies the decode fills from
// it. Every other key belongs to the handler of the action's type, which
// reads Body itself, so a product's action type gets its settings untouched.
// Build an action by decoding its JSON, never as a Go literal.
//
// An ActionHandler checks the actions of one type. NewHandlers builds the
// handler set: the nine built-in types (ActionTypePublish and the others)
// plus a product's own, refusing a product type that reuses a built-in name.
// A built-in handler refuses a body key its type does not read, a `publish`
// subject in the reserved `user.response.>` family, and a triple predicate
// that is substituted or not declared.
//
// An action's `max_iterations` is its firing cap: the most times the rule
// processor that runs rules may fire it for one rule and entity match
// (absent means 3, 0 means no cap). Load checks the value and works out the
// key that count is kept under: the action's `id`, or, without one, a hash of
// the rule ID, the type and the body in canonical form, so any edit to the
// action gives a new key while reformatting its file does not.
//
// # Loading
//
// Load checks a definition against a handler set and returns the Rule ready
// to run, or refuses it. It checks disabled rules the same way, so a broken
// rule cannot wait, switched off, until someone enables it. A field in a
// rule condition and in an action's `when` gets the same check: it starts
// with `$` (`$message.`, `$state.` and the like) or is a declared predicate
// that is not rule-opaque, as a rule-opaque predicate holds private content
// no rule may match on. An expression rule's condition reads only what its
// path has: with no `entity.pattern` the rule runs on messages and may not
// name `$entity.` in a field or value, and with one it runs on entities and
// may not name `$message.`. A refusal is
// classified invalid and names the rule ID, the source, the position (such as
// `on_enter[1].when[0]`) and the cause:
//
//	handlers, err := rule.NewHandlers(productHandlers)
//	if err != nil {
//	    return err
//	}
//	def, err := rule.DecodeDefinition(data, path)
//	if err != nil {
//	    return err
//	}
//	loaded, err := rule.Load(def, path, handlers)
//	if err != nil {
//	    return err
//	}
//
// Nothing in this package runs a rule: Load is where a rule source checks its
// rules before any of them fires. An enabled cron rule is refused, as nothing
// yet fires one on its schedule.
package rule
