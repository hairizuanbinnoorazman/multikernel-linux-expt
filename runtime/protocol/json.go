package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

// StrictDecode decodes exactly one JSON value, rejects unknown struct fields,
// and rejects duplicate object names at every nesting level. The standard Go
// decoder otherwise accepts duplicate names and silently keeps the last value.
// Errors contain fixed classifications only: callers put them on RPC wires and
// in diagnostics, so field names, values and custom decoder errors must not be
// copied from a potentially credential-bearing document.
func StrictDecode(data []byte, dst any) error {
	target := reflect.TypeOf(dst)
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target != nil && target.Kind() == reflect.Struct {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return errors.New("JSON object required for a structured document")
		}
	}
	if err := rejectDuplicateNames(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return errors.New("unknown field in JSON document")
		}
		var valueError *json.UnmarshalTypeError
		if errors.As(err, &valueError) {
			return errors.New("invalid JSON value type")
		}
		return errors.New("invalid JSON document")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func rejectDuplicateNames(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	type scope struct {
		object    bool
		expectKey bool
		names     map[string]struct{}
	}
	var stack []scope
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if len(stack) != 0 {
				return errors.New("invalid JSON syntax")
			}
			return nil
		}
		if err != nil {
			return errors.New("invalid JSON syntax")
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, scope{object: true, expectKey: true, names: make(map[string]struct{})})
			case '[':
				stack = append(stack, scope{})
			case '}', ']':
				if len(stack) == 0 {
					return errors.New("unbalanced JSON delimiter")
				}
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].object {
					stack[len(stack)-1].expectKey = true
				}
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expectKey {
				current := &stack[len(stack)-1]
				if _, exists := current.names[value]; exists {
					return errors.New("duplicate JSON object name")
				}
				current.names[value] = struct{}{}
				current.expectKey = false
			} else if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].expectKey = true
			}
		default:
			if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].expectKey = true
			}
		}
	}
}
