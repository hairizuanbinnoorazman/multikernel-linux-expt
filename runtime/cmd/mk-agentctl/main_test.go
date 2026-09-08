package main

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestTerminateRelayReapsListeningHelper(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "sleep 30 & wait")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	processGroup := command.Process.Pid
	started := time.Now()
	if err := terminateRelay(command); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("relay termination was not bounded")
	}
	if command.ProcessState == nil || !errors.Is(command.Process.Signal(syscall.Signal(0)), os.ErrProcessDone) {
		t.Fatalf("relay was not reaped: %+v", command.ProcessState)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err := syscall.Kill(-processGroup, syscall.Signal(0))
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay process group %d remains: %v", processGroup, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAgentExchangeBoundsBlockedWriteAndRead(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		started := time.Now()
		_, err := exchangePayloadWithTimeout(client, []byte("request"), 50*time.Millisecond)
		if err == nil {
			t.Fatal("blocked agent request write succeeded")
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("blocked agent request write took %s", elapsed)
		}
	})

	t.Run("read", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		readDone := make(chan struct{})
		release := make(chan struct{})
		go func() {
			defer close(readDone)
			var header [4]byte
			if _, err := io.ReadFull(server, header[:]); err != nil {
				return
			}
			payload := make([]byte, binary.BigEndian.Uint32(header[:]))
			_, _ = io.ReadFull(server, payload)
			<-release
		}()
		started := time.Now()
		_, err := exchangePayloadWithTimeout(client, []byte("request"), 50*time.Millisecond)
		if err == nil {
			t.Fatal("blocked agent response read succeeded")
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("blocked agent response read took %s", elapsed)
		}
		close(release)
		_ = server.Close()
		<-readDone
	})
}
