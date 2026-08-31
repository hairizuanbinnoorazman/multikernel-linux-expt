package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
)

func exchange(c net.Conn, e agent.Envelope, key []byte) (agent.Reply, error) {
	agent.Sign(&e, key)
	payload, err := json.Marshal(e)
	if err != nil {
		return agent.Reply{}, err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err = c.Write(header[:]); err != nil {
		return agent.Reply{}, err
	}
	if _, err = c.Write(payload); err != nil {
		return agent.Reply{}, err
	}
	if _, err = io.ReadFull(c, header[:]); err != nil {
		return agent.Reply{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > 1<<20 {
		return agent.Reply{}, errors.New("response too large")
	}
	b := make([]byte, size)
	if _, err = io.ReadFull(c, b); err != nil {
		return agent.Reply{}, err
	}
	var reply agent.Reply
	if err = json.Unmarshal(b, &reply); err != nil {
		return reply, err
	}
	if reply.Error != "" {
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}

func body(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func main() {
	var id, generation, token, bundle, unixSocket string
	var port uint
	flag.StringVar(&id, "sandbox-id", "", "sandbox ID")
	flag.StringVar(&generation, "generation", "", "generation")
	flag.StringVar(&token, "token-hex", "", "authentication token")
	flag.StringVar(&bundle, "bundle", "/bundle", "child OCI bundle")
	flag.UintVar(&port, "port", 0, "primary listening port")
	flag.StringVar(&unixSocket, "unix-socket", "", "primary relay Unix socket")
	flag.Parse()
	key, err := hex.DecodeString(token)
	if err != nil || len(key) != 32 || id == "" || generation == "" || port < 1024 {
		fmt.Fprintln(os.Stderr, "identity, port, and valid token are required")
		os.Exit(2)
	}
	var conn net.Conn
	if unixSocket == "" {
		fmt.Fprintln(os.Stderr, "--unix-socket is required")
		os.Exit(2)
	}
	for attempt := 0; attempt < 300; attempt++ {
		conn, err = net.Dial("unix", unixSocket)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()
	methods := []struct {
		name  string
		value any
	}{
		{"Capabilities", map[string]any{}},
		{"CreateProcess", map[string]any{"ID": "p1", "Bundle": bundle}},
		{"StartProcess", map[string]any{"ID": "p1"}},
		{"WaitProcess", map[string]any{"ID": "p1"}},
		{"DeleteProcess", map[string]any{"ID": "p1"}},
		{"Shutdown", map[string]any{}},
	}
	results := map[string]any{}
	for i, method := range methods {
		reply, e := exchange(conn, agent.Envelope{Version: 1, SandboxID: id, Generation: generation, Endpoint: uint32(port), Sequence: uint64(i + 1), Method: method.name, Body: body(method.value)}, key)
		if e != nil {
			fmt.Fprintln(os.Stderr, method.name+":", e)
			os.Exit(1)
		}
		results[method.name] = reply.Body
	}
	b, _ := json.MarshalIndent(results, "", "  ")
	os.Stdout.Write(append(b, '\n'))
}
