package protocol

import (
	"strings"
	"testing"
)

func TestStrictDecode(t *testing.T) {
	type nested struct {
		Value int `json:"value"`
	}
	type document struct {
		Name   string `json:"name"`
		Nested nested `json:"nested"`
	}

	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "valid", json: `{"name":"one","nested":{"value":2}}`},
		{name: "unknown field", json: `{"name":"one","nested":{"value":2},"extra":true}`, want: "unknown field"},
		{name: "duplicate top level", json: `{"name":"one","name":"two","nested":{"value":2}}`, want: "duplicate JSON object name"},
		{name: "duplicate nested", json: `{"name":"one","nested":{"value":2,"value":3}}`, want: "duplicate JSON object name"},
		{name: "trailing value", json: `{"name":"one","nested":{"value":2}} {}`, want: "multiple JSON values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got document
			err := StrictDecode([]byte(test.json), &got)
			if test.want == "" && err != nil {
				t.Fatalf("StrictDecode() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("StrictDecode() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
