//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// The containerd bridge owns one reconnectable task connection. Read-only
// inspection clients need independent, non-replayed connections to the current
// worker; otherwise they sit in the public listener backlog until containerd
// disconnects. No additional listener or credential is introduced.
type shimInspectionRouter struct {
	listener      net.Listener
	tasks         chan net.Conn
	slots         chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	clients       map[net.Conn]struct{}
	workerAddress string
	workerContext context.Context
	workerCancel  context.CancelFunc
}

func newShimInspectionRouter(listener net.Listener) *shimInspectionRouter {
	ctx, cancel := context.WithCancel(context.Background())
	r := &shimInspectionRouter{listener: listener, tasks: make(chan net.Conn, 1), slots: make(chan struct{}, 8), ctx: ctx, cancel: cancel, clients: map[net.Conn]struct{}{}}
	go r.accept()
	return r
}

func (r *shimInspectionRouter) close() {
	r.cancel()
	_ = r.listener.Close()
	r.mu.Lock()
	if r.workerCancel != nil {
		r.workerCancel()
	}
	for c := range r.clients {
		_ = c.Close()
	}
	r.mu.Unlock()
	for {
		select {
		case c := <-r.tasks:
			_ = c.Close()
		default:
			return
		}
	}
}

func (r *shimInspectionRouter) setWorker(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.workerCancel != nil {
		r.workerCancel()
	}
	r.workerAddress = address
	r.workerContext, r.workerCancel = context.WithCancel(r.ctx)
	if address == "" {
		r.workerCancel()
	}
}

func (r *shimInspectionRouter) taskConnection(timeout time.Duration) (net.Conn, error) {
	ctx := r.ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	select {
	case c := <-r.tasks:
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *shimInspectionRouter) accept() {
	for {
		c, err := r.listener.Accept()
		if err != nil {
			r.cancel()
			return
		}
		select {
		case r.slots <- struct{}{}:
		default:
			_ = c.Close()
			continue
		}
		r.mu.Lock()
		if r.ctx.Err() != nil {
			r.mu.Unlock()
			_ = c.Close()
			<-r.slots
			return
		}
		r.clients[c] = struct{}{}
		r.mu.Unlock()
		go r.route(c)
	}
}

type prefixedShimConnection struct {
	net.Conn
	prefix io.Reader
}

func (c *prefixedShimConnection) Read(p []byte) (int, error) { return c.prefix.Read(p) }

func (r *shimInspectionRouter) route(c net.Conn) {
	handedOff := false
	defer func() {
		r.mu.Lock()
		delete(r.clients, c)
		r.mu.Unlock()
		if !handedOff {
			_ = c.Close()
		}
		<-r.slots
	}()
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	// Authenticate the original public peer, before proxying through our own UID.
	if _, _, err := ttrpc.UnixSocketRequireSameUser().Handshake(ctx, c); err != nil {
		return
	}
	deadline, _ := ctx.Deadline()
	if c.SetDeadline(deadline) != nil {
		return
	}
	frame, err := readTTRPCFrame(c)
	if err != nil || frame.typeID != ttrpcRequest || frame.flags != 0 || frame.streamID == 0 || frame.streamID%2 == 0 {
		return
	}
	var request ttrpc.Request
	if proto.Unmarshal(frame.encoded[ttrpcHeaderLength:], &request) != nil {
		return
	}
	if request.Service == inspection.Service {
		r.inspect(ctx, c, frame.streamID, &request)
		return
	}
	// Only containerd task RPCs can take the reconnectable/replayable bridge.
	if request.Service != "containerd.task.v2.Task" {
		return
	}
	if !stop() || ctx.Err() != nil || c.SetDeadline(time.Time{}) != nil {
		return
	}
	connection := &prefixedShimConnection{Conn: c, prefix: io.MultiReader(bytes.NewReader(frame.encoded), c)}
	// Enqueue while holding the same lock used by close, so shutdown cannot leave
	// an unowned queued descriptor behind. Never queue a second reconnect client.
	r.mu.Lock()
	if r.ctx.Err() == nil {
		select {
		case r.tasks <- connection:
			handedOff = true
		default:
		}
	}
	r.mu.Unlock()
}

func (r *shimInspectionRouter) inspect(parent context.Context, c net.Conn, stream uint32, wire *ttrpc.Request) {
	reply := func(report inspection.Report, err error) {
		response := &ttrpc.Response{Status: status.New(codes.OK, "").Proto()}
		if err != nil {
			code := status.Code(err)
			switch code {
			case codes.InvalidArgument, codes.FailedPrecondition, codes.Unimplemented, codes.DeadlineExceeded, codes.Canceled, codes.Unavailable:
			default:
				code = codes.Unavailable
			}
			response.Status = status.New(code, "Docker engine inspection unavailable").Proto()
		} else {
			data, e := json.Marshal(report)
			if e != nil {
				return
			}
			response.Payload, e = proto.Marshal(wrapperspb.Bytes(data))
			if e != nil {
				return
			}
		}
		payload, e := proto.Marshal(response)
		if e != nil {
			return
		}
		encoded := make([]byte, ttrpcHeaderLength+len(payload))
		binary.BigEndian.PutUint32(encoded, uint32(len(payload)))
		binary.BigEndian.PutUint32(encoded[4:], stream)
		encoded[8] = ttrpcResponse
		copy(encoded[ttrpcHeaderLength:], payload)
		_ = writeFull(c, encoded)
	}
	fail := func(code codes.Code) {
		reply(inspection.Report{}, status.Error(code, "Docker engine inspection unavailable"))
	}
	var body wrapperspb.BytesValue
	if wire.Method != inspection.Method || len(wire.Payload) > inspection.MaxRequest+16 || len(wire.Metadata) != 0 || proto.Unmarshal(wire.Payload, &body) != nil {
		fail(codes.InvalidArgument)
		return
	}
	owner, err := inspection.DecodeRequest(body.Value)
	if err != nil {
		fail(codes.InvalidArgument)
		return
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if wire.TimeoutNano > 0 {
		var timedCancel context.CancelFunc
		ctx, timedCancel = context.WithTimeout(ctx, time.Duration(wire.TimeoutNano))
		defer timedCancel()
	}
	r.mu.Lock()
	address, epoch := r.workerAddress, r.workerContext
	r.mu.Unlock()
	if address == "" || epoch == nil || epoch.Err() != nil {
		fail(codes.Unavailable)
		return
	}
	// This cancellation is tied to one captured worker epoch. Never redial or
	// transfer a queued observation to a replacement worker.
	stop := context.AfterFunc(epoch, cancel)
	defer stop()
	backend, err := (&net.Dialer{}).DialContext(ctx, "unix", address)
	if err != nil {
		fail(codes.Unavailable)
		return
	}
	defer backend.Close()
	closeBackend := context.AfterFunc(ctx, func() { _ = backend.Close() })
	defer closeBackend()
	if _, _, err = ttrpc.UnixSocketRequireSameUser().Handshake(ctx, backend); err != nil {
		fail(codes.Unavailable)
		return
	}
	client := ttrpc.NewClient(backend)
	defer client.Close()
	report, err := inspection.Inspect(ctx, client, owner)
	if ctx.Err() != nil || epoch.Err() != nil {
		fail(codes.Unavailable)
		return
	}
	reply(report, err)
}
