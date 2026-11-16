// Package mermaid reads just enough of a Mermaid flowchart to know which
// nodes and edges it defines, so a tour's `mapFocus` can be checked without a
// browser. It is deliberately forgiving: anything it doesn't recognize is
// skipped, never an error.
package mermaid

import (
	"regexp"
	"slices"
	"strings"
)

var (
	// An edge, optionally named with Mermaid's `id@` syntax: `e1@-->`,
	// `-.->`, `==>`, `---`, `--o`, `--x`, `<-->`, and the `-- text -->` forms.
	edge = regexp.MustCompile(`(?:([A-Za-z_][\w-]*)@)?(?:<|o|x)?(?:--+|==+|-\.+-?)(?:[^-=.>|]*?(?:--+|==+|\.+-))?(?:>|o|x)?`)
	// An edge label written between pipes: `-->|text|`.
	pipeLabel = regexp.MustCompile(`\|[^|]*\|`)
	// A node id at the start of a node reference.
	nodeID = regexp.MustCompile(`^[A-Za-z0-9_][\w.-]*`)
	// Quoted text and bracketed shapes, whose contents aren't syntax.
	quoted = regexp.MustCompile(`"[^"]*"`)

	// Statements that define no nodes.
	skipped = []string{"classDef", "class", "style", "linkStyle", "click", "direction", "accTitle", "accDescr", "end"}
)

// Map is what a flowchart defines that a step can focus.
type Map struct {
	_ struct{}
	// Nodes are node and subgraph ids, in order of first appearance.
	Nodes []string
	// Edges are the ids of edges named with the `id@` syntax.
	Edges []string
}

// Has reports whether id names a node or a named edge.
func (m *Map) Has(id string) bool {
	return slices.Contains(m.Nodes, id) || slices.Contains(m.Edges, id)
}

// IsFlowchart reports whether source starts as a flowchart, the only diagram
// type a system map can be.
func IsFlowchart(source string) bool {
	frontmatter := false

	for i, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)

		if line == "---" && (i == 0 || frontmatter) {
			frontmatter = !frontmatter
			continue
		}

		if frontmatter || line == "" || strings.HasPrefix(line, "%%") {
			continue
		}

		return strings.HasPrefix(line, "flowchart") || strings.HasPrefix(line, "graph")
	}

	return false
}

// Parse lists the nodes and named edges of a flowchart.
func Parse(source string) *Map {
	m := &Map{}
	frontmatter := false

	for i, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)

		// YAML frontmatter, delimited by `---` lines at the very top.
		if line == "---" && (i == 0 || frontmatter) {
			frontmatter = !frontmatter
			continue
		}

		if frontmatter || line == "" || strings.HasPrefix(line, "flowchart") || strings.HasPrefix(line, "graph") {
			continue
		}

		if rest, ok := strings.CutPrefix(line, "subgraph"); ok {
			m.addNode(nodeID.FindString(strings.TrimSpace(rest)))
			continue
		}

		if strings.HasPrefix(line, "%%") || slices.ContainsFunc(skipped, func(keyword string) bool {
			return line == keyword || strings.HasPrefix(line, keyword+" ")
		}) {
			continue
		}

		for statement := range strings.SplitSeq(line, ";") {
			m.statement(statement)
		}
	}

	return m
}

// statement reads one `a --> b & c --> d` chain.
func (m *Map) statement(statement string) {
	// Blank out text that could contain arrow-like characters. The id comes
	// before any shape or label, so nothing needed is lost.
	statement = quoted.ReplaceAllString(statement, `""`)
	statement = blankShapes(statement)
	statement = pipeLabel.ReplaceAllString(statement, " ")

	for _, match := range edge.FindAllStringSubmatch(statement, -1) {
		if match[1] != "" && !slices.Contains(m.Edges, match[1]) {
			m.Edges = append(m.Edges, match[1])
		}
	}

	for _, group := range edge.Split(statement, -1) {
		for ref := range strings.SplitSeq(group, "&") {
			m.addNode(nodeID.FindString(strings.TrimSpace(ref)))
		}
	}
}

func (m *Map) addNode(id string) {
	if id != "" && !slices.Contains(m.Nodes, id) {
		m.Nodes = append(m.Nodes, id)
	}
}

// blankShapes empties every bracketed node shape, `[...]`, `(...)`, `{...}`,
// leaving the brackets, so label text can't be mistaken for an arrow.
func blankShapes(s string) string {
	var b strings.Builder

	depth := 0

	for _, r := range s {
		switch r {
		case '[', '(', '{':
			if depth == 0 {
				b.WriteRune(r)
			}

			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}

			if depth == 0 {
				b.WriteRune(r)
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}

	return b.String()
}
