package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzStrictDecodeRequest(f *testing.F) {
	for _, seed := range []string{
		`{"version":1,"request_id":"fuzz","method":"NodeInfo"}`,
		`{"version":1,"body":{"nested":[{"key":"value"}]}}`,
		`{"version":1,"version":2}`,
		`{"body":{"x":1,"\u0078":2}}`,
		`null`, `[]`, `{`, `{} {}`, `{"version":18446744073709551615}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Bound the campaign's per-input memory/CPU independently of the wire cap.
		if len(data) > 64<<10 {
			return
		}
		var request Request
		if err := StrictDecode(data, &request); err != nil {
			return
		}
		if !json.Valid(data) || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			t.Fatal("invalid structured document accepted")
		}
		canonical, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var roundTrip Request
		if err := StrictDecode(canonical, &roundTrip); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(roundTrip)
		if err != nil || !bytes.Equal(encoded, canonical) {
			t.Fatal("accepted request changed during canonical round trip")
		}
		duplicate := append(append([]byte(nil), canonical[:len(canonical)-1]...), []byte(`,"version":0}`)...)
		if err := StrictDecode(duplicate, &roundTrip); err == nil {
			t.Fatal("duplicate field accepted")
		}
	})
}
