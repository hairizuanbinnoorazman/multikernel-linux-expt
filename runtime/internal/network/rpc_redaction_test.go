package network

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNetworkWireDecodeErrorsOmitSuppliedSecrets(t *testing.T) {
	secret := strings.Repeat("synthetic-private-network-credential-348bdc", 256)
	for name, payload := range map[string]string{
		"unknown":   `{"version":1,"method":"ADD","` + secret + `":true}`,
		"duplicate": `{"version":1,"method":"ATTACH","` + secret + `":true,"` + secret + `":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			done := make(chan struct{})
			go func() { defer close(done); (&Server{FrameIOTimeout: time.Second}).handle(context.Background(), server) }()
			if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Write([]byte(payload + "\n")); err != nil {
				t.Fatal(err)
			}
			data, err := bufio.NewReader(client).ReadBytes('\n')
			if err != nil {
				t.Fatal(err)
			}
			var response Response
			if err = json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if response.Error == nil || response.Error.Code != "INVALID_ARGUMENT" || len(data) > 256 || bytes.Contains(data, []byte("synthetic-private-network-credential-348bdc")) {
				t.Fatalf("unsafe network decode reply (%d bytes)", len(data))
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not close")
			}
		})
	}
}
