// Package orderedjson parses JSON into a tree that keeps object key order and
// the exact text of numbers, so a document can be rewritten without losing
// anything the typed model doesn't know about.
package orderedjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The kinds a Value can hold.
const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindArray
	KindObject
)

var (
	// ErrTrailingData is returned when a document has content after its first value.
	ErrTrailingData = errors.New("unexpected content after the top-level JSON value")

	utf8BOM = []byte{0xEF, 0xBB, 0xBF}
)

type (
	// Kind identifies which JSON type a Value holds.
	Kind int

	// Member is one key/value pair of an object, in document order.
	Member struct {
		_     struct{}
		Value *Value
		Key   string
	}

	// Value is one node of a parsed JSON document.
	Value struct {
		_       struct{}
		Str     string
		Num     json.Number
		Members []Member
		Elems   []*Value
		Kind    Kind
		Bool    bool
	}
)

// String names the kind the way JSON Schema does.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBool:
		return "boolean"
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindArray:
		return "array"
	case KindObject:
		return "object"
	default:
		return "unknown"
	}
}

// Parse reads a single JSON document. A leading UTF-8 byte order mark is
// tolerated, because editors on Windows write one and the files are still JSON
// to every tool their authors used.
func Parse(data []byte) (*Value, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	dec.UseNumber()

	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}

	if _, err = dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("reading past the top-level value: %w", err)
		}

		return nil, ErrTrailingData
	}

	return v, nil
}

func parseValue(dec *json.Decoder) (*Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("reading JSON token: %w", err)
	}

	switch t := tok.(type) {
	case nil:
		return &Value{Kind: KindNull}, nil
	case bool:
		return &Value{Kind: KindBool, Bool: t}, nil
	case json.Number:
		return &Value{Kind: KindNumber, Num: t}, nil
	case string:
		return &Value{Kind: KindString, Str: t}, nil
	case json.Delim:
		switch t {
		case '{':
			return parseObject(dec)
		case '[':
			return parseArray(dec)
		}
	}

	return nil, fmt.Errorf("unexpected JSON token %v", tok)
}

func parseObject(dec *json.Decoder) (*Value, error) {
	obj := &Value{Kind: KindObject}

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading object key: %w", err)
		}

		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected object key %v", tok)
		}

		val, err := parseValue(dec)
		if err != nil {
			return nil, err
		}

		obj.Members = append(obj.Members, Member{Key: key, Value: val})
	}

	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("reading end of object: %w", err)
	}

	return obj, nil
}

func parseArray(dec *json.Decoder) (*Value, error) {
	arr := &Value{Kind: KindArray}

	for dec.More() {
		val, err := parseValue(dec)
		if err != nil {
			return nil, err
		}

		arr.Elems = append(arr.Elems, val)
	}

	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("reading end of array: %w", err)
	}

	return arr, nil
}

// Get returns the value of an object member, or nil when v isn't an object or
// has no such key. When a key is repeated the last one wins, as it does in
// every JSON parser a tour has already been through.
func (v *Value) Get(key string) *Value {
	if v == nil || v.Kind != KindObject {
		return nil
	}

	for i := len(v.Members) - 1; i >= 0; i-- {
		if v.Members[i].Key == key {
			return v.Members[i].Value
		}
	}

	return nil
}

// Interface converts the tree to the plain Go values encoding/json would have
// produced with UseNumber, which is the shape JSON Schema validation wants.
func (v *Value) Interface() any {
	if v == nil {
		return nil
	}

	switch v.Kind {
	case KindBool:
		return v.Bool
	case KindNumber:
		return v.Num
	case KindString:
		return v.Str
	case KindArray:
		out := make([]any, len(v.Elems))
		for i, e := range v.Elems {
			out[i] = e.Interface()
		}

		return out
	case KindObject:
		out := make(map[string]any, len(v.Members))
		for _, m := range v.Members {
			out[m.Key] = m.Value.Interface()
		}

		return out
	default:
		return nil
	}
}

// Marshal writes the tree back out with the given indent, keeping key order.
// Empty arrays and objects stay on one line.
func (v *Value) Marshal(indent string) ([]byte, error) {
	var buf bytes.Buffer
	if err := v.write(&buf, indent, 0); err != nil {
		return nil, err
	}

	buf.WriteByte('\n')

	return buf.Bytes(), nil
}

func (v *Value) write(buf *bytes.Buffer, indent string, depth int) error {
	switch v.Kind {
	case KindNull:
		buf.WriteString("null")
	case KindBool:
		buf.WriteString(strconv.FormatBool(v.Bool))
	case KindNumber:
		buf.WriteString(v.Num.String())
	case KindString:
		return writeString(buf, v.Str)
	case KindArray:
		if len(v.Elems) == 0 {
			buf.WriteString("[]")
			return nil
		}

		buf.WriteByte('[')

		for i, e := range v.Elems {
			if i > 0 {
				buf.WriteByte(',')
			}

			newline(buf, indent, depth+1)

			if err := e.write(buf, indent, depth+1); err != nil {
				return err
			}
		}

		newline(buf, indent, depth)
		buf.WriteByte(']')
	case KindObject:
		if len(v.Members) == 0 {
			buf.WriteString("{}")
			return nil
		}

		buf.WriteByte('{')

		for i, m := range v.Members {
			if i > 0 {
				buf.WriteByte(',')
			}

			newline(buf, indent, depth+1)

			if err := writeString(buf, m.Key); err != nil {
				return err
			}

			buf.WriteString(": ")

			if err := m.Value.write(buf, indent, depth+1); err != nil {
				return err
			}
		}

		newline(buf, indent, depth)
		buf.WriteByte('}')
	default:
		return fmt.Errorf("unknown JSON kind %d", v.Kind)
	}

	return nil
}

func newline(buf *bytes.Buffer, indent string, depth int) {
	buf.WriteByte('\n')
	buf.WriteString(strings.Repeat(indent, depth))
}

func writeString(buf *bytes.Buffer, s string) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encoding JSON string: %w", err)
	}

	// Encode appends a newline that isn't part of the value.
	buf.Truncate(buf.Len() - 1)

	return nil
}
