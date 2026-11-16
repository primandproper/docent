package links

import (
	"testing"

	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/shoenig/test"
)

func TestVars_Lookup(T *testing.T) {
	T.Parallel()

	defined := []tour.Var{{Name: "env", Default: "staging", Choices: []string{"staging", "prod"}}, {Name: "bare"}}

	T.Run("sources are consulted in priority order", func(t *testing.T) {
		t.Parallel()

		cfg := &workspace.Config{
			Workspace: workspace.File{Vars: map[string]string{"env": "from-workspace"}},
			User:      workspace.File{Vars: map[string]string{"env": "from-user"}},
		}

		lookup := func(v *Vars) (string, string) {
			value, source, _ := v.Lookup("env", defined)
			return value, source
		}

		v := NewVars(&workspace.Config{}, nil)
		value, source := lookup(v)
		test.Eq(t, "staging", value)
		test.Eq(t, SourceTour, source)

		v = NewVars(&workspace.Config{User: cfg.User}, nil)
		value, source = lookup(v)
		test.Eq(t, "from-user", value)
		test.Eq(t, SourceUser, source)

		v = NewVars(cfg, nil)
		value, source = lookup(v)
		test.Eq(t, "from-workspace", value)
		test.Eq(t, SourceWorkspace, source)

		v.Pick("env", "picked")
		value, source = lookup(v)
		test.Eq(t, "picked", value)
		test.Eq(t, SourcePicker, source)

		v.Pick("env", "")
		value, _ = lookup(v)
		test.Eq(t, "from-workspace", value)

		v = NewVars(cfg, map[string]string{"env": "flagged"})
		v.Pick("env", "picked")
		value, source = lookup(v)
		test.Eq(t, "flagged", value)
		test.Eq(t, SourceFlag, source)
	})

	T.Run("a variable with no value anywhere is not found", func(t *testing.T) {
		t.Parallel()

		v := NewVars(nil, nil)

		_, _, ok := v.Lookup("bare", defined)
		test.False(t, ok)

		_, _, ok = v.Lookup("undeclared", defined)
		test.False(t, ok)
	})
}

func TestVars_Expand(T *testing.T) {
	T.Parallel()

	defined := []tour.Var{{Name: "env", Default: "staging"}}
	vars := NewVars(&workspace.Config{Workspace: workspace.File{Vars: map[string]string{"grafana": "https://grafana.example.com"}}}, nil)

	T.Run("fills in variables and reports the host", func(t *testing.T) {
		t.Parallel()

		card := vars.Expand(&tour.Link{Kind: "grafana", Title: "p99", URL: "{{grafana}}/d/abc?var-env={{ env }}&again={{env}}", Description: "d"}, defined)
		test.Eq(t, Card{
			Kind: "grafana", Title: "p99", Description: "d", Missing: []string{},
			URL:  "https://grafana.example.com/d/abc?var-env=staging&again=staging",
			Host: "grafana.example.com",
		}, card)
	})

	T.Run("a missing variable disables the card and names it", func(t *testing.T) {
		t.Parallel()

		card := vars.Expand(&tour.Link{URL: "{{logs}}/search?q={{query}}&e={{env}}&l={{logs}}"}, defined)
		test.Eq(t, []string{"logs", "query"}, card.Missing)
		test.Eq(t, "No value for {{logs}}, {{query}}", card.Problem)
		test.Eq(t, "", card.Host)
	})

	T.Run("only web links are allowed", func(t *testing.T) {
		t.Parallel()

		for _, internal := range []string{"docent:step/write-path", "docent:tour/other.tour#start"} {
			card := vars.Expand(&tour.Link{URL: internal}, defined)
			test.Eq(t, "", card.Problem, test.Sprint(internal))
			test.Eq(t, "", card.Host)
		}

		for _, bad := range []string{"docent:", "docent:step/", "docent:shell/rm", "file:///etc/passwd", "javascript:alert(1)", "vscode://file/x", "/relative", "https://", "{{env}}://x"} {
			card := vars.Expand(&tour.Link{URL: bad}, defined)
			test.Eq(t, "Only http, https and docent links can be opened", card.Problem, test.Sprint(bad))
		}

		test.Eq(t, "", vars.Expand(&tour.Link{URL: "http://localhost:3000/d"}, defined).Problem)
	})
}

func TestNames(T *testing.T) {
	T.Parallel()

	T.Run("lists distinct variables in order", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, []string{"grafana", "env"}, Names("{{grafana}}/d?e={{env}}&f={{ grafana }}"))
		test.Len(t, 0, Names("https://example.com/{not}/{{}}"))
	})
}
