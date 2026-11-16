package visual

import (
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const sequenceBody = `participants: [client, api, queue, worker, cache]
messages:
  - { from: client, to: api, label: "PUT /items/42" }
  - { from: api, to: queue, label: "enqueue invalidate(42)", async: true, step: enqueue }
  - { from: queue, to: worker, label: "deliver" }
  - { from: worker, to: cache, label: "PURGE /items/42", step: purge }
`

func validateFence(t *testing.T, lang, body string) []Problem {
	t.Helper()

	v, err := FromFence(&Fence{Lang: lang, Body: body})
	must.NoError(t, err)

	problems, err := Validate(v)
	must.NoError(t, err)

	return problems
}

func TestValidate(T *testing.T) {
	T.Parallel()

	T.Run("the design doc's sequence is valid, as YAML or JSON", func(t *testing.T) {
		t.Parallel()

		test.Len(t, 0, validateFence(t, "docent-sequence", sequenceBody))
		test.Len(t, 0, validateFence(t, "docent-sequence", `{"participants": ["a", {"id": "b", "label": "B"}], "messages": [{"from": "a", "to": "b", "label": "hi"}]}`))
	})

	T.Run("each of the other visuals has a valid form", func(t *testing.T) {
		t.Parallel()

		test.Len(t, 0, validateFence(t, "docent-flow", `
nodes: [client, {id: api, label: API}, queue, worker]
edges:
  - { from: client, to: api }
  - { from: api, to: queue, label: enqueue }
  - { from: queue, to: worker }
paths:
  - { label: "A write", through: [client, api, queue, worker], step: write-path }
`))
		test.Len(t, 0, validateFence(t, "docent-state", `
states: [idle, {id: running, label: Running}, done]
transitions:
  - { from: idle, to: running, on: start }
  - { from: running, to: running, on: retry }
  - { from: running, to: done, on: finish }
run: [start, retry, retry, finish]
`))
		test.Len(t, 0, validateFence(t, "docent-timeline", `
unit: ms
lanes: [worker, cache]
events:
  - { lane: worker, at: 0, label: "attempt 1" }
  - { lane: worker, at: 200.5, label: "attempt 2", step: retries }
  - { lane: cache, at: 250, label: purged }
`))
	})

	T.Run("schema violations say where", func(t *testing.T) {
		t.Parallel()

		problems := validateFence(t, "docent-sequence", "participants: [a, b]\nmessages:\n  - { from: a, to: b }\nextra: 1\n")
		where := map[string]bool{}
		for _, p := range problems {
			where[p.Where] = true
		}

		test.True(t, where["messages/0"], test.Sprintf("%+v", problems))
		test.True(t, where[""], test.Sprintf("%+v", problems))
	})

	T.Run("references to things the visual doesn't define are caught", func(t *testing.T) {
		t.Parallel()

		problems := validateFence(t, "docent-sequence", "participants: [a, b]\nmessages:\n  - { from: a, to: c, label: x }\n")
		must.Len(t, 1, problems)
		test.Eq(t, "messages/0/to", problems[0].Where)
		test.StrContains(t, problems[0].Message, `"c" is not one of the participants (a, b)`)

		problems = validateFence(t, "docent-flow", "nodes: [a, b, c]\nedges:\n  - { from: a, to: b }\npaths:\n  - { label: p, through: [a, b, c] }\n")
		must.Len(t, 1, problems)
		test.Eq(t, "paths/0/through/2", problems[0].Where)
		test.StrContains(t, problems[0].Message, `no edge from "b" to "c"`)

		problems = validateFence(t, "docent-timeline", "lanes: [a]\nevents:\n  - { lane: b, at: 1, label: x }\n")
		must.Len(t, 1, problems)
		test.Eq(t, "events/0/lane", problems[0].Where)
	})

	T.Run("a state machine's run has to be playable", func(t *testing.T) {
		t.Parallel()

		problems := validateFence(t, "docent-state", "states: [idle, busy]\ntransitions:\n  - { from: idle, to: busy, on: start }\nrun: [start, start]\n")
		must.Len(t, 1, problems)
		test.Eq(t, "run/1", problems[0].Where)
		test.StrContains(t, problems[0].Message, `state "busy" has no transition on "start"`)
	})

	T.Run("an unknown type and a non-object are refused", func(t *testing.T) {
		t.Parallel()

		problems, err := Validate(map[string]any{"type": "pie"})
		must.NoError(t, err)
		must.Len(t, 1, problems)
		test.StrContains(t, problems[0].Message, `there is no "pie" visual`)

		problems, err = Validate([]any{"x"})
		must.NoError(t, err)
		test.Len(t, 1, problems)

		_, err = FromFence(&Fence{Lang: "docent-sequence", Body: "- a\n- b\n"})
		test.Error(t, err)

		_, err = FromFence(&Fence{Lang: "docent-sequence", Body: "a: [unclosed"})
		test.Error(t, err)
	})
}

func TestFences(T *testing.T) {
	T.Parallel()

	T.Run("finds fenced blocks with their language and line", func(t *testing.T) {
		t.Parallel()

		got := Fences("intro\n\n```docent-sequence\nparticipants: [a, b]\n```\n\ntext ``` not a fence\n\n~~~~mermaid extra\nflowchart LR\n~~~\nstill inside\n~~~~\n\n```\nplain\n```\n\n```go\nnever closed\n")
		test.Eq(t, []Fence{
			{Lang: "docent-sequence", Body: "participants: [a, b]", Line: 3},
			{Lang: "mermaid", Body: "flowchart LR\n~~~\nstill inside", Line: 9},
			{Lang: "", Body: "plain", Line: 15},
		}, got)
	})
}

func TestStepRefs(T *testing.T) {
	T.Parallel()

	T.Run("lists the steps a visual's frames point at", func(t *testing.T) {
		t.Parallel()

		v, err := FromFence(&Fence{Lang: "docent-sequence", Body: sequenceBody})
		must.NoError(t, err)
		test.Eq(t, []string{"enqueue", "purge"}, StepRefs(v))
		test.Len(t, 0, StepRefs("nope"))
	})
}
