package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestTerminateRelayReapsListeningHelper(t *testing.T) {
	command := exec.Command("sleep", "30")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
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
}
