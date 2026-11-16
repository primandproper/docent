package tour

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/orderedjson"
	"github.com/primandproper/docent/schema"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.TourV1))
	if err != nil {
		return nil, fmt.Errorf("decoding the embedded tour schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat()

	if err = c.AddResource(schema.TourV1ID, doc); err != nil {
		return nil, fmt.Errorf("registering the embedded tour schema: %w", err)
	}

	compiled, err := c.Compile(schema.TourV1ID)
	if err != nil {
		return nil, fmt.Errorf("compiling the embedded tour schema: %w", err)
	}

	return compiled, nil
})

// validateSchema checks a document against the tour schema and returns one
// diagnostic per leaf violation.
func validateSchema(file string, doc *orderedjson.Value) ([]diag.Diagnostic, error) {
	compiled, err := compiledSchema()
	if err != nil {
		return nil, err
	}

	err = compiled.Validate(doc.Interface())
	if err == nil {
		return nil, nil
	}

	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return nil, fmt.Errorf("validating against the tour schema: %w", err)
	}

	printer := message.NewPrinter(language.English)

	var out []diag.Diagnostic

	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, cause := range e.Causes {
				walk(cause)
			}

			return
		}

		tokens := make([]any, len(e.InstanceLocation))
		for i, tok := range e.InstanceLocation {
			tokens[i] = tok
		}

		out = append(out, diag.Diagnostic{
			File:     file,
			Pointer:  diag.Pointer(tokens...),
			Step:     stepFromLocation(doc, e.InstanceLocation),
			Severity: diag.SeverityError,
			Code:     diag.CodeSchema,
			Message:  e.ErrorKind.LocalizedString(printer),
			Hint:     "Run `docent schema` to see the tour JSON Schema.",
		})
	}
	walk(verr)

	return out, nil
}

// stepFromLocation labels the step an instance location falls inside, if any.
func stepFromLocation(doc *orderedjson.Value, location []string) string {
	if len(location) < 2 || location[0] != "steps" {
		return ""
	}

	index, err := strconv.Atoi(location[1])
	if err != nil {
		return ""
	}

	steps := doc.Get("steps")
	if steps != nil && steps.Kind == orderedjson.KindArray && index < len(steps.Elems) {
		if id := steps.Elems[index].Get("id"); id != nil && id.Kind == orderedjson.KindString && id.Str != "" {
			return id.Str
		}
	}

	return stepNumber(index)
}

func stepNumber(index int) string {
	return "#" + strconv.Itoa(index+1)
}
