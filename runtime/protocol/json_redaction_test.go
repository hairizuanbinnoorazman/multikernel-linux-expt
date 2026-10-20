package protocol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const decoderSyntheticSecret = "synthetic-credential-must-not-enter-errors-739ba22c"

type credentialDecoder struct{}

func (*credentialDecoder) UnmarshalJSON([]byte) error { return errors.New(decoderSyntheticSecret) }

func TestStrictDecodeErrorsNeverCopyDocumentSecrets(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(decoderSyntheticSecret))
	for _, secret := range []string{decoderSyntheticSecret, encoded, strings.Repeat(decoderSyntheticSecret, 256)} {
		key, _ := json.Marshal(secret)
		for _, test := range []struct{ name, payload, want string }{
			{"unknown", `{` + string(key) + `:true}`, "unknown field in JSON document"},
			{"duplicate", `{"nested":{` + string(key) + `:1,` + string(key) + `:2}}`, "duplicate JSON object name"},
			{"overflow", `{"count":1234567890123456789012345678901234567890}`, "invalid JSON value type"},
			{"wrong type", `{"count":` + string(key) + `}`, "invalid JSON value type"},
			{"syntax", `{"nested":{` + string(key) + `:`, "invalid JSON syntax"},
			{"trailing", `{"count":1} {` + string(key) + `:true}`, "multiple JSON values"},
		} {
			t.Run(test.name, func(t *testing.T) {
				var target struct {
					Count  uint8          `json:"count"`
					Nested map[string]any `json:"nested"`
				}
				err := StrictDecode([]byte(test.payload), &target)
				if err == nil || err.Error() != test.want || len(err.Error()) > 128 || strings.Contains(err.Error(), decoderSyntheticSecret) || strings.Contains(err.Error(), encoded) {
					t.Fatalf("unsafe decoder error classification: %v", err)
				}
			})
		}
	}
	var custom credentialDecoder
	if err := StrictDecode([]byte(`{}`), &custom); err == nil || err.Error() != "invalid JSON document" {
		t.Fatalf("custom decoder error escaped: %v", err)
	}
	// Unknown fields remain rejected even when their spelling is escaped in JSON.
	var empty struct{}
	escaped := strings.ReplaceAll(`{"`+decoderSyntheticSecret+`":1}`, "s", `\u0073`)
	if err := StrictDecode([]byte(escaped), &empty); err == nil || err.Error() != "unknown field in JSON document" {
		t.Fatalf("escaped-key error: %v", err)
	}
}
