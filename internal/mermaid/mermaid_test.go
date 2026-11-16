package mermaid

import (
	"testing"

	"github.com/shoenig/test"
)

func TestParse(T *testing.T) {
	T.Parallel()

	T.Run("finds nodes, subgraphs and named edges", func(t *testing.T) {
		t.Parallel()

		m := Parse(`---
title: Cache invalidation
---
flowchart LR
    %% the write path
    client([Client]) --> api[API -- the "front" door]
    api e1@--> queue[(Queue)]
    queue e2@-. deliver .-> worker{{Worker}}
    worker e3@==>|PURGE /items/42| cache
    api -->|read| store[(Store)] & cache
    subgraph edge [Edge network]
        cache
    end
    orphan
    a-b --- c.d; x --o y; y <--> z
    api -- writes --> store
    classDef hot fill:#f96
    class api,worker hot
    style cache fill:#bbf
    linkStyle 0 stroke:#333
    click api "docent:step/write-path"
`)

		test.Eq(t, []string{"client", "api", "queue", "worker", "cache", "store", "edge", "orphan", "a-b", "c.d", "x", "y", "z"}, m.Nodes)
		test.Eq(t, []string{"e1", "e2", "e3"}, m.Edges)
		test.True(t, m.Has("api"))
		test.True(t, m.Has("e2"))
		test.False(t, m.Has("hot"))
		test.False(t, m.Has("nope"))
	})

	T.Run("the graph keyword and top-down layouts work the same", func(t *testing.T) {
		t.Parallel()

		m := Parse("graph TD\n  A[Start] --> B{Is it?}\n  B -->|Yes| C[OK]\n  B -- No --> D[End]\n")
		test.Eq(t, []string{"A", "B", "C", "D"}, m.Nodes)
		test.Len(t, 0, m.Edges)
	})

	T.Run("an empty or unreadable source defines nothing", func(t *testing.T) {
		t.Parallel()

		test.Len(t, 0, Parse("").Nodes)
		test.Len(t, 0, Parse("flowchart LR\n").Nodes)
	})
}

func TestIsFlowchart(T *testing.T) {
	T.Parallel()

	T.Run("only flowcharts qualify", func(t *testing.T) {
		t.Parallel()

		test.True(t, IsFlowchart("flowchart LR\n a --> b"))
		test.True(t, IsFlowchart("\n%% comment\ngraph TD\n a --> b"))
		test.True(t, IsFlowchart("---\ntitle: x\n---\nflowchart LR"))
		test.False(t, IsFlowchart("sequenceDiagram\n a->>b: hi"))
		test.False(t, IsFlowchart(""))
	})
}
