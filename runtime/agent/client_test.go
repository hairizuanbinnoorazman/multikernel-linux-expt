package agent

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
)

func TestClientRejectsMismatchedReplyBinding(t *testing.T) {
	tests := []struct {
		name    string
		version int
		seq     uint64
		want    string
	}{
		{"version", 2, 1, "version mismatch"},
		{"sequence", 1, 99, "sequence mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			client := &Client{conn: clientConn, token: []byte("01234567890123456789012345678901"), sandboxID: "box", generation: "0123456789abcdef0123456789abcdef", endpoint: 7001}
			go func() {
				var header [4]byte
				if _, err := io.ReadFull(serverConn, header[:]); err != nil {
					return
				}
				request := make([]byte, binary.BigEndian.Uint32(header[:]))
				if _, err := io.ReadFull(serverConn, request); err != nil {
					return
				}
				response, _ := json.Marshal(Reply{Version: test.version, Sequence: test.seq, Body: map[string]any{"protocol": 1}})
				binary.BigEndian.PutUint32(header[:], uint32(len(response)))
				_, _ = serverConn.Write(header[:])
				_, _ = serverConn.Write(response)
			}()
			if err := client.Call("Capabilities", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Call() error = %v, want %q", err, test.want)
			}
		})
	}
}
