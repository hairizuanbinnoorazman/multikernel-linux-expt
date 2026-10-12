//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *service) pumpStdin(agentID string, p *process) {
	reader := p.stdinReader
	if reader != nil {
		defer func() {
			s.mu.Lock()
			_ = reader.Close()
			p.stdinReader = nil
			pending := len(p.stdinPending) != 0
			s.mu.Unlock()
			if !pending {
				s.retryPendingStdinClose(agentID, p)
			}
		}()
	}
	if err := s.deliverPendingStdin(agentID, p); err != nil {
		fmt.Fprintf(os.Stderr, "multikernel stdin: %v\n", err)
		return
	}
	if reader == nil {
		s.retryPendingStdinClose(agentID, p)
		return
	}
	buffer := make([]byte, 32<<10)
	for {
		s.mu.Lock()
		closeRequested := p.stdinClosed
		closeAcknowledged := p.stdinCloseAcked
		s.mu.Unlock()
		if closeRequested {
			if !closeAcknowledged {
				s.retryPendingStdinClose(agentID, p)
			}
			return
		}
		n, err := reader.Read(buffer)
		if n > 0 {
			s.mu.Lock()
			stopped := p.status != tasktypes.Status_RUNNING && p.status != tasktypes.Status_PAUSED
			if !stopped {
				p.stdinPending = append(p.stdinPending[:0], buffer[:n]...)
				if persistErr := s.persistRecovery(); persistErr != nil {
					stopped = true
					fmt.Fprintf(os.Stderr, "multikernel stdin intent: %v\n", persistErr)
				}
			}
			s.mu.Unlock()
			if stopped {
				return
			}
			if callErr := s.deliverPendingStdin(agentID, p); callErr != nil {
				fmt.Fprintf(os.Stderr, "multikernel stdin: %v\n", callErr)
				return
			}
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return
		}
		if n == 0 {
			s.mu.Lock()
			closeRequested := p.stdinClosed
			closeAcknowledged := p.stdinCloseAcked
			s.mu.Unlock()
			if closeRequested && !closeAcknowledged {
				s.retryPendingStdinClose(agentID, p)
				return
			}
			// FIFO EOF can also mean that an attaching client disconnected.
			// Keep the guest side open until CloseIO explicitly requests EOF.
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *service) deliverPendingStdin(agentID string, p *process) error {
	s.mu.Lock()
	offset := p.stdinOffset
	data := append([]byte(nil), p.stdinPending...)
	s.mu.Unlock()
	if len(data) == 0 {
		return nil
	}
	var response struct {
		Offset uint64 `json:"offset"`
	}
	if err := s.callAgentWithReconnect("WriteProcess", map[string]any{
		"id": agentID, "offset": offset, "data": data,
	}, &response); err != nil {
		return err
	}
	if uint64(len(data)) > ^uint64(0)-offset || response.Offset != offset+uint64(len(data)) {
		return errors.New("guest acknowledged an invalid stdin offset")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.stdinOffset != offset || !bytes.Equal(p.stdinPending, data) {
		return errors.New("pending stdin identity changed during guest acknowledgement")
	}
	p.stdinOffset, p.stdinPending = response.Offset, nil
	if err := s.persistRecovery(); err != nil {
		p.stdinOffset, p.stdinPending = offset, data
		return fmt.Errorf("persist acknowledged stdin offset: %w", err)
	}
	return nil
}

func (s *service) retryPendingStdinClose(agentID string, p *process) {
	for {
		s.mu.Lock()
		requested := p.stdinClosed && !p.stdinCloseAcked
		stopped := p.status == tasktypes.Status_STOPPED
		s.mu.Unlock()
		if !requested || stopped {
			return
		}
		if err := s.acknowledgeStdinClose(context.Background(), agentID, p); err == nil {
			return
		} else {
			fmt.Fprintf(os.Stderr, "multikernel close stdin: %v\n", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *service) acknowledgeStdinClose(ctx context.Context, agentID string, p *process) error {
	s.mu.Lock()
	if p.stdinCloseAcked {
		s.mu.Unlock()
		return nil
	}
	connected := s.agent != nil
	s.mu.Unlock()
	if !connected {
		return errdefs.ErrFailedPrecondition
	}
	if err := s.callAgentWithReconnectContext(ctx, "CloseProcessStdin", map[string]string{"id": agentID}, nil); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.stdinCloseAcked {
		return nil
	}
	p.stdinCloseAcked = true
	if err := s.persistRecovery(); err != nil {
		p.stdinCloseAcked = false
		return fmt.Errorf("persist acknowledged stdin close: %w", err)
	}
	return nil
}

func (s *service) waitProcess(agentID, execID string, p *process) {
	var state agent.ProcessState
	var pid, exit uint32
	var err error
	stdoutTruncationReported, stderrTruncationReported := false, false
	for {
		var output processOutput
		err = s.readProcessOutput(agentID, p.stdoutOffset, p.stderrOffset, &output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "multikernel output monitor: %v\n", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		now := time.Now()
		stdoutAdvance, stdoutDropped := deliverOutput(p.stdoutWriter, output.Stdout, &p.stdoutPressure, now, 30*time.Second)
		stderrAdvance, stderrDropped := deliverOutput(p.stderrWriter, output.Stderr, &p.stderrPressure, now, 30*time.Second)
		s.mu.Lock()
		oldStdoutOffset, oldStderrOffset := p.stdoutOffset, p.stderrOffset
		if stdoutAdvance {
			p.stdoutOffset = output.StdoutOffset
		}
		if stderrAdvance {
			p.stderrOffset = output.StderrOffset
		}
		stdoutChanged := p.stdoutOffset != oldStdoutOffset
		stderrChanged := p.stderrOffset != oldStderrOffset
		var persistErr error
		if stdoutChanged || stderrChanged {
			persistErr = s.persistRecovery()
			if persistErr != nil {
				p.stdoutOffset, p.stderrOffset = oldStdoutOffset, oldStderrOffset
				if stdoutChanged {
					stdoutAdvance = false
				}
				if stderrChanged {
					stderrAdvance = false
				}
			}
		}
		s.mu.Unlock()
		if persistErr != nil {
			fmt.Fprintf(os.Stderr, "multikernel output acknowledgement: %v\n", persistErr)
		}
		if stdoutDropped {
			fmt.Fprintf(os.Stderr, "multikernel stdout: discarded %d bytes after 30s output pressure\n", len(output.Stdout))
		}
		if stderrDropped {
			fmt.Fprintf(os.Stderr, "multikernel stderr: discarded %d bytes after 30s output pressure\n", len(output.Stderr))
		}
		if output.StdoutTruncated && !stdoutTruncationReported {
			fmt.Fprintln(os.Stderr, "multikernel stdout: guest retention limit discarded output before delivery")
			stdoutTruncationReported = true
		}
		if output.StderrTruncated && !stderrTruncationReported {
			fmt.Fprintln(os.Stderr, "multikernel stderr: guest retention limit discarded output before delivery")
			stderrTruncationReported = true
		}
		if !stdoutAdvance || !stderrAdvance {
			time.Sleep(10 * time.Millisecond)
		}
		if output.Status == "STOPPED" && len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			err = s.callAgentWithReconnect("WaitProcess", map[string]string{"ID": agentID}, &state)
			if err != nil {
				fmt.Fprintf(os.Stderr, "multikernel process wait: %v\n", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			pid, exit, err = stoppedGuestProcess(state, agentID, p.pid)
			if err != nil {
				fmt.Fprintf(os.Stderr, "multikernel process wait validation: %v\n", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			break
		}
		if len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	now := time.Now().UTC()
	for {
		s.mu.Lock()
		closeProcessIO(p)
		oldStatus, oldPID, oldExit, oldExited := p.status, p.pid, p.exit, p.exited
		oldPending := append([]byte(nil), p.stdinPending...)
		oldSignal, oldSignalOperation, oldSignalAck := p.pendingSignal, p.pendingSignalOperation, p.pendingSignalAcknowledgement
		p.status, p.pid, p.exit, p.exited, p.stdinPending = tasktypes.Status_STOPPED, pid, exit, now, nil
		if p.pendingSignalOperation != "" {
			p.pendingSignalAcknowledgement = true
		}
		if err = s.persistRecovery(); err == nil {
			break
		}
		p.status, p.pid, p.exit, p.exited, p.stdinPending = oldStatus, oldPID, oldExit, oldExited, oldPending
		p.pendingSignal, p.pendingSignalOperation, p.pendingSignalAcknowledgement = oldSignal, oldSignalOperation, oldSignalAck
		s.mu.Unlock()
		fmt.Fprintf(os.Stderr, "multikernel exit state persistence: %v\n", err)
		time.Sleep(100 * time.Millisecond)
	}
	if p.pendingSignalOperation != "" {
		operationID, signal := p.pendingSignalOperation, p.pendingSignal
		ackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ackErr := s.acknowledgeGuestSignal(ackCtx, agentID, operationID)
		cancel()
		if ackErr != nil {
			fmt.Fprintf(os.Stderr, "multikernel signal acknowledgement: %v\n", ackErr)
		} else {
			p.pendingSignal, p.pendingSignalOperation, p.pendingSignalAcknowledgement = 0, "", false
			if err = s.persistRecovery(); err != nil {
				p.pendingSignal, p.pendingSignalOperation, p.pendingSignalAcknowledgement = signal, operationID, true
				fmt.Fprintf(os.Stderr, "multikernel signal acknowledgement persistence: %v\n", err)
			}
		}
	}
	if err := s.publishExit(context.Background(), execID, p); err != nil {
		fmt.Fprintf(os.Stderr, "multikernel exit event queue: %v\n", err)
	} else {
		p.exitEventQueued = true
		if err := s.persistRecovery(); err != nil {
			p.exitEventQueued = false
			fmt.Fprintf(os.Stderr, "multikernel exit event acknowledgement: %v\n", err)
		}
	}
	close(p.done)
	s.mu.Unlock()
}

func (s *service) readProcessOutput(agentID string, stdoutOffset, stderrOffset uint64, output *processOutput) error {
	if err := s.callAgentWithReconnect("ReadProcessOutput", map[string]any{
		"id": agentID, "stdout_offset": stdoutOffset, "stderr_offset": stderrOffset, "limit": uint64(processOutputChunk),
	}, output); err != nil {
		return err
	}
	return validateProcessOutput(stdoutOffset, stderrOffset, *output)
}

func validateProcessOutput(stdoutOffset, stderrOffset uint64, output processOutput) error {
	if len(output.Stdout) > processOutputChunk || len(output.Stderr) > processOutputChunk {
		return errors.New("guest output exceeded the requested atomic chunk")
	}
	exactOffset := func(current, next uint64, data []byte) bool {
		return uint64(len(data)) <= ^uint64(0)-current && next == current+uint64(len(data))
	}
	if !exactOffset(stdoutOffset, output.StdoutOffset, output.Stdout) ||
		!exactOffset(stderrOffset, output.StderrOffset, output.Stderr) {
		return errors.New("guest output returned a non-contiguous offset")
	}
	if output.Status != "RUNNING" && output.Status != "STOPPED" {
		return errors.New("guest output returned an invalid process state")
	}
	return nil
}

// deliverOutput advances a guest offset only after the complete chunk reaches
// its destination. Chunks are capped at Linux PIPE_BUF by waitProcess, making
// nonblocking FIFO writes all-or-nothing. A permanently absent/slow consumer
// cannot retain the shim forever: after the documented bound the chunk is
// deliberately discarded and acknowledged with a diagnostic.
func deliverOutput(writer io.Writer, data []byte, pressure *time.Time, now time.Time, timeout time.Duration) (advance, dropped bool) {
	if len(data) == 0 || writer == nil {
		*pressure = time.Time{}
		return true, false
	}
	n, err := writer.Write(data)
	if err == nil && n == len(data) {
		*pressure = time.Time{}
		return true, false
	}
	if pressure.IsZero() {
		*pressure = now
	}
	if timeout > 0 && now.Sub(*pressure) >= timeout {
		*pressure = time.Time{}
		return true, true
	}
	return false, false
}

func (s *service) publishExit(ctx context.Context, execID string, p *process) error {
	eventID := execID
	if eventID == "" {
		eventID = s.id
	}
	return s.publish(ctx, ctruntime.TaskExitEventTopic, &eventstypes.TaskExit{ContainerID: s.id, ID: eventID,
		Pid: s.taskPID(execID, p), ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)})
}
