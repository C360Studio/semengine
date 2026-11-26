package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/c360studio/semengine/internal/harness/semantictest"
)

// Requirement: rule-substitution/"A token ends where its name ends" and "An
// unresolved token stays and is reported", for $message.<path>.

func TestMessageFlatFields(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{MessageData: map[string]any{"loop_id": "loop-abc", "call_id": "call-001", "tool_name": "bash"}}

	assert.Equal(t, "loop=loop-abc call=call-001 tool=bash",
		ec.SubstituteVariables(t.Context(), "loop=$message.loop_id call=$message.call_id tool=$message.tool_name"))
}

// A path reads through nested maps, so a field of a tool call's arguments can
// be put in a subject or a reason.
func TestMessageDeepPath(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{MessageData: map[string]any{
		"tool_name": "bash",
		"tool_args": map[string]any{"command": "rm -rf /tmp/danger", "timeout": 30},
	}}

	assert.Equal(t, "tool=bash cmd=rm -rf /tmp/danger timeout=30",
		ec.SubstituteVariables(t.Context(), "tool=$message.tool_name cmd=$message.tool_args.command timeout=$message.tool_args.timeout"))
}

// With no message data, or a path that is not there, the token stays and is
// reported, so an empty string never lands in a subject or key unnoticed.
func TestMessageUnresolvedPathLeavesToken(t *testing.T) {
	log := captureWarnings(t)

	cases := map[string]struct {
		data  map[string]any
		in    string
		token string
	}{
		"nil data":                     {data: nil, in: "loop=$message.loop_id", token: "$message.loop_id"},
		"empty data":                   {data: map[string]any{}, in: "loop=$message.loop_id", token: "$message.loop_id"},
		"missing terminal key":         {data: map[string]any{"loop_id": "loop-abc"}, in: "call=$message.call_id", token: "$message.call_id"},
		"missing intermediate":         {data: map[string]any{"tool_name": "bash"}, in: "cmd=$message.tool_args.command", token: "$message.tool_args.command"},
		"intermediate is a string":     {data: map[string]any{"tool_args": "not-a-map"}, in: "cmd=$message.tool_args.command", token: "$message.tool_args.command"},
		"intermediate is a number":     {data: map[string]any{"tool_args": 42}, in: "cmd=$message.tool_args.command", token: "$message.tool_args.command"},
		"a longer path is not shorter": {data: map[string]any{"tool_args": map[string]any{}}, in: "$message.tool_args.x", token: "$message.tool_args.x"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ec := &ExecutionContext{MessageData: tc.data}

			assert.Equal(t, tc.in, ec.SubstituteVariables(t.Context(), tc.in))
			assert.Equal(t, []string{tc.token}, log.unresolved(t))
		})
	}
}

// A token that resolves and one that does not, in one template, each keep
// their own outcome.
func TestMessageMixedResolution(t *testing.T) {
	log := captureWarnings(t)
	ec := &ExecutionContext{MessageData: map[string]any{"loop_id": "loop-abc"}}

	assert.Equal(t, "loop=loop-abc call=$message.call_id",
		ec.SubstituteVariables(t.Context(), "loop=$message.loop_id call=$message.call_id"))
	assert.Equal(t, []string{"$message.call_id"}, log.unresolved(t))
}

// A message path ends at a character its grammar does not take, so a token
// followed by punctuation or a hyphen reads only its own path.
func TestMessagePathEnds(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{MessageData: map[string]any{"loop_id": "L", "call_id": "C"}}

	assert.Equal(t, "L. C, L-x", ec.SubstituteVariables(t.Context(), "$message.loop_id. $message.call_id, $message.loop_id-x"))
}

func TestMessageBesideEntity(t *testing.T) {
	t.Parallel()

	id := semantictest.EntityID(t, "c360", "osh", "agentic-loop", "agent", "execution", "uuid-001")
	ec := &ExecutionContext{
		EntityID: id,
		MessageData: map[string]any{
			"loop_id":   "loop-abc",
			"call_id":   "call-001",
			"tool_name": "bash",
			"tool_args": map[string]any{"command": "ls /tmp"},
		},
	}

	assert.Equal(t, "rule fired on "+id+" for loop=loop-abc call=call-001 cmd=ls /tmp entity=uuid-001 tool=bash",
		ec.SubstituteVariables(t.Context(),
			"rule fired on $entity.id for loop=$message.loop_id call=$message.call_id cmd=$message.tool_args.command entity=$entity.instance tool=$message.tool_name"))
}

// A message field in a subject: the use a tool-call verdict makes of it.
func TestMessageInSubject(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{MessageData: map[string]any{"execution_id": "tool-exec-001"}}

	assert.Equal(t, "agent.toolcall.rejected.tool-exec-001",
		ec.SubstituteVariables(t.Context(), "agent.toolcall.rejected.$message.execution_id"))
}
