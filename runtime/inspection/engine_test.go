package inspection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEngineInspectionStrictOwnerWire(t *testing.T) {
	q := Request{Version: 1, Namespace: "moby", ContainerID: "container", SandboxID: "box", Generation: strings.Repeat("a", 32)}
	data, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeRequest(data); err != nil || got != q {
		t.Fatal("valid owner refused")
	}
	for _, bad := range []string{`null`, `{}`, string(data) + string(data), strings.Replace(string(data), `"version":1`, `"Version":1`, 1), strings.Replace(string(data), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(data), `"namespace":"moby"`, `"namespace":null`, 1), strings.Replace(string(data), `"container_id":"container"`, `"container_id":"../private"`, 1), strings.Replace(string(data), `}`, `,"token":"private"}`, 1)} {
		if _, err := DecodeRequest([]byte(bad)); err == nil {
			t.Fatal("ambiguous/unsafe request accepted")
		}
	}
}
