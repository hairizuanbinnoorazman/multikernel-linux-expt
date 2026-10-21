package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

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
	// Error is the local diagnostic. Server replies retain their typed cause
	// through setError; a diagnostic string alone serializes as INTERNAL.
	Error string `json:"-"`
	// Failure is the structured error decoded from a remote reply.
	Failure       *protocol.Error `json:"-"`
	cause         error
	authenticated bool
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
		wire.Error = safeAgentError(r.Sequence, r.cause)
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
	if (wire.Body == nil) == (wire.Error == nil) {
		return errors.New("agent reply must contain exactly one body or error")
	}
	r.Version, r.Sequence, r.Body, r.Error = wire.Version, wire.Sequence, wire.Body, ""
	r.cause = nil
	r.Failure = wire.Error
	if wire.Error != nil {
		r.Error = wire.Error.Code + ": " + wire.Error.Message
	}
	return nil
}

type Server struct {
	FrameIOTimeout        time.Duration
	Manager               *Manager
	SandboxID, Generation string
	Bundle                string
	Endpoint              uint32
	Token                 []byte
	BeforeShutdown        func() error
	mu                    sync.Mutex
	last                  uint64
	quiesceMu             sync.Mutex
	quiesced              bool
}

func (e Envelope) canonical() []byte { e.MAC = ""; b, _ := json.Marshal(e); return b }
func Sign(e *Envelope, token []byte) {
	m := hmac.New(sha256.New, token)
	m.Write(e.canonical())
	e.MAC = hex.EncodeToString(m.Sum(nil))
}
func (s *Server) verify(e Envelope) error {
	if len(s.Token) == 0 {
		return errorf(unauthenticated, "agent authentication is not configured")
	}
	if e.Version != 1 {
		return errorf(unsupported, "unsupported version")
	}
	if e.SandboxID != s.SandboxID || e.Generation != s.Generation || e.Endpoint != s.Endpoint {
		return errorf(unauthenticated, "stale or wrong identity")
	}
	got, x := hex.DecodeString(e.MAC)
	if x != nil {
		return errorf(unauthenticated, "invalid authentication")
	}
	m := hmac.New(sha256.New, s.Token)
	m.Write(e.canonical())
	if !hmac.Equal(got, m.Sum(nil)) {
		return errorf(unauthenticated, "invalid authentication")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Sequence <= s.last {
		return errorf(unauthenticated, "replayed sequence")
	}
	s.last = e.Sequence
	return nil
}
func decode(b []byte, v any) error {
	if err := protocol.StrictDecode(b, v); err != nil {
		return &operationError{kind: invalidArgument, cause: err}
	}
	return nil
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
		"protocol":          1,
		"protocol_features": []string{"signal-operation-id-v1", "stdin-offset-v1", "two-phase-shutdown-v1"},
		"oci_features": []string{"argv", "environment", "cwd", "split-stdio", "exit-code", "stdin", "attach", "terminal", "terminal-resize",
			"no-new-privileges", "rlimits", "linux-capabilities", "hostname", "masked-paths", "readonly-paths", "readonly-root", "standard-mounts", "readonly-bind-inputs-v1"},
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
	return s.DispatchContext(context.Background(), e)
}

func (s *Server) quiesce() error {
	s.quiesceMu.Lock()
	defer s.quiesceMu.Unlock()
	if s.quiesced {
		return nil
	}
	if !s.Manager.Quiescent() {
		return errorf(failedPrecondition, "managed processes are still running")
	}
	if s.BeforeShutdown != nil {
		if err := s.BeforeShutdown(); err != nil {
			return errors.New("storage quiescence failed")
		}
	}
	s.quiesced = true
	return nil
}

func (s *Server) isQuiesced() bool {
	s.quiesceMu.Lock()
	defer s.quiesceMu.Unlock()
	return s.quiesced
}

func (s *Server) DispatchContext(ctx context.Context, e Envelope) Reply {
	r := Reply{Version: 1, Sequence: e.Sequence}
	if x := s.verify(e); x != nil {
		r.setError(x)
		return r
	}
	r.authenticated = true
	if s.isQuiesced() && e.Method != "Capabilities" && e.Method != "Quiesce" && e.Method != "Shutdown" {
		r.setError(errorf(failedPrecondition, "agent is quiesced"))
		return r
	}
	switch e.Method {
	case "Capabilities":
		r.Body = capabilityReport()
	case "CreateProcess":
		var q struct{ ID, Bundle string }
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if s.Bundle != "" && q.Bundle != s.Bundle {
			r.setError(errorf(invalidArgument, "bundle path does not match the runtime-owned bundle"))
		} else if x = s.Manager.Create(q.ID, q.Bundle); x != nil {
			r.setError(x)
		}
	case "ExecProcess":
		var q struct {
			ID       string      `json:"id"`
			ParentID string      `json:"parent_id"`
			Spec     ProcessSpec `json:"spec"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.Exec(q.ID, q.ParentID, q.Spec); x != nil {
			r.setError(x)
		}
	case "StartProcess":
		var q struct {
			ID          string `json:"id"`
			Width       uint32 `json:"width,omitempty"`
			Height      uint32 `json:"height,omitempty"`
			InitialSize bool   `json:"initial_size,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.StartWithSize(q.ID, q.Width, q.Height, q.InitialSize); x != nil {
			r.setError(x)
		}
	case "SignalProcess":
		var q struct {
			ID          string `json:"ID"`
			Signal      string `json:"Signal"`
			OperationID string `json:"operation_id,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if sig, x := SignalNumber(q.Signal); x != nil {
			r.setError(x)
		} else if q.OperationID != "" {
			if x = s.Manager.SignalOnce(q.ID, sig, q.OperationID); x != nil {
				r.setError(x)
			}
		} else if x = s.Manager.Signal(q.ID, sig); x != nil {
			r.setError(x)
		}
	case "AcknowledgeSignal":
		var q struct {
			ID          string `json:"ID"`
			OperationID string `json:"operation_id"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.AcknowledgeSignal(q.ID, q.OperationID); x != nil {
			r.setError(x)
		}
	case "ResizeProcess":
		var q struct {
			ID     string `json:"id"`
			Width  uint32 `json:"width"`
			Height uint32 `json:"height"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.Resize(q.ID, q.Width, q.Height); x != nil {
			r.setError(x)
		}
	case "WriteProcess":
		var q struct {
			ID     string  `json:"id"`
			Offset *uint64 `json:"offset,omitempty"`
			Data   []byte  `json:"data"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if q.Offset == nil {
			if x = s.Manager.Write(q.ID, q.Data); x != nil {
				r.setError(x)
			}
		} else if offset, x := s.Manager.WriteAt(q.ID, *q.Offset, q.Data); x != nil {
			r.setError(x)
		} else {
			r.Body = map[string]uint64{"offset": offset}
		}
	case "CloseProcessStdin":
		var q struct {
			ID string `json:"id"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.CloseStdin(q.ID); x != nil {
			r.setError(x)
		}
	case "ReadProcessOutput":
		var q struct {
			ID           string `json:"id"`
			StdoutOffset uint64 `json:"stdout_offset"`
			StderrOffset uint64 `json:"stderr_offset"`
			Limit        uint64 `json:"limit"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if stdout, stderr, nextStdout, nextStderr, status, x := s.Manager.ReadOutput(q.ID, q.StdoutOffset, q.StderrOffset, q.Limit); x != nil {
			r.setError(x)
		} else {
			state, _ := s.Manager.State(q.ID)
			r.Body = map[string]any{"stdout": stdout, "stderr": stderr, "stdout_offset": nextStdout, "stderr_offset": nextStderr, "status": status, "stdout_truncated": state.StdoutTruncated, "stderr_truncated": state.StderrTruncated}
		}
	case "WaitProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if st, x := s.Manager.Wait(q.ID); x != nil {
			r.setError(x)
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
			r.setError(x)
		} else if st, x := s.Manager.State(q.ID); x != nil {
			r.setError(x)
		} else {
			st.Stdout = ""
			st.Stderr = ""
			r.Body = st
		}
	case "StatsProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if stats, x := s.Manager.Stats(q.ID); x != nil {
			r.setError(x)
		} else {
			r.Body = stats
		}
	case "ConfigureNetwork":
		var q NetworkConfig
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.ConfigureNetworkContext(ctx, q); x != nil {
			r.setError(x)
		}
	case "ExchangeNetwork":
		var q struct {
			Packet []byte `json:"packet,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if packet, x := s.Manager.ExchangeNetwork(q.Packet); x != nil {
			r.setError(x)
		} else {
			r.Body = map[string][]byte{"packet": packet}
		}
	case "CloseNetwork":
		if x := s.Manager.CloseNetworkContext(ctx); x != nil {
			r.setError(x)
		}
	case "DeleteProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.setError(x)
		} else if x = s.Manager.Delete(q.ID); x != nil {
			r.setError(x)
		}
	case "Quiesce", "Shutdown":
		if x := s.quiesce(); x != nil {
			r.setError(x)
		} else {
			r.Body = map[string]string{"status": "quiesced"}
		}
	default:
		r.setError(errorf(unsupported, "unsupported method"))
	}
	return r
}
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	stopCancellation := protocol.CloseOnContext(ctx, l)
	defer stopCancellation()
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
	stopCancellation := protocol.CloseOnContext(ctx, c)
	defer stopCancellation()
	budget := protocol.FrameIOTimeout(s.FrameIOTimeout)
	authenticationDeadline := time.Now().Add(budget)
	authenticated := false
	for {
		var header [4]byte
		idleDeadline := authenticationDeadline
		if authenticated {
			idleDeadline = time.Time{}
		}
		if e := c.SetReadDeadline(idleDeadline); e != nil {
			if errors.Is(e, io.ErrClosedPipe) || errors.Is(e, net.ErrClosed) {
				return nil
			}
			return e
		}
		if _, e := io.ReadFull(c, header[:1]); e != nil {
			if errors.Is(e, io.EOF) {
				return nil
			}
			return e
		}
		deadline := time.Now().Add(budget)
		if !authenticated && authenticationDeadline.Before(deadline) {
			deadline = authenticationDeadline
		}
		if e := c.SetReadDeadline(deadline); e != nil {
			return e
		}
		if _, e := io.ReadFull(c, header[1:]); e != nil {
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
			reply.setError(errorf(frameTooLarge, "frame too large"))
			closeAfterReply = true
		} else {
			b := make([]byte, size)
			if _, e := io.ReadFull(c, b); e != nil {
				return e
			}
			if e := c.SetReadDeadline(time.Time{}); e != nil {
				return e
			}
			if decodeErr := decode(b, &env); decodeErr != nil {
				reply.setError(decodeErr)
			} else {
				reply = s.DispatchContext(ctx, env)
				authenticated = authenticated || reply.authenticated
			}
		}
		b, e := json.Marshal(reply)
		if e != nil {
			return e
		}
		binary.BigEndian.PutUint32(header[:], uint32(len(b)))
		if e = c.SetWriteDeadline(time.Now().Add(budget)); e != nil {
			return e
		}
		if e = protocol.WriteFull(c, header[:]); e != nil {
			return e
		}
		if e = protocol.WriteFull(c, b); e != nil {
			return e
		}
		if e = c.SetWriteDeadline(time.Time{}); e != nil {
			// The peer may close immediately after receiving the complete reply.
			// Clearing a deadline then reports a closed pipe on transports such
			// as net.Pipe; the response was delivered successfully.
			if errors.Is(e, io.ErrClosedPipe) || errors.Is(e, net.ErrClosed) {
				return nil
			}
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
