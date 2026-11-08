//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func routerOwner() inspection.Request {
	return inspection.Request{Version: 1, Namespace: "moby", ContainerID: "synthetic", SandboxID: "synthetic", Generation: strings.Repeat("a", 32)}
}
func routerReport(owner inspection.Request) inspection.Report {
	return inspection.Report{Version: 1, Owner: owner, Observation: protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"}}
}
func testInspectionWorker(t *testing.T, observer inspection.Observer) string {
	t.Helper()
	address := filepath.Join(privateTestDirectory(t), "worker.sock")
	l, err := net.Listen("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	server, err := ttrpc.NewServer(ttrpc.WithServerHandshaker(ttrpc.UnixSocketRequireSameUser()))
	if err != nil {
		t.Fatal(err)
	}
	inspection.Register(server, observer)
	go func() { _ = server.Serve(t.Context(), l) }()
	t.Cleanup(func() { _ = server.Close(); _ = l.Close() })
	return address
}
func testInspectionRouter(t *testing.T) (*shimInspectionRouter, string) {
	t.Helper()
	address := filepath.Join(privateTestDirectory(t), "public.sock")
	l, err := net.Listen("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	r := newShimInspectionRouter(l)
	t.Cleanup(r.close)
	return r, address
}
func routerClient(t *testing.T, address string) *ttrpc.Client {
	t.Helper()
	c, err := net.DialTimeout("unix", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := ttrpc.NewClient(c)
	t.Cleanup(func() { _ = client.Close() })
	return client
}
func taskFrame(t *testing.T, service string) []byte {
	t.Helper()
	body, err := proto.Marshal(&ttrpc.Request{Service: service, Method: "Connect"})
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, ttrpcHeaderLength+len(body))
	binary.BigEndian.PutUint32(frame, uint32(len(body)))
	binary.BigEndian.PutUint32(frame[4:], 1)
	frame[8] = ttrpcRequest
	copy(frame[ttrpcHeaderLength:], body)
	return frame
}
func TestInspectionRouterKeepsContainerdConnected(t *testing.T) {
	r, address := testInspectionRouter(t)
	var calls atomic.Int32
	worker := testInspectionWorker(t, func(ctx context.Context, q inspection.Request) (inspection.Report, error) {
		calls.Add(1)
		if q != routerOwner() {
			return inspection.Report{}, status.Error(codes.FailedPrecondition, "private-path token")
		}
		return routerReport(q), nil
	})
	r.setWorker(worker)
	task, err := net.DialTimeout("unix", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer task.Close()
	original := taskFrame(t, "containerd.task.v2.Task")
	if _, err = task.Write(original); err != nil {
		t.Fatal(err)
	}
	bridge, err := r.taskConnection(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	if err = bridge.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	frame, err := readTTRPCFrame(bridge)
	if err != nil || string(frame.encoded) != string(original) {
		t.Fatal("first task frame was not preserved", err)
	}
	for n := 0; n < 3; n++ {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		report, e := inspection.Inspect(ctx, routerClient(t, address), routerOwner())
		cancel()
		if e != nil || report != routerReport(routerOwner()) {
			t.Fatal("inspection queued behind live task client", e)
		}
	}
	// The original task connection and its stream IDs remain independent.
	if _, err = task.Write(original); err != nil {
		t.Fatal(err)
	}
	frame, err = readTTRPCFrame(bridge)
	if err != nil || string(frame.encoded) != string(original) {
		t.Fatal("inspection corrupted task stream", err)
	}
	if calls.Load() != 3 {
		t.Fatal("unexpected replay", calls.Load())
	}
}
func TestInspectionRouterRejectsAndRedacts(t *testing.T) {
	r, address := testInspectionRouter(t)
	var calls atomic.Int32
	worker := testInspectionWorker(t, func(ctx context.Context, q inspection.Request) (inspection.Report, error) {
		calls.Add(1)
		return inspection.Report{}, status.Error(codes.FailedPrecondition, "private-path token")
	})
	r.setWorker(worker)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := inspection.Inspect(ctx, routerClient(t, address), routerOwner())
	if status.Code(err) != codes.FailedPrecondition || strings.Contains(err.Error(), "token") {
		t.Fatal("wrong redacted refusal", err)
	}
	for _, mode := range []string{"method", "payload", "metadata"} {
		request := wrapperspb.Bytes([]byte(`{}`))
		client := routerClient(t, address)
		var response wrapperspb.BytesValue
		method := inspection.Method
		if mode == "method" {
			method = "Create"
		}
		queryCtx := ctx
		if mode == "metadata" {
			queryCtx = ttrpc.WithMetadata(ctx, ttrpc.MD{"foreign": []string{"private"}})
		}
		err = client.Call(queryCtx, inspection.Service, method, request, &response)
		if status.Code(err) != codes.InvalidArgument || len(response.Value) != 0 {
			t.Fatal("invalid inspection accepted", mode, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("malformed requests reached worker")
	}
	r.setWorker("")
	_, err = inspection.Inspect(ctx, routerClient(t, address), routerOwner())
	if status.Code(err) != codes.Unavailable {
		t.Fatal("absent worker accepted", err)
	}
}
func TestInspectionRouterCancelsCapturedWorkerWithoutReplay(t *testing.T) {
	r, address := testInspectionRouter(t)
	entered := make(chan struct{})
	var oldCalls, newCalls atomic.Int32
	old := testInspectionWorker(t, func(ctx context.Context, q inspection.Request) (inspection.Report, error) {
		oldCalls.Add(1)
		close(entered)
		<-ctx.Done()
		return inspection.Report{}, ctx.Err()
	})
	next := testInspectionWorker(t, func(ctx context.Context, q inspection.Request) (inspection.Report, error) {
		newCalls.Add(1)
		return routerReport(q), nil
	})
	r.setWorker(old)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	client := routerClient(t, address)
	go func() { _, e := inspection.Inspect(ctx, client, routerOwner()); done <- e }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("old worker not reached")
	}
	r.setWorker(next)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("replaced worker observation accepted")
		}
	case <-ctx.Done():
		t.Fatal("old observation did not cancel")
	}
	if oldCalls.Load() != 1 || newCalls.Load() != 0 {
		t.Fatal("observation replayed across replacement")
	}
	report, err := inspection.Inspect(ctx, routerClient(t, address), routerOwner())
	if err != nil || report != routerReport(routerOwner()) || newCalls.Load() != 1 {
		t.Fatal("fresh replacement observation failed", err)
	}
}
func TestInspectionRouterDoesNotRouteForeignServicesOrNonUnixPeers(t *testing.T) {
	r, address := testInspectionRouter(t)
	conn, err := net.DialTimeout("unix", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.Write(taskFrame(t, "foreign.service")); err != nil {
		t.Fatal(err)
	}
	if c, err := r.taskConnection(20 * time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		if c != nil {
			_ = c.Close()
		}
		t.Fatal("foreign service took task bridge", err)
	}
	// Handshake must refuse a connection with no Unix peer credentials.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	r.slots <- struct{}{}
	r.mu.Lock()
	r.clients[a] = struct{}{}
	r.mu.Unlock()
	finished := make(chan struct{})
	go func() { r.route(a); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("non-Unix peer was not refused")
	}
}

func TestInspectionRouterBoundsIdleClientsAndClosesThem(t *testing.T) {
	r, address := testInspectionRouter(t)
	var clients []net.Conn
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()
	for n := 0; n < cap(r.slots); n++ {
		c, err := net.DialTimeout("unix", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
	}
	deadline := time.Now().Add(time.Second)
	for len(r.slots) != cap(r.slots) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(r.slots) != cap(r.slots) {
		t.Fatal("idle connections not classified")
	}
	extra, err := net.DialTimeout("unix", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	_ = extra.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err = extra.Read(b[:]); err == nil {
		t.Fatal("excess client was not closed")
	} else if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("excess client waited for a slot")
	}
	r.close()
	for _, c := range clients {
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		if _, err = c.Read(b[:]); err == nil {
			t.Fatal("idle client survived router shutdown")
		} else if e, ok := err.(net.Error); ok && e.Timeout() {
			t.Fatal("idle client not canceled")
		}
	}
	if c, err := r.taskConnection(time.Second); !errors.Is(err, context.Canceled) {
		if c != nil {
			_ = c.Close()
		}
		t.Fatal("closed router handed off a task connection", err)
	}
}
