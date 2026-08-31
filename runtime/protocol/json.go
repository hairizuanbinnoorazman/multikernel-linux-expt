package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// StrictDecode decodes exactly one JSON value, rejects unknown struct fields,
// and rejects duplicate object names at every nesting level. The standard Go
// decoder otherwise accepts duplicate names and silently keeps the last value.
func StrictDecode(data []byte, dst any) error {
	if err := rejectDuplicateNames(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
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
			return nil
		}
		if err != nil {
			return err
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
					return fmt.Errorf("duplicate JSON object name %q", value)
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
