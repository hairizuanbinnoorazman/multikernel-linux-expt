package agent

import (
	"bytes"
	"testing"
)

func TestZeroOffsetStoppedOutputObservationDoesNotConsumeRetainedBuffers(t *testing.T) {
	m := NewManager(true)
	p := &process{state: ProcessState{ID: "init", Status: "STOPPED", PID: 616, ExitCode: 137}}
	p.stdout.Write([]byte("old-retained-stdout"))
	p.stderr.Write([]byte("old-retained-stderr"))
	m.processes["init"] = p
	// Model acknowledgments made by a prior shim before it failed.
	m.ReadOutput("init", 4, 4, 3)
	stdoutBefore, stderrBefore := p.stdout.String(), p.stderr.String()
	stdoutBase, stderrBase := p.stdout.base, p.stderr.base
	var previousOut, previousErr []byte
	var previousNextOut, previousNextErr uint64
	for i := 0; i < 2; i++ {
		out, errout, nextOut, nextErr, status, err := m.ReadOutput("init", 0, 0, 8)
		if err != nil || status != "STOPPED" {
			t.Fatal("stopped observation refused")
		}
		if i == 1 && (!bytes.Equal(out, previousOut) || !bytes.Equal(errout, previousErr) || nextOut != previousNextOut || nextErr != previousNextErr) {
			t.Fatal("zero-offset observation changed retained sample")
		}
		previousOut, previousErr, previousNextOut, previousNextErr = out, errout, nextOut, nextErr
	}
	if p.stdout.base != stdoutBase || p.stderr.base != stderrBase || p.stdout.String() != stdoutBefore || p.stderr.String() != stderrBefore || p.state.Status != "STOPPED" {
		t.Fatal("diagnostic consumed retained bytes or changed process state")
	}
}
