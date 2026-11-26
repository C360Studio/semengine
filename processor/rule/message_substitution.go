package rule

import (
	"fmt"

	"github.com/c360studio/semengine/processor/rule/expression"
)

// $message.<path> reads a field of the message a rule was evaluated for, so an
// action's subject or reason can carry a value from that message. The path is
// dotted, as in $message.tool_args.command, and is read with
// expression.ExtractMessageValue, the same reading conditions use. A path that
// is not there, one that descends through a value that is not a map, and a
// context with no message data leave the token as written, to be reported.

const messagePrefix = "$message."

// resolveMessage reads the $message.<path> token at the start of s.
func resolveMessage(s string, data map[string]any) (string, int, bool) {
	path := s[len(messagePrefix):]
	path = path[:dottedRun(path, isWordByte)]
	if path == "" {
		return "", 0, false
	}
	value, ok := expression.ExtractMessageValue(expression.MessageFields(data), path)
	if !ok {
		return "", 0, false
	}
	return fmt.Sprintf("%v", value), len(messagePrefix) + len(path), true
}
