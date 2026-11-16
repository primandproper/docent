package tour

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/diag"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

type corpusEntry struct {
	File    string `json:"file"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	SHA     string `json:"sha"`
	License string `json:"license"`
	URL     string `json:"url"`
}

// TestCorpus is the M0 superset gate: real-world CodeTour files, byte for byte
// as published, must parse and pass the schema with no errors. testdata/corpus
// holds them, and manifest.json records where each came from.
func TestCorpus(T *testing.T) {
	T.Parallel()

	dir := filepath.Join("testdata", "corpus")

	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	must.NoError(T, err)

	var manifest []corpusEntry
	must.NoError(T, json.Unmarshal(raw, &manifest))
	must.GreaterEq(T, 20, len(manifest), must.Sprint("the gate needs at least 20 real tours"))

	onDisk, err := filepath.Glob(filepath.Join(dir, "*"+Extension))
	must.NoError(T, err)
	must.Eq(T, len(manifest), len(onDisk), must.Sprint("every corpus file needs a manifest entry, and the reverse"))

	for _, entry := range manifest {
		T.Run(entry.File, func(t *testing.T) {
			t.Parallel()

			data, readErr := os.ReadFile(filepath.Join(dir, entry.File))
			must.NoError(t, readErr)

			parsed, diags, parseErr := Parse(entry.File, data)
			must.NoError(t, parseErr)
			must.NotNil(t, parsed)

			for _, d := range diags {
				test.NotEq(t, diag.SeverityError, d.Severity, test.Sprintf("%s %s: %s", d.Code, d.Pointer, d.Message))
			}

			test.False(t, parsed.IsDocent())
			test.NotEq(t, "", parsed.Title)
			test.NonZero(t, len(parsed.Steps))

			// Nothing the typed model doesn't know about may be lost.
			var want, got any
			must.NoError(t, json.Unmarshal(trimBOM(data), &want))

			out, marshalErr := parsed.Raw.Format()
			must.NoError(t, marshalErr)
			must.NoError(t, json.Unmarshal(out, &got))
			test.Eq(t, want, got)
		})
	}
}

func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}

	return data
}
