package rule

import (
	"strings"

	"github.com/c360studio/semengine/processor/rule/expression"
)

// SubstituteVariablesTyped returns the value of template, with its own Go
// type, and true when the whole template is one token that keeps a type and
// resolves:
//
//   - $entity.triple.<predicate>, bare: the first triple's object as stored,
//     so a float64 stays a float64. The suffix forms .length, .triples and
//     .value are text.
//   - $message.<path>: the field as MessageData holds it. A present field
//     holding JSON null gives nil and true.
//   - $state.iteration and $state.max_iterations: an int.
//
// Every other template returns nil and false: text around a token, two
// tokens, a token that does not resolve, a template with no token, and the
// tokens whose value is always text ($now, $entity.id, the entity parts and
// $entity.lifecycle.*). The caller then substitutes with SubstituteVariables.
// The token grammar is SubstituteVariables'.
func (ec *ExecutionContext) SubstituteVariablesTyped(template string) (any, bool) {
	switch {
	case strings.HasPrefix(template, triplePrefix):
		predicate, form, n, ok := parseTriple(template)
		if !ok || form != tripleBare || n != len(template) {
			return nil, false
		}
		return firstObjectTyped(ec.Entity, predicate)
	case strings.HasPrefix(template, messagePrefix):
		path := template[len(messagePrefix):]
		if path == "" || dottedRun(path, isWordByte) != len(path) || ec.MessageData == nil {
			return nil, false
		}
		return expression.ExtractMessageValue(expression.MessageFields(ec.MessageData), path)
	case ec.State == nil:
		return nil, false
	case template == "$state.iteration":
		return ec.State.Iteration, true
	case template == "$state.max_iterations":
		return ec.State.MaxIterations, true
	}
	return nil, false
}
