// runtime-task-barrier is a qualification-only containerd client that exposes
// the otherwise internal boundary between Task Create and Task Start.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	containerd "github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
)

type observation struct {
	Phase     string `json:"phase"`
	ID        string `json:"id"`
	PID       uint32 `json:"pid"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

func writeExclusive(path string, value observation) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = json.NewEncoder(file).Encode(value); err != nil {
		return err
	}
	return file.Sync()
}

func emit(value observation) error {
	return json.NewEncoder(os.Stdout).Encode(value)
}

func main() {
	var socket, namespace, id, ready, continuation string
	var timeout time.Duration
	flag.StringVar(&socket, "address", "/run/containerd/containerd.sock", "containerd socket")
	flag.StringVar(&namespace, "namespace", "default", "containerd namespace")
	flag.StringVar(&id, "id", "", "container ID")
	flag.StringVar(&ready, "ready", "", "exclusive Task CREATED marker")
	flag.StringVar(&continuation, "continue", "", "file whose appearance permits Task Start")
	flag.DurationVar(&timeout, "timeout", 10*time.Minute, "maximum barrier wait")
	flag.Parse()
	if id == "" || namespace == "" || !filepath.IsAbs(socket) || !filepath.IsAbs(ready) || !filepath.IsAbs(continuation) || timeout <= 0 {
		fmt.Fprintln(os.Stderr, "id, namespace, absolute address/ready/continue paths, and positive timeout are required")
		os.Exit(2)
	}

	client, err := containerd.New(socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect containerd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()
	ctx := namespaces.WithNamespace(context.Background(), namespace)
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load container: %v\n", err)
		os.Exit(1)
	}
	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create task: %v\n", err)
		os.Exit(1)
	}
	status, err := task.Status(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe created task: %v\n", err)
		os.Exit(1)
	}
	created := observation{Phase: "created", ID: id, PID: task.Pid(), Status: string(status.Status), Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = writeExclusive(ready, created); err != nil {
		fmt.Fprintf(os.Stderr, "publish created barrier: %v\n", err)
		os.Exit(1)
	}
	if err = emit(created); err != nil {
		fmt.Fprintf(os.Stderr, "emit created observation: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(timeout)
	for {
		_, err = os.Lstat(continuation)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "observe continuation: %v\n", err)
			os.Exit(1)
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "continuation deadline exceeded")
			os.Exit(1)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err = task.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "start task: %v\n", err)
		os.Exit(1)
	}
	status, err = task.Status(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe started task: %v\n", err)
		os.Exit(1)
	}
	started := observation{Phase: "started", ID: id, PID: task.Pid(), Status: string(status.Status), Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = emit(started); err != nil {
		fmt.Fprintf(os.Stderr, "emit started observation: %v\n", err)
		os.Exit(1)
	}
}
