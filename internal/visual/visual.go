// Package visual checks the animated visuals a tour declares: a step's
// `visual` object, and the docent-* fences in Markdown bodies. Visuals are
// data, never code, and this is where that data is held to its schema.
package visual

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/primandproper/docent/schema"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// FencePrefix starts the fence language of every Docent embed.
const FencePrefix = "docent-"

// LinksFence is the fence that holds an array of link cards.
const LinksFence = FencePrefix + "links"

// Types are the visuals there are.
var Types = []string{"sequence", "flow", "state", "timeline"}

type (
	// Fence is a fenced block of a Markdown document.
	Fence struct {
		_ struct{}
		// Lang is the first word after the opening fence.
		Lang string
		Body string
		// Line is the 1-based line the fence opens on.
		Line int
	}

	// Problem is one thing wrong with a visual.
	Problem struct {
		_ struct{}
		// Where is a path into the visual, like "messages/2/to", or "".
		Where   string
		Message string
	}
)

// schemas compiles the schema for each type of visual, once. A visual is
// checked against its own type's schema, not the top-level oneOf, so the
// errors are about the visual the author meant to write.
var schemas = sync.OnceValues(func() (map[string]*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.VisualV1))
	if err != nil {
		return nil, fmt.Errorf("decoding the embedded visual schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err = c.AddResource(schema.VisualV1ID, doc); err != nil {
		return nil, fmt.Errorf("registering the embedded visual schema: %w", err)
	}

	out := make(map[string]*jsonschema.Schema, len(Types))

	for _, kind := range Types {
		if out[kind], err = c.Compile(schema.VisualV1ID + "#/$defs/" + kind); err != nil {
			return nil, fmt.Errorf("compiling the %s visual schema: %w", kind, err)
		}
	}

	return out, nil
})

// Fences lists the fenced code blocks of a Markdown document. It understands
// backtick and tilde fences of any length, which is all a body needs.
func Fences(markdown string) []Fence {
	var (
		out    []Fence
		open   *Fence
		marker string
		body   []string
	)

	for i, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))

		if open == nil {
			for _, char := range []string{"`", "~"} {
				if run := leadingRun(trimmed, char); len(run) >= 3 {
					marker = run
					lang, _, _ := strings.Cut(strings.TrimSpace(trimmed[len(run):]), " ")
					open = &Fence{Lang: lang, Line: i + 1}
					body = nil
				}
			}

			continue
		}

		if run := leadingRun(trimmed, marker[:1]); len(run) >= len(marker) && run == trimmed {
			open.Body = strings.Join(body, "\n")
			out = append(out, *open)
			open = nil

			continue
		}

		body = append(body, strings.TrimSuffix(line, "\r"))
	}

	return out
}

func leadingRun(s, char string) string {
	return s[:len(s)-len(strings.TrimLeft(s, char))]
}

// Decode parses a fence body, which is YAML (and so may be JSON), into plain
// values.
func Decode(body string) (any, error) {
	var value any
	if err := yaml.Unmarshal([]byte(body), &value); err != nil {
		return nil, fmt.Errorf("not valid YAML: %w", err)
	}

	return value, nil
}

// FromFence decodes a docent-<type> fence into a visual, adding the `type`
// its fence language stands for.
func FromFence(f *Fence) (map[string]any, error) {
	value, err := Decode(f.Body)
	if err != nil {
		return nil, err
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("a visual is written as a set of fields, not a list or a single value")
	}

	object["type"] = strings.TrimPrefix(f.Lang, FencePrefix)

	return object, nil
}

// Validate checks a visual against the schema, then checks what a schema
// can't: that every reference names something the visual defines.
func Validate(value any) ([]Problem, error) {
	byType, err := schemas()
	if err != nil {
		return nil, err
	}

	// Normalize through JSON so numbers and maps are the types the schema
	// library expects, whatever decoded them.
	encoded, err := json.Marshal(value)
	if err != nil {
		return []Problem{{Message: fmt.Sprintf("not representable as JSON: %v", err)}}, nil
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("decoding visual: %w", err)
	}

	object, ok := instance.(map[string]any)
	if !ok {
		return []Problem{{Message: "a visual is a set of fields, not a list or a single value"}}, nil
	}

	kind, _ := object["type"].(string) //nolint:errcheck // a non-string type is reported just below

	sch, known := byType[kind]
	if !known {
		return []Problem{{Where: "type", Message: fmt.Sprintf("there is no %q visual; the visuals are %s", fmt.Sprint(object["type"]), strings.Join(Types, ", "))}}, nil
	}

	if err = sch.Validate(instance); err != nil {
		var verr *jsonschema.ValidationError
		if !errors.As(err, &verr) {
			return nil, fmt.Errorf("validating visual: %w", err)
		}

		return schemaProblems(verr), nil
	}

	return references(object), nil
}

func schemaProblems(verr *jsonschema.ValidationError) []Problem {
	printer := message.NewPrinter(language.English)

	var out []Problem

	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, cause := range e.Causes {
				walk(cause)
			}

			return
		}

		out = append(out, Problem{Where: strings.Join(e.InstanceLocation, "/"), Message: e.ErrorKind.LocalizedString(printer)})
	}
	walk(verr)

	return out
}

// references checks the ids a visual uses against the ones it declares.
func references(v map[string]any) []Problem {
	var out []Problem

	check := func(known []string, what, where string, id any) {
		if s, ok := id.(string); ok && !slices.Contains(known, s) {
			out = append(out, Problem{Where: where, Message: fmt.Sprintf("%q is not one of the %s (%s)", s, what, strings.Join(known, ", "))})
		}
	}

	switch v["type"] {
	case "sequence":
		known := ids(v["participants"])
		for i, m := range objects(v["messages"]) {
			check(known, "participants", fmt.Sprintf("messages/%d/from", i), m["from"])
			check(known, "participants", fmt.Sprintf("messages/%d/to", i), m["to"])
		}
	case "flow":
		known := ids(v["nodes"])
		edges := map[string]bool{}

		for i, e := range objects(v["edges"]) {
			check(known, "nodes", fmt.Sprintf("edges/%d/from", i), e["from"])
			check(known, "nodes", fmt.Sprintf("edges/%d/to", i), e["to"])
			edges[fmt.Sprint(e["from"])+"\x00"+fmt.Sprint(e["to"])] = true
		}

		for i, p := range objects(v["paths"]) {
			through, _ := p["through"].([]any) //nolint:errcheck // the schema already required an array

			for j := range through {
				check(known, "nodes", fmt.Sprintf("paths/%d/through/%d", i, j), through[j])

				if j > 0 && !edges[fmt.Sprint(through[j-1])+"\x00"+fmt.Sprint(through[j])] {
					out = append(out, Problem{
						Where:   fmt.Sprintf("paths/%d/through/%d", i, j),
						Message: fmt.Sprintf("there is no edge from %q to %q for this path to travel along", through[j-1], through[j]),
					})
				}
			}
		}
	case "state":
		out = append(out, stateProblems(v, check)...)
	case "timeline":
		known := ids(v["lanes"])
		for i, e := range objects(v["events"]) {
			check(known, "lanes", fmt.Sprintf("events/%d/lane", i), e["lane"])
		}
	}

	return out
}

// stateProblems plays a state machine's run and reports the first event that
// has nowhere to go.
func stateProblems(v map[string]any, check func(known []string, what, where string, id any)) []Problem {
	var out []Problem

	known := ids(v["states"])
	transitions := objects(v["transitions"])

	for i, t := range transitions {
		check(known, "states", fmt.Sprintf("transitions/%d/from", i), t["from"])
		check(known, "states", fmt.Sprintf("transitions/%d/to", i), t["to"])
	}

	if len(known) == 0 {
		return out
	}

	current := known[0]
	run, _ := v["run"].([]any) //nolint:errcheck // the schema already required an array

	for i, event := range run {
		next := ""

		for _, t := range transitions {
			if t["from"] == current && t["on"] == event {
				next = fmt.Sprint(t["to"])
				break
			}
		}

		if next == "" {
			return append(out, Problem{
				Where:   fmt.Sprintf("run/%d", i),
				Message: fmt.Sprintf("state %q has no transition on %q", current, event),
			})
		}

		current = next
	}

	return out
}

// ids reads a list of nodes, each a bare id or an object with one.
func ids(value any) []string {
	list, _ := value.([]any) //nolint:errcheck // not a list means no ids

	out := make([]string, 0, len(list))

	for _, item := range list {
		switch n := item.(type) {
		case string:
			out = append(out, n)
		case map[string]any:
			if id, ok := n["id"].(string); ok {
				out = append(out, id)
			}
		}
	}

	return out
}

func objects(value any) []map[string]any {
	list, _ := value.([]any) //nolint:errcheck // not a list means no objects

	out := make([]map[string]any, 0, len(list))

	for _, item := range list {
		if object, ok := item.(map[string]any); ok {
			out = append(out, object)
		}
	}

	return out
}

// StepRefs lists the tour step ids a visual links its frames to.
func StepRefs(value any) []string {
	object, _ := value.(map[string]any) //nolint:errcheck // not an object means no references

	var out []string

	for _, key := range []string{"messages", "paths", "events"} {
		for _, item := range objects(object[key]) {
			if id, ok := item["step"].(string); ok {
				out = append(out, id)
			}
		}
	}

	return out
}
