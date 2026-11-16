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

// lineWidth is the column a container must fit within to be written on one line.
const lineWidth = 100

// Format writes the tree back out in Docent's canonical layout: two-space
// indent, key order as found, and a container on one line when it holds only
// scalars and fits. It changes no values.
func (v *Value) Format() ([]byte, error) {
	var buf bytes.Buffer
	if err := v.write(&buf, 0, true); err != nil {
		return nil, err
	}

	buf.WriteByte('\n')

	return buf.Bytes(), nil
}

// inline renders a container of scalars on one line, or reports that it has
// nested containers and can't be.
func (v *Value) inline() (string, bool) {
	var buf bytes.Buffer

	switch v.Kind {
	case KindArray:
		buf.WriteByte('[')

		for i, e := range v.Elems {
			if e.Kind == KindArray || e.Kind == KindObject {
				return "", false
			}

			if i > 0 {
				buf.WriteString(", ")
			}

			if err := e.write(&buf, 0, false); err != nil {
				return "", false
			}
		}

		buf.WriteByte(']')
	case KindObject:
		buf.WriteString("{ ")

		for i, m := range v.Members {
			if m.Value.Kind == KindArray || m.Value.Kind == KindObject {
				return "", false
			}

			if i > 0 {
				buf.WriteString(", ")
			}

			if err := writeString(&buf, m.Key); err != nil {
				return "", false
			}

			buf.WriteString(": ")

			if err := m.Value.write(&buf, 0, false); err != nil {
				return "", false
			}
		}

		buf.WriteString(" }")
	default:
		return "", false
	}

	return buf.String(), true
}

// write renders a value at a depth. column is how far along the line it
// starts, which decides whether a small container still fits inline.
func (v *Value) write(buf *bytes.Buffer, depth int, root bool) error {
	switch v.Kind {
	case KindNull:
		buf.WriteString("null")
	case KindBool:
		buf.WriteString(strconv.FormatBool(v.Bool))
	case KindNumber:
		buf.WriteString(v.Num.String())
	case KindString:
		return writeString(buf, v.Str)
	case KindArray, KindObject:
		return v.writeContainer(buf, depth, root)
	default:
		return fmt.Errorf("unknown JSON kind %d", v.Kind)
	}

	return nil
}

func (v *Value) writeContainer(buf *bytes.Buffer, depth int, root bool) error {
	if v.Kind == KindArray && len(v.Elems) == 0 {
		buf.WriteString("[]")
		return nil
	}

	if v.Kind == KindObject && len(v.Members) == 0 {
		buf.WriteString("{}")
		return nil
	}

	if !root {
		// What is already on this line (indent, key) counts against the width.
		used := buf.Len() - (bytes.LastIndexByte(buf.Bytes(), '\n') + 1)
		if text, ok := v.inline(); ok && used+len(text) < lineWidth {
			buf.WriteString(text)
			return nil
		}
	}

	if v.Kind == KindArray {
		buf.WriteByte('[')

		for i, e := range v.Elems {
			if i > 0 {
				buf.WriteByte(',')
			}

			newline(buf, depth+1)

			if err := e.write(buf, depth+1, false); err != nil {
				return err
			}
		}

		newline(buf, depth)
		buf.WriteByte(']')

		return nil
	}

	buf.WriteByte('{')

	for i, m := range v.Members {
		if i > 0 {
			buf.WriteByte(',')
		}

		newline(buf, depth+1)

		if err := writeString(buf, m.Key); err != nil {
			return err
		}

		buf.WriteString(": ")

		if err := m.Value.write(buf, depth+1, false); err != nil {
			return err
		}
	}

	newline(buf, depth)
	buf.WriteByte('}')

	return nil
}

func newline(buf *bytes.Buffer, depth int) {
	buf.WriteByte('\n')
	buf.WriteString(strings.Repeat("  ", depth))
}

// String returns a string value.
func String(s string) *Value {
	return &Value{Kind: KindString, Str: s}
}

// Object returns an object with the given members, in order.
func Object(members ...Member) *Value {
	return &Value{Kind: KindObject, Members: members}
}

// Set replaces an object member's value, or appends the member when the key
// is new. It does nothing to a value that isn't an object.
func (v *Value) Set(key string, value *Value) {
	if v == nil || v.Kind != KindObject {
		return
	}

	for i := range v.Members {
		if v.Members[i].Key == key {
			v.Members[i].Value = value
			return
		}
	}

	v.Members = append(v.Members, Member{Key: key, Value: value})
}

// Delete removes every member with the key from an object.
func (v *Value) Delete(key string) {
	if v == nil || v.Kind != KindObject {
		return
	}

	kept := v.Members[:0]

	for _, m := range v.Members {
		if m.Key != key {
			kept = append(kept, m)
		}
	}

	v.Members = kept
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
