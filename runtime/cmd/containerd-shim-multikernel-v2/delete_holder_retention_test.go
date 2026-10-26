//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func TestInitDeleteRetainsRealHolderUntilHostCleanupSucceeds(t *testing.T) {
	for _, tc := range []struct {
		name, method, code string
		retryable          bool
	}{
		{"stop unavailable", "StopSandbox", "UNAVAILABLE", true},
		{"delete unavailable", "DeleteSandbox", "UNAVAILABLE", true},
		{"unsafe storage release", "DeleteSandbox", "OPERATOR_ACTION", false},
		{"event publication unavailable", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := exec.Command("/bin/sleep", "30")
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = command.Process.Kill()
				if command.ProcessState == nil {
					_ = command.Wait()
				}
			})
			holder := &commandNamespaceHolder{command: command}
			pid := holder.PID()
			fd, err := unix.PidfdOpen(int(pid), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			netns := "/proc/" + strconv.Itoa(int(pid)) + "/ns/net"
			identity, err := os.Stat(netns)
			if err != nil {
				t.Fatal(err)
			}
			p := &process{pid: 17, status: tasktypes.Status_STOPPED, exit: 9, exited: time.Unix(123, 0).UTC(), exitEventQueued: true, done: make(chan struct{})}
			close(p.done)
			failing := true
			publisher := &fakePublisher{}
			if tc.method == "" {
				publisher.failures = 2
			}
			bundle := privateTestDirectory(t)
			if err := os.Mkdir(bundle+"/.multikernel", 0700); err != nil {
				t.Fatal(err)
			}
			s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher, namespaceHolder: holder,
				sandbox: protocol.Sandbox{ID: "mk-task", Generation: strings.Repeat("a", 32)}, processes: map[string]*process{"": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
				daemon: daemonCallFunc(func(_ context.Context, r protocol.Request, _ any) *protocol.Error {
					if failing && r.Method == tc.method {
						return &protocol.Error{Code: tc.code, Message: "injected host cleanup refusal", Retryable: tc.retryable}
					}
					return nil
				})}
			response, err := s.Delete(t.Context(), &taskapi.DeleteRequest{ID: "task"})
			if err == nil || response != nil {
				t.Fatalf("failed cleanup returned success: %+v %v", response, err)
			}
			if s.namespaceHolder != holder || s.processes[""] != p || p.deleting || (p.deleteEventQueued != (tc.method == "")) || len(publisher.topics) != 0 {
				t.Fatal("failed host cleanup released holder/task or queued deletion")
			}
			polls := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, err := unix.Poll(polls, 0)
			if err != nil || n != 0 {
				t.Fatalf("host cleanup failure killed its representative PID: poll=%d err=%v", n, err)
			}
			after, err := os.Stat(netns)
			if err != nil || !os.SameFile(identity, after) {
				t.Fatal("failed cleanup lost the held namespace")
			}
			if got := s.taskPID("", p); got != pid {
				t.Fatalf("retained task PID=%d want=%d", got, pid)
			}
			if !tc.retryable {
				return
			}
			failing = false
			response, err = s.Delete(t.Context(), &taskapi.DeleteRequest{ID: "task"})
			if err != nil || response.Pid != pid || s.namespaceHolder != nil || s.processes[""] != nil {
				t.Fatalf("retry did not finish exact retained holder: %+v %v", response, err)
			}
			polls[0].Revents = 0
			n, err = unix.Poll(polls, 0)
			if err != nil || n != 1 {
				t.Fatalf("successful cleanup left holder alive: %d %v", n, err)
			}
			if fmt.Sprint(publisher.topics) != "[/tasks/delete]" {
				t.Fatalf("delete events=%v", publisher.topics)
			}
		})
	}
}
