package orderedjson

import (
	"strings"
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
  "apple": [true, null, "<b>&</b>"],
  "empty": {},
  "none": [],
  "nested": {
    "leaf": { "b": 1e3, "a": "é" },
    "list": [
      { "line": 1, "character": 2 },
      ["x"]
    ]
  }
}
`

		v, err := Parse([]byte(doc))
		must.NoError(t, err)

		out, err := v.Format()
		must.NoError(t, err)
		test.Eq(t, doc, string(out))
	})

	T.Run("a container that doesn't fit on the line is broken up", func(t *testing.T) {
		t.Parallel()

		long := strings.Repeat("x", 60)

		v, err := Parse([]byte(`{"a": {"first": "` + long + `", "second": "` + long + `"}, "b": ["` + long + `", "` + long + `"]}`))
		must.NoError(t, err)

		out, err := v.Format()
		must.NoError(t, err)
		test.Eq(t, "{\n  \"a\": {\n    \"first\": \""+long+"\",\n    \"second\": \""+long+"\"\n  },\n  \"b\": [\n    \""+long+"\",\n    \""+long+"\"\n  ]\n}\n", string(out))
	})

	T.Run("formatting is stable", func(t *testing.T) {
		t.Parallel()

		v, err := Parse([]byte(`{"steps":[{"id":"a","selection":{"start":{"line":1,"character":1},"end":{"line":2,"character":3}}}]}`))
		must.NoError(t, err)

		once, err := v.Format()
		must.NoError(t, err)

		again, err := Parse(once)
		must.NoError(t, err)

		twice, err := again.Format()
		must.NoError(t, err)
		test.Eq(t, string(once), string(twice))
		test.StrContains(t, string(once), `"start": { "line": 1, "character": 1 },`)
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

func TestValue_Edit(T *testing.T) {
	T.Parallel()

	T.Run("sets in place, appends new keys and deletes", func(t *testing.T) {
		t.Parallel()

		v, err := Parse([]byte(`{"a":1,"b":2,"c":3}`))
		must.NoError(t, err)

		v.Set("b", String("two"))
		v.Set("d", Object(Member{Key: "x", Value: String("y")}))
		v.Delete("a")

		out, err := v.Format()
		must.NoError(t, err)
		test.Eq(t, "{\n  \"b\": \"two\",\n  \"c\": 3,\n  \"d\": { \"x\": \"y\" }\n}\n", string(out))

		// Neither does anything to a value that isn't an object.
		String("s").Set("k", String("v"))
		String("s").Delete("k")
	})
}
