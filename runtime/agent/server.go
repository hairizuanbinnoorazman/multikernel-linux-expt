package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var ErrShutdownRequested = errors.New("agent shutdown requested")

type Envelope struct {
	Version    int             `json:"version"`
	SandboxID  string          `json:"sandbox_id"`
	Generation string          `json:"generation"`
	Endpoint   uint32          `json:"endpoint"`
	Sequence   uint64          `json:"sequence"`
	Method     string          `json:"method"`
	Body       json.RawMessage `json:"body,omitempty"`
	MAC        string          `json:"mac"`
}
type Reply struct {
	Version  int    `json:"version"`
	Sequence uint64 `json:"sequence"`
	Body     any    `json:"-"`
	Error    string `json:"-"`
}

func (r Reply) MarshalJSON() ([]byte, error) {
	type wireReply struct {
		Version  int             `json:"version"`
		Sequence uint64          `json:"sequence"`
		Body     any             `json:"body,omitempty"`
		Error    *protocol.Error `json:"error,omitempty"`
	}
	wire := wireReply{Version: r.Version, Sequence: r.Sequence, Body: r.Body}
	if r.Error != "" {
		wire.Body = nil
		wire.Error = safeAgentError(r.Sequence, r.Error)
	} else if wire.Body == nil {
		wire.Body = map[string]any{}
	}
	return json.Marshal(wire)
}

func (r *Reply) UnmarshalJSON(data []byte) error {
	type wireReply struct {
		Version  int             `json:"version"`
		Sequence uint64          `json:"sequence"`
		Body     any             `json:"body,omitempty"`
		Error    *protocol.Error `json:"error,omitempty"`
	}
	var wire wireReply
	if err := protocol.StrictDecode(data, &wire); err != nil {
		return err
	}
	r.Version, r.Sequence, r.Body = wire.Version, wire.Sequence, wire.Body
	if wire.Error != nil {
		r.Error = wire.Error.Code + ": " + wire.Error.Message
	}
	return nil
}

func safeAgentError(sequence uint64, raw string) *protocol.Error {
	code, message := "INTERNAL", "agent operation failed"
	switch {
	case strings.Contains(raw, "frame too large"):
		code, message = "INVALID_ARGUMENT", "agent frame is too large"
	case strings.Contains(raw, "authentication"), strings.Contains(raw, "identity"), strings.Contains(raw, "replayed sequence"):
		code, message = "UNAUTHENTICATED", "agent authentication failed"
	case strings.Contains(raw, "unsupported"), strings.Contains(raw, "not implemented"):
		code, message = "UNSUPPORTED", "agent operation is unsupported"
	case strings.Contains(raw, "not found"):
		code, message = "NOT_FOUND", "managed process was not found"
	case strings.Contains(raw, "exists"):
		code, message = "ALREADY_EXISTS", "managed process already exists"
	case strings.Contains(raw, "not running"), strings.Contains(raw, "not started"), strings.Contains(raw, "still running"), strings.Contains(raw, "not created"), strings.Contains(raw, "stdin is closed"):
		code, message = "FAILED_PRECONDITION", "managed process is in the wrong state"
	case strings.Contains(raw, "invalid"), strings.Contains(raw, "must"), strings.Contains(raw, "required"), strings.Contains(raw, "exceed"), strings.Contains(raw, "limit"), strings.Contains(raw, "absolute"), strings.Contains(raw, "multiple JSON"), strings.Contains(raw, "unknown field"):
		code, message = "INVALID_ARGUMENT", "invalid agent request"
	}
	return &protocol.Error{Code: code, Message: message, OperationID: fmt.Sprintf("agent-%d", sequence), Retryable: false}
}

type Server struct {
	Manager               *Manager
	SandboxID, Generation string
	Bundle                string
	Endpoint              uint32
	Token                 []byte
	BeforeShutdown        func() error
	mu                    sync.Mutex
	last                  uint64
}

func (e Envelope) canonical() []byte { e.MAC = ""; b, _ := json.Marshal(e); return b }
func Sign(e *Envelope, token []byte) {
	m := hmac.New(sha256.New, token)
	m.Write(e.canonical())
	e.MAC = hex.EncodeToString(m.Sum(nil))
}
func (s *Server) verify(e Envelope) error {
	if e.Version != 1 {
		return errors.New("unsupported version")
	}
	if e.SandboxID != s.SandboxID || e.Generation != s.Generation || e.Endpoint != s.Endpoint {
		return errors.New("stale or wrong identity")
	}
	got, x := hex.DecodeString(e.MAC)
	if x != nil {
		return errors.New("invalid authentication")
	}
	m := hmac.New(sha256.New, s.Token)
	m.Write(e.canonical())
	if !hmac.Equal(got, m.Sum(nil)) {
		return errors.New("invalid authentication")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Sequence <= s.last {
		return errors.New("replayed sequence")
	}
	s.last = e.Sequence
	return nil
}
func decode(b []byte, v any) error {
	return protocol.StrictDecode(b, v)
}

func capabilityReport() map[string]any {
	read := func(path string) string {
		value, err := os.ReadFile(path)
		if err != nil {
			return "unknown"
		}
		return strings.TrimSpace(string(value))
	}
	status := map[string]string{"CapEff": "unknown", "CapBnd": "unknown", "NoNewPrivs": "unknown"}
	for _, line := range strings.Split(read("/proc/self/status"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if _, ok := status[strings.TrimSuffix(fields[0], ":")]; ok {
				status[strings.TrimSuffix(fields[0], ":")] = fields[1]
			}
		}
	}
	namespaces := make([]string, 0, 8)
	for _, name := range []string{"cgroup", "ipc", "mnt", "net", "pid", "time", "user", "uts"} {
		if _, err := os.Lstat("/proc/self/ns/" + name); err == nil {
			namespaces = append(namespaces, name)
		}
	}
	sort.Strings(namespaces)
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	return map[string]any{
		"protocol": 1,
		"oci_features": []string{"argv", "environment", "cwd", "split-stdio", "exit-code", "stdin", "attach", "terminal", "terminal-resize",
			"no-new-privileges", "rlimits", "linux-capabilities", "hostname", "masked-paths", "readonly-paths", "readonly-root", "standard-mounts"},
		"kernel": map[string]any{
			"architecture": runtime.GOARCH,
			"release":      read("/proc/sys/kernel/osrelease"),
			"multikernel":  exists("/sys/fs/multikernel"),
			"cgroup_v2":    exists("/sys/fs/cgroup/cgroup.controllers"),
			"mk_transport": exists("/sys/module/mk_transport"),
			"namespaces":   namespaces,
		},
		"agent": map[string]any{
			"uid":                    os.Getuid(),
			"gid":                    os.Getgid(),
			"effective_capabilities": status["CapEff"],
			"bounding_capabilities":  status["CapBnd"],
			"no_new_privileges":      status["NoNewPrivs"],
		},
	}
}
func (s *Server) Dispatch(e Envelope) Reply {
	r := Reply{Version: 1, Sequence: e.Sequence}
	if x := s.verify(e); x != nil {
		r.Error = x.Error()
		return r
	}
	switch e.Method {
	case "Capabilities":
		r.Body = capabilityReport()
	case "CreateProcess":
		var q struct{ ID, Bundle string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if s.Bundle != "" && q.Bundle != s.Bundle {
			r.Error = "bundle path does not match the runtime-owned bundle"
		} else if x = s.Manager.Create(q.ID, q.Bundle); x != nil {
			r.Error = x.Error()
		}
	case "ExecProcess":
		var q struct {
			ID       string      `json:"id"`
			ParentID string      `json:"parent_id"`
			Spec     ProcessSpec `json:"spec"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Exec(q.ID, q.ParentID, q.Spec); x != nil {
			r.Error = x.Error()
		}
	case "StartProcess":
		var q struct {
			ID          string `json:"id"`
			Width       uint32 `json:"width,omitempty"`
			Height      uint32 `json:"height,omitempty"`
			InitialSize bool   `json:"initial_size,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.StartWithSize(q.ID, q.Width, q.Height, q.InitialSize); x != nil {
			r.Error = x.Error()
		}
	case "SignalProcess":
		var q struct{ ID, Signal string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if sig, x := SignalNumber(q.Signal); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Signal(q.ID, sig); x != nil {
			r.Error = x.Error()
		}
	case "ResizeProcess":
		var q struct {
			ID     string `json:"id"`
			Width  uint32 `json:"width"`
			Height uint32 `json:"height"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Resize(q.ID, q.Width, q.Height); x != nil {
			r.Error = x.Error()
		}
	case "WriteProcess":
		var q struct {
			ID   string `json:"id"`
			Data []byte `json:"data"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Write(q.ID, q.Data); x != nil {
			r.Error = x.Error()
		}
	case "CloseProcessStdin":
		var q struct {
			ID string `json:"id"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.CloseStdin(q.ID); x != nil {
			r.Error = x.Error()
		}
	case "ReadProcessOutput":
		var q struct {
			ID           string `json:"id"`
			StdoutOffset uint64 `json:"stdout_offset"`
			StderrOffset uint64 `json:"stderr_offset"`
			Limit        uint64 `json:"limit"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if stdout, stderr, nextStdout, nextStderr, status, x := s.Manager.ReadOutput(q.ID, q.StdoutOffset, q.StderrOffset, q.Limit); x != nil {
			r.Error = x.Error()
		} else {
			state, _ := s.Manager.State(q.ID)
			r.Body = map[string]any{"stdout": stdout, "stderr": stderr, "stdout_offset": nextStdout, "stderr_offset": nextStderr, "status": status, "stdout_truncated": state.StdoutTruncated, "stderr_truncated": state.StderrTruncated}
		}
	case "WaitProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if st, x := s.Manager.Wait(q.ID); x != nil {
			r.Error = x.Error()
		} else {
			st.Stdout = ""
			st.Stderr = ""
			r.Body = st
		}
	case "StateProcess":
		var q struct {
			ID string `json:"id"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if st, x := s.Manager.State(q.ID); x != nil {
			r.Error = x.Error()
		} else {
			st.Stdout = ""
			st.Stderr = ""
			r.Body = st
		}
	case "StatsProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if stats, x := s.Manager.Stats(q.ID); x != nil {
			r.Error = x.Error()
		} else {
			r.Body = stats
		}
	case "ConfigureNetwork":
		var q NetworkConfig
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.ConfigureNetwork(q); x != nil {
			r.Error = x.Error()
		}
	case "ExchangeNetwork":
		var q struct {
			Packet []byte `json:"packet,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if packet, x := s.Manager.ExchangeNetwork(q.Packet); x != nil {
			r.Error = x.Error()
		} else {
			r.Body = map[string][]byte{"packet": packet}
		}
	case "CloseNetwork":
		if x := s.Manager.CloseNetwork(); x != nil {
			r.Error = x.Error()
		}
	case "DeleteProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Delete(q.ID); x != nil {
			r.Error = x.Error()
		}
	case "Shutdown":
		if !s.Manager.Quiescent() {
			r.Error = "managed processes are still running"
		} else if s.BeforeShutdown != nil {
			if err := s.BeforeShutdown(); err != nil {
				r.Error = "storage quiescence failed"
			} else {
				r.Body = map[string]string{"status": "quiesced"}
			}
		} else {
			r.Body = map[string]string{"status": "quiesced"}
		}
	default:
		r.Error = "unsupported method"
	}
	return r
}
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	go func() { <-ctx.Done(); l.Close() }()
	for {
		c, e := l.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		e = s.ServeConn(ctx, c)
		_ = c.Close()
		if errors.Is(e, ErrShutdownRequested) {
			return e
		}
		// A malformed or disconnected session is isolated to that connection.
		// The manager and authenticated sequence remain live for reconnect.
	}
}

func (s *Server) ServeConn(ctx context.Context, c net.Conn) error {
	go func() { <-ctx.Done(); c.Close() }()
	for {
		var header [4]byte
		if _, e := io.ReadFull(c, header[:]); e != nil {
			if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
				return nil
			}
			return e
		}
		size := binary.BigEndian.Uint32(header[:])
		var env Envelope
		reply := Reply{Version: 1}
		closeAfterReply := false
		if size > 1<<20 {
			reply.Error = "frame too large"
			closeAfterReply = true
		} else {
			b := make([]byte, size)
			if _, e := io.ReadFull(c, b); e != nil {
				return e
			}
			if decodeErr := decode(b, &env); decodeErr != nil {
				reply.Error = decodeErr.Error()
			} else {
				reply = s.Dispatch(env)
			}
		}
		b, e := json.Marshal(reply)
		if e != nil {
			return e
		}
		binary.BigEndian.PutUint32(header[:], uint32(len(b)))
		if _, e = c.Write(header[:]); e != nil {
			return e
		}
		if _, e = c.Write(b); e != nil {
			return e
		}
		if env.Method == "Shutdown" && reply.Error == "" {
			return ErrShutdownRequested
		}
		if closeAfterReply {
			return nil
		}
	}
}
