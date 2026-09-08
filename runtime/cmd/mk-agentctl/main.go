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
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
)

const agentExchangeTimeout = 30 * time.Second

func exchangePayload(c net.Conn, payload []byte) (agent.Reply, error) {
	return exchangePayloadWithTimeout(c, payload, agentExchangeTimeout)
}

func exchangePayloadWithTimeout(c net.Conn, payload []byte, timeout time.Duration) (agent.Reply, error) {
	if timeout <= 0 {
		return agent.Reply{}, errors.New("agent exchange timeout must be positive")
	}
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return agent.Reply{}, err
	}
	defer c.SetDeadline(time.Time{})
	var header [4]byte
	var err error
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

func exchange(c net.Conn, e agent.Envelope, key []byte) (agent.Reply, error) {
	agent.Sign(&e, key)
	payload, err := json.Marshal(e)
	if err != nil {
		return agent.Reply{}, err
	}
	return exchangePayload(c, payload)
}

func body(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func terminateRelay(command *exec.Cmd) error {
	if command == nil {
		return nil
	}
	if command.Process != nil {
		pid := command.Process.Pid
		err := error(nil)
		if command.SysProcAttr != nil && command.SysProcAttr.Setpgid {
			err = syscall.Kill(-pid, syscall.SIGKILL)
		} else {
			err = command.Process.Kill()
		}
		if err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	err := command.Wait()
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return nil
	}
	return err
}

func main() {
	if buildinfo.PrintRequested(os.Stdout, "mk-agentctl", os.Args[1:]) {
		return
	}
	var id, generation, token, bundle, unixSocket, relay string
	var port uint
	var authMatrix bool
	flag.StringVar(&id, "sandbox-id", "", "sandbox ID")
	flag.StringVar(&generation, "generation", "", "generation")
	flag.StringVar(&token, "token-hex", "", "authentication token")
	flag.StringVar(&bundle, "bundle", "/bundle", "child OCI bundle")
	flag.UintVar(&port, "port", 0, "primary listening port")
	flag.StringVar(&unixSocket, "unix-socket", "", "primary relay Unix socket")
	flag.StringVar(&relay, "relay", "", "relay binary used for reconnect qualification")
	flag.BoolVar(&authMatrix, "auth-matrix", false, "run the safe live authentication and framing matrix")
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
	defer func() { _ = conn.Close() }()
	results := map[string]any{}
	sequence := uint64(0)
	call := func(method string, request any) (agent.Reply, error) {
		sequence++
		return exchange(conn, agent.Envelope{Version: 1, SandboxID: id, Generation: generation, Endpoint: uint32(port), Sequence: sequence, Method: method, Body: body(request)}, key)
	}
	if authMatrix {
		negative := map[string]string{}
		cases := []struct {
			name   string
			modify func(*agent.Envelope)
		}{
			{"wrong_protocol", func(envelope *agent.Envelope) { envelope.Version = 2 }},
			{"wrong_sandbox", func(envelope *agent.Envelope) { envelope.SandboxID = "wrong-sandbox" }},
			{"stale_generation", func(envelope *agent.Envelope) { envelope.Generation = "ffffffffffffffffffffffffffffffff" }},
			{"wrong_endpoint", func(envelope *agent.Envelope) { envelope.Endpoint++ }},
		}
		for _, test := range cases {
			envelope := agent.Envelope{Version: 1, SandboxID: id, Generation: generation, Endpoint: uint32(port), Sequence: 1, Method: "Capabilities", Body: body(map[string]any{})}
			test.modify(&envelope)
			_, callErr := exchange(conn, envelope, key)
			if callErr == nil {
				fmt.Fprintln(os.Stderr, test.name, "was accepted")
				os.Exit(1)
			}
			negative[test.name] = callErr.Error()
		}
		envelope := agent.Envelope{Version: 1, SandboxID: id, Generation: generation, Endpoint: uint32(port), Sequence: 1, Method: "Capabilities", Body: body(map[string]any{}), MAC: "00"}
		raw, _ := json.Marshal(envelope)
		if _, callErr := exchangePayload(conn, raw); callErr == nil {
			fmt.Fprintln(os.Stderr, "invalid MAC was accepted")
			os.Exit(1)
		} else {
			negative["invalid_mac"] = callErr.Error()
		}
		if _, callErr := exchangePayload(conn, []byte("{")); callErr == nil {
			fmt.Fprintln(os.Stderr, "malformed message was accepted")
			os.Exit(1)
		} else {
			negative["malformed_message"] = callErr.Error()
		}
		capabilities, callErr := call("Capabilities", map[string]any{})
		if callErr != nil {
			fmt.Fprintln(os.Stderr, "Capabilities:", callErr)
			os.Exit(1)
		}
		results["Capabilities"] = capabilities.Body
		replay := agent.Envelope{Version: 1, SandboxID: id, Generation: generation, Endpoint: uint32(port), Sequence: 1, Method: "Capabilities", Body: body(map[string]any{})}
		if _, callErr = exchange(conn, replay, key); callErr == nil {
			fmt.Fprintln(os.Stderr, "replayed sequence was accepted")
			os.Exit(1)
		} else {
			negative["replay"] = callErr.Error()
		}
		future := agent.Envelope{Version: 1, SandboxID: id, Generation: generation, Endpoint: uint32(port), Sequence: 3, Method: "Capabilities", Body: body(map[string]any{})}
		if _, callErr = exchange(conn, future, key); callErr != nil {
			fmt.Fprintln(os.Stderr, "future sequence:", callErr)
			os.Exit(1)
		}
		late := future
		late.Sequence = 2
		if _, callErr = exchange(conn, late, key); callErr == nil {
			fmt.Fprintln(os.Stderr, "out-of-order sequence was accepted")
			os.Exit(1)
		} else {
			negative["out_of_order"] = callErr.Error()
		}
		sequence = 3
		results["AuthenticationMatrix"] = negative
	}
	methods := []struct {
		name  string
		value any
	}{
		{"CreateProcess", map[string]any{"ID": "p1", "Bundle": bundle}},
		{"StartProcess", map[string]any{"ID": "p1"}},
	}
	if !authMatrix {
		methods = append([]struct {
			name  string
			value any
		}{{"Capabilities", map[string]any{}}}, methods...)
	}
	for _, method := range methods {
		reply, e := call(method.name, method.value)
		if e != nil {
			fmt.Fprintln(os.Stderr, method.name+":", e)
			os.Exit(1)
		}
		results[method.name] = reply.Body
	}
	var ownedRelay *exec.Cmd
	reconnect := func() {
		if relay == "" {
			fmt.Fprintln(os.Stderr, "--relay is required for reconnect qualification")
			os.Exit(2)
		}
		_ = conn.Close()
		if ownedRelay != nil {
			if err = terminateRelay(ownedRelay); err != nil {
				fmt.Fprintln(os.Stderr, "stop reconnect relay:", err)
				os.Exit(1)
			}
		}
		_ = os.Remove(unixSocket)
		ownedRelay = exec.Command(relay, "server", fmt.Sprint(port), unixSocket)
		ownedRelay.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err = ownedRelay.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "start reconnect relay:", err)
			os.Exit(1)
		}
		for attempt := 0; attempt < 300; attempt++ {
			conn, err = net.Dial("unix", unixSocket)
			if err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Fprintln(os.Stderr, "reconnect:", err)
		os.Exit(1)
	}
	if authMatrix {
		var header [4]byte
		if err = conn.SetDeadline(time.Now().Add(agentExchangeTimeout)); err != nil {
			fmt.Fprintln(os.Stderr, "oversized frame deadline:", err)
			os.Exit(1)
		}
		binary.BigEndian.PutUint32(header[:], 1<<20+1)
		if _, err = conn.Write(header[:]); err != nil {
			fmt.Fprintln(os.Stderr, "oversized frame write:", err)
			os.Exit(1)
		}
		if _, err = io.ReadFull(conn, header[:]); err != nil {
			fmt.Fprintln(os.Stderr, "oversized frame reply:", err)
			os.Exit(1)
		}
		replyBytes := make([]byte, binary.BigEndian.Uint32(header[:]))
		if _, err = io.ReadFull(conn, replyBytes); err != nil || !strings.Contains(string(replyBytes), "INVALID_ARGUMENT") {
			fmt.Fprintln(os.Stderr, "oversized frame response:", err, string(replyBytes))
			os.Exit(1)
		}
		reconnect()
		results["OversizedFrame"] = "rejected-and-session-closed"
		_ = conn.Close()
		reconnect()
		results["Reconnect"] = "transport-disconnect-preserved-process-and-sequence"
	}
	var stdout, stderr []byte
	var stdoutOffset, stderrOffset uint64
	for {
		reply, callErr := call("ReadProcessOutput", map[string]any{"id": "p1", "stdout_offset": stdoutOffset, "stderr_offset": stderrOffset, "limit": uint64(64 << 10)})
		if callErr != nil {
			fmt.Fprintln(os.Stderr, "ReadProcessOutput:", callErr)
			os.Exit(1)
		}
		raw, _ := json.Marshal(reply.Body)
		var output struct {
			Stdout       []byte `json:"stdout"`
			Stderr       []byte `json:"stderr"`
			StdoutOffset uint64 `json:"stdout_offset"`
			StderrOffset uint64 `json:"stderr_offset"`
			Status       string `json:"status"`
		}
		if err = json.Unmarshal(raw, &output); err != nil {
			fmt.Fprintln(os.Stderr, "ReadProcessOutput:", err)
			os.Exit(1)
		}
		stdout = append(stdout, output.Stdout...)
		stderr = append(stderr, output.Stderr...)
		stdoutOffset, stderrOffset = output.StdoutOffset, output.StderrOffset
		if output.Status == "STOPPED" && len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			break
		}
		if len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitReply, err := call("WaitProcess", map[string]any{"id": "p1"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "WaitProcess:", err)
		os.Exit(1)
	}
	waitRaw, _ := json.Marshal(waitReply.Body)
	var waitBody map[string]any
	if err = json.Unmarshal(waitRaw, &waitBody); err != nil {
		fmt.Fprintln(os.Stderr, "WaitProcess:", err)
		os.Exit(1)
	}
	waitBody["stdout"] = string(stdout)
	waitBody["stderr"] = string(stderr)
	results["WaitProcess"] = waitBody
	for _, method := range []struct {
		name  string
		value any
	}{{"DeleteProcess", map[string]any{"id": "p1"}}, {"Shutdown", map[string]any{}}} {
		reply, callErr := call(method.name, method.value)
		if callErr != nil {
			fmt.Fprintln(os.Stderr, method.name+":", callErr)
			os.Exit(1)
		}
		results[method.name] = reply.Body
	}
	if ownedRelay != nil {
		// The server relay may return to accept after the child endpoint closes.
		// Shutdown has already received its final reply, so terminate this
		// controller-owned helper explicitly instead of waiting indefinitely.
		if err = terminateRelay(ownedRelay); err != nil {
			fmt.Fprintln(os.Stderr, "stop final relay:", err)
			os.Exit(1)
		}
	}
	b, _ := json.MarshalIndent(results, "", "  ")
	os.Stdout.Write(append(b, '\n'))
}
