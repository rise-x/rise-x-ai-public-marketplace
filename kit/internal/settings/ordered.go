package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// object is a JSON object that remembers the order its keys were written in.
// encoding/json unmarshals an object into a map, which sorts its keys on the
// way out; settings.json belongs to the partner, so rewriting one key must
// leave every other key exactly where Claude Code (or their editor) put it.
type object struct {
	keys   []string
	values map[string]json.RawMessage
}

func newObject() *object {
	return &object{values: map[string]json.RawMessage{}}
}

func (o *object) get(key string) (json.RawMessage, bool) {
	v, ok := o.values[key]
	return v, ok
}

// set replaces key in place, or appends it when it is new.
func (o *object) set(key string, value json.RawMessage) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// decodeObject reads a JSON object with json.Decoder's token stream, keeping
// each value's raw bytes so nothing but the key we touch is re-encoded.
func decodeObject(data []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected a JSON object, got %v", tok)
	}

	o := newObject()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		o.set(key, raw)
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing data after the JSON object")
	}
	return o, nil
}

// marshal renders the object compactly, in key order.
func (o *object) marshal() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := encodeJSON(k)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		var compact bytes.Buffer
		if err := json.Compact(&compact, o.values[k]); err != nil {
			return nil, err
		}
		b.Write(compact.Bytes())
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// indent renders the object as Claude Code writes settings.json: two-space
// indent, one key per line, in the original order, with no HTML escaping.
func (o *object) indent() ([]byte, error) {
	if len(o.keys) == 0 {
		return []byte("{}\n"), nil
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, k := range o.keys {
		key, err := encodeJSON(k)
		if err != nil {
			return nil, err
		}
		b.WriteString("  ")
		b.Write(key)
		b.WriteString(": ")
		var value bytes.Buffer
		if err := json.Indent(&value, o.values[k], "  ", "  "); err != nil {
			return nil, err
		}
		b.Write(value.Bytes())
		if i < len(o.keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

// encodeJSON marshals v without encoding/json's default HTML escaping, which
// would rewrite "&&", ">" and "<" inside hook commands and API key helpers.
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
