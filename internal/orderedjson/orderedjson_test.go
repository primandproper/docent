package orderedjson

import (
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestParse(T *testing.T) {
	T.Parallel()

	T.Run("keeps key order and number text through a round trip", func(t *testing.T) {
		t.Parallel()

		const doc = `{
  "zebra": 1.50,
  "apple": [
    true,
    null,
    "<b>&</b>"
  ],
  "empty": {},
  "none": [],
  "nested": {
    "b": 1e3,
    "a": "é"
  }
}
`

		v, err := Parse([]byte(doc))
		must.NoError(t, err)

		out, err := v.Marshal("  ")
		must.NoError(t, err)
		test.Eq(t, doc, string(out))
	})

	T.Run("tolerates a byte order mark", func(t *testing.T) {
		t.Parallel()

		v, err := Parse(append([]byte{0xEF, 0xBB, 0xBF}, `{"a":1}`...))
		must.NoError(t, err)
		test.Eq(t, "1", v.Get("a").Num.String())
	})

	T.Run("last duplicate key wins on lookup and both survive", func(t *testing.T) {
		t.Parallel()

		v, err := Parse([]byte(`{"a":1,"a":2}`))
		must.NoError(t, err)
		test.Eq(t, "2", v.Get("a").Num.String())
		test.Len(t, 2, v.Members)
	})

	T.Run("rejects trailing content", func(t *testing.T) {
		t.Parallel()

		_, err := Parse([]byte(`{} {}`))
		test.ErrorIs(t, err, ErrTrailingData)
	})

	T.Run("rejects invalid JSON", func(t *testing.T) {
		t.Parallel()

		_, err := Parse([]byte(`{"a": // nope`))
		test.Error(t, err)
	})
}

func TestValue_Get(T *testing.T) {
	T.Parallel()

	T.Run("is nil safe", func(t *testing.T) {
		t.Parallel()

		var v *Value
		test.Nil(t, v.Get("a"))
		test.Nil(t, (&Value{Kind: KindArray}).Get("a"))
	})
}

func TestValue_Interface(T *testing.T) {
	T.Parallel()

	T.Run("produces plain values", func(t *testing.T) {
		t.Parallel()

		v, err := Parse([]byte(`{"a":[1,"x",false,null]}`))
		must.NoError(t, err)

		m, ok := v.Interface().(map[string]any)
		must.True(t, ok)

		arr, ok := m["a"].([]any)
		must.True(t, ok)
		test.Len(t, 4, arr)
		test.Nil(t, arr[3])
	})
}
