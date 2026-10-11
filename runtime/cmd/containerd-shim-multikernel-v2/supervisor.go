//go:build linux

package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

func serverInvocation() bool {
	if len(os.Args) > 1 {
		action := os.Args[len(os.Args)-1]
		if action == "start" || action == "delete" {
			return false
		}
	}
	var info unix.Stat_t
	return unix.Fstat(3, &info) == nil && info.Mode&unix.S_IFMT == unix.S_IFSOCK
}

func superviseShimWorker() int {
	listener := os.NewFile(3, "shim-listener")
	if listener == nil {
		fmt.Fprintln(os.Stderr, "multikernel shim supervisor: inherited listener is missing")
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: executable: %v\n", err)
		return 1
	}
	directory, err := safefile.OpenCurrentOwnedDirectory()
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: open current worker directory: %v\n", err)
		return 1
	}
	defer directory.Close()
	publicListener, err := net.FileListener(listener)
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: open inherited listener: %v\n", err)
		return 1
	}
	_ = listener.Close()
	defer publicListener.Close()
	containerdConnection, err := publicListener.Accept()
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: accept containerd connection: %v\n", err)
		return 1
	}
	reconnect := func(timeout time.Duration) (net.Conn, error) {
		deadline := time.Now().Add(timeout)
		if deadlineListener, ok := publicListener.(interface{ SetDeadline(time.Time) error }); ok {
			if err := deadlineListener.SetDeadline(deadline); err != nil {
				return nil, err
			}
			defer deadlineListener.SetDeadline(time.Time{})
		}
		return publicListener.Accept()
	}
	return superviseReconnectableShimWorkerWithDirectory(containerdConnection, reconnect, self, os.Args[1:], directory.ProcPath(), os.Environ(), directory)
}

type shimContainerdReconnect func(time.Duration) (net.Conn, error)

type shimConnectionBridge struct {
	containerd  net.Conn
	requests    chan ttrpcFrame
	clientErr   chan error
	pending     map[uint32][][]byte
	pendingSize int
}

const (
	ttrpcHeaderLength = 10
	ttrpcPayloadMax   = 4 << 20
	ttrpcRequest      = 0x1
	ttrpcResponse     = 0x2
	ttrpcData         = 0x3
	ttrpcRemoteClosed = 0x1
	ttrpcPendingMax   = 256
	ttrpcPendingBytes = 64 << 20
)

type ttrpcFrame struct {
	streamID uint32
	typeID   byte
	flags    byte
	encoded  []byte
}

func readTTRPCFrame(input io.Reader) (ttrpcFrame, error) {
	header := make([]byte, ttrpcHeaderLength)
	if _, err := io.ReadFull(input, header); err != nil {
		return ttrpcFrame{}, err
	}
	length := binary.BigEndian.Uint32(header[:4])
	if length > ttrpcPayloadMax {
		return ttrpcFrame{}, fmt.Errorf("TTRPC frame length %d exceeds limit", length)
	}
	encoded := make([]byte, ttrpcHeaderLength+int(length))
	copy(encoded, header)
	if _, err := io.ReadFull(input, encoded[ttrpcHeaderLength:]); err != nil {
		return ttrpcFrame{}, err
	}
	return ttrpcFrame{
		streamID: binary.BigEndian.Uint32(header[4:8]),
		typeID:   header[8],
		flags:    header[9],
		encoded:  encoded,
	}, nil
}

func (f ttrpcFrame) terminalResponse() bool {
	return f.typeID == ttrpcResponse || f.typeID == ttrpcData && f.flags&ttrpcRemoteClosed != 0
}

func newShimConnectionBridge(containerd net.Conn) *shimConnectionBridge {
	bridge := &shimConnectionBridge{
		containerd: containerd,
		requests:   make(chan ttrpcFrame, 32),
		clientErr:  make(chan error, 1),
		pending:    make(map[uint32][][]byte),
	}
	go func() {
		for {
			frame, err := readTTRPCFrame(containerd)
			if err != nil {
				bridge.clientErr <- err
				return
			}
			bridge.requests <- frame
		}
	}()
	return bridge
}

func writeFull(output io.Writer, value []byte) error {
	for len(value) != 0 {
		written, err := output.Write(value)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}

func (b *shimConnectionBridge) serveWorker(worker net.Conn) error {
	streamIDs := make([]uint32, 0, len(b.pending))
	for streamID := range b.pending {
		streamIDs = append(streamIDs, streamID)
	}
	sort.Slice(streamIDs, func(i, j int) bool { return streamIDs[i] < streamIDs[j] })
	for _, streamID := range streamIDs {
		for _, encoded := range b.pending[streamID] {
			if err := writeFull(worker, encoded); err != nil {
				return err
			}
		}
	}
	responses := make(chan ttrpcFrame, 32)
	workerErr := make(chan error, 1)
	go func() {
		for {
			frame, err := readTTRPCFrame(worker)
			if err != nil {
				workerErr <- err
				return
			}
			responses <- frame
		}
	}()
	for {
		select {
		case frame := <-b.requests:
			if frame.typeID == ttrpcRequest {
				if _, exists := b.pending[frame.streamID]; exists {
					return fmt.Errorf("duplicate pending TTRPC stream %d", frame.streamID)
				}
				if len(b.pending) >= ttrpcPendingMax {
					return errors.New("pending TTRPC stream limit exceeded")
				}
				b.pending[frame.streamID] = nil
			} else if frame.typeID == ttrpcData {
				if _, exists := b.pending[frame.streamID]; !exists {
					return fmt.Errorf("TTRPC data for unknown stream %d", frame.streamID)
				}
			}
			if frame.typeID == ttrpcRequest || frame.typeID == ttrpcData {
				if len(frame.encoded) > ttrpcPendingBytes-b.pendingSize {
					return errors.New("pending TTRPC replay byte limit exceeded")
				}
				b.pending[frame.streamID] = append(b.pending[frame.streamID], append([]byte(nil), frame.encoded...))
				b.pendingSize += len(frame.encoded)
			}
			if err := writeFull(worker, frame.encoded); err != nil {
				return err
			}
		case frame := <-responses:
			if err := writeFull(b.containerd, frame.encoded); err != nil {
				return fmt.Errorf("write containerd response: %w", err)
			}
			if frame.terminalResponse() {
				for _, encoded := range b.pending[frame.streamID] {
					b.pendingSize -= len(encoded)
				}
				delete(b.pending, frame.streamID)
			}
		case err := <-workerErr:
			return err
		case err := <-b.clientErr:
			return fmt.Errorf("containerd connection: %w", err)
		}
	}
}

func privateShimListener() (*net.UnixListener, *os.File, string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nil, "", fmt.Errorf("generate private listener identity: %w", err)
	}
	address := "\x00multikernel-shim-" + hex.EncodeToString(nonce[:])
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, nil, "", fmt.Errorf("listen on private worker socket: %w", err)
	}
	file, err := listener.File()
	if err != nil {
		_ = listener.Close()
		return nil, nil, "", fmt.Errorf("duplicate private worker listener: %w", err)
	}
	return listener, file, address, nil
}

func superviseProxiedShimWorkerWithDirectory(containerdConnection net.Conn, self string, arguments []string, workingDirectory string, environment []string, directory *safefile.Directory) int {
	return superviseReconnectableShimWorkerWithDirectory(containerdConnection, nil, self, arguments, workingDirectory, environment, directory)
}

func superviseReconnectableShimWorkerWithDirectory(containerdConnection net.Conn, reconnect shimContainerdReconnect, self string, arguments []string, workingDirectory string, environment []string, directory *safefile.Directory) int {
	bridge := newShimConnectionBridge(containerdConnection)
	defer func() { _ = containerdConnection.Close() }()
	var err error
	pidName := ".multikernel-worker.pid"
	bundleIdentity := directory.Identity()
	workerEnvironment := append(environment,
		"MK_SHIM_BUNDLE_DEVICE="+strconv.FormatUint(bundleIdentity.Device, 10),
		"MK_SHIM_BUNDLE_INODE="+strconv.FormatUint(bundleIdentity.Inode, 10),
		"MK_SHIM_BUNDLE_UID="+strconv.FormatUint(uint64(bundleIdentity.UID), 10))
	var pidIdentity safefile.Identity
	pidOwned := false
	removePID := func() error {
		if !pidOwned {
			return nil
		}
		_, removeErr := directory.RemoveIfIdentity(pidName, pidIdentity)
		if removeErr == nil {
			pidOwned = false
		}
		return removeErr
	}
	defer func() { _ = removePID() }()
	if _, present, _, staleIdentity, readErr := directory.ReadPrivateOrQuarantineIdentity(pidName, 64); readErr != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: inspect worker PID residue: %v\n", readErr)
		return 1
	} else if present {
		if _, err = directory.RemoveIfIdentity(pidName, staleIdentity); err != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: remove worker PID residue: %v\n", err)
			return 1
		}
	}
	for attempt := 0; attempt < 10; attempt++ {
		privateListener, listenerFile, address, listenErr := privateShimListener()
		if listenErr != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: prepare private worker listener: %v\n", listenErr)
			return 1
		}
		cmd := exec.Command(self, arguments...)
		cmd.Dir = directory.ProcPath()
		cmd.Env = append(workerEnvironment, "MK_SHIM_WORKER=1")
		cmd.ExtraFiles = []*os.File{listenerFile}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
		if err = cmd.Start(); err != nil {
			_ = listenerFile.Close()
			_ = privateListener.Close()
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: start worker: %v\n", err)
			return 1
		}
		_ = listenerFile.Close()
		workerConnection, dialErr := net.DialTimeout("unix", address, 5*time.Second)
		_ = privateListener.Close()
		if dialErr != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: connect private worker listener: %v\n", dialErr)
			return 1
		}
		created, published, publishErr := directory.PublishExclusiveIdentity(pidName,
			[]byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600)
		if publishErr != nil || !created {
			_ = workerConnection.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: publish worker PID: created=%v error=%v\n", created, publishErr)
			return 1
		}
		pidIdentity, pidOwned = published, true
		workerDone := make(chan error, 1)
		go func() { workerDone <- cmd.Wait() }()
		var proxyErr error
		proxyEndedFirst := false
		workerEndedDuringGrace := false
		restartForContainerd := false
	connectionLoop:
		for {
			proxyDone := make(chan error, 1)
			go func(connection net.Conn) { proxyDone <- bridge.serveWorker(connection) }(workerConnection)
			select {
			case err = <-workerDone:
				_ = workerConnection.Close()
				proxyErr = <-proxyDone
				break connectionLoop
			case proxyErr = <-proxyDone:
				proxyEndedFirst = true
				_ = workerConnection.Close()
				if reconnect == nil {
					timer := time.NewTimer(shimWorkerDisconnectGrace)
					select {
					case err = <-workerDone:
						workerEndedDuringGrace = true
						if !timer.Stop() {
							<-timer.C
						}
					case <-timer.C:
						_ = cmd.Process.Kill()
						err = <-workerDone
					}
					break connectionLoop
				}

				// The public shim listener must outlive containerd itself. Give a
				// replacement daemon one bounded restart window to reconnect before
				// applying the existing orphan-reap policy.
				workerExited := false
				select {
				case err = <-workerDone:
					workerExited = true
					workerEndedDuringGrace = true
				case <-time.After(250 * time.Millisecond):
				}
				nextConnection, reconnectErr := reconnect(shimWorkerDisconnectGrace)
				if reconnectErr != nil {
					if !workerExited {
						_ = cmd.Process.Kill()
						err = <-workerDone
					}
					break connectionLoop
				}
				_ = containerdConnection.Close()
				containerdConnection = nextConnection
				bridge = newShimConnectionBridge(containerdConnection)
				if workerExited {
					restartForContainerd = true
					break connectionLoop
				}
				workerConnection, err = net.DialTimeout("unix", address, 5*time.Second)
				if err != nil {
					_ = cmd.Process.Kill()
					err = <-workerDone
					workerEndedDuringGrace = true
					if err == nil {
						restartForContainerd = true
					}
					break connectionLoop
				}
				proxyEndedFirst = false
				workerEndedDuringGrace = false
			}
		}
		if removeErr := removePID(); removeErr != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: remove worker PID: %v\n", removeErr)
			return 1
		}
		if proxyEndedFirst {
			if proxyErr != nil && !errors.Is(proxyErr, net.ErrClosed) && !errors.Is(proxyErr, io.EOF) {
				fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker proxy ended: %v\n", proxyErr)
			}
		}
		if proxyEndedFirst && !workerEndedDuringGrace {
			return 0
		}
		if restartForContainerd && err == nil {
			// A clean worker exit following the old daemon's disconnect is a
			// handoff, not a consumed crash-restart attempt.
			attempt--
			continue
		}
		if err == nil {
			return 0
		}
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: wait worker: %v\n", err)
			return 1
		}
		status, ok := exitError.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker exited without a recoverable signal: %v\n", err)
			return exitError.ExitCode()
		}
		if proxyErr != nil && !errors.Is(proxyErr, net.ErrClosed) && !errors.Is(proxyErr, io.EOF) {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker proxy: %v\n", proxyErr)
		}
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker pid=%d signal=%s restart_attempt=%d\n", cmd.Process.Pid, status.Signal(), attempt+1)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "multikernel shim supervisor: restart budget exhausted")
	return 1
}

func superviseShimWorkerWith(listener *os.File, self string, arguments []string, workingDirectory string, environment []string) int {
	directory, err := safefile.OpenOwnedDirectory(workingDirectory)
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: open worker directory: %v\n", err)
		return 1
	}
	defer directory.Close()
	return superviseShimWorkerWithDirectory(listener, self, arguments, workingDirectory, environment, directory)
}

func superviseShimWorkerWithDirectory(listener *os.File, self string, arguments []string, workingDirectory string, environment []string, directory *safefile.Directory) int {
	var err error
	pidName := ".multikernel-worker.pid"
	bundleIdentity := directory.Identity()
	workerEnvironment := append(environment,
		"MK_SHIM_BUNDLE_DEVICE="+strconv.FormatUint(bundleIdentity.Device, 10),
		"MK_SHIM_BUNDLE_INODE="+strconv.FormatUint(bundleIdentity.Inode, 10),
		"MK_SHIM_BUNDLE_UID="+strconv.FormatUint(uint64(bundleIdentity.UID), 10))
	var pidIdentity safefile.Identity
	pidOwned := false
	removePID := func() error {
		if !pidOwned {
			return nil
		}
		_, removeErr := directory.RemoveIfIdentity(pidName, pidIdentity)
		if removeErr == nil {
			pidOwned = false
		}
		return removeErr
	}
	defer func() { _ = removePID() }()
	if _, present, _, staleIdentity, readErr := directory.ReadPrivateOrQuarantineIdentity(pidName, 64); readErr != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: inspect worker PID residue: %v\n", readErr)
		return 1
	} else if present {
		if _, err = directory.RemoveIfIdentity(pidName, staleIdentity); err != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: remove worker PID residue: %v\n", err)
			return 1
		}
	}
	for attempt := 0; attempt < 10; attempt++ {
		cmd := exec.Command(self, arguments...)
		cmd.Dir = directory.ProcPath()
		cmd.Env = append(workerEnvironment, "MK_SHIM_WORKER=1")
		cmd.ExtraFiles = []*os.File{listener}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
		if err = cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: start worker: %v\n", err)
			return 1
		}
		created, published, publishErr := directory.PublishExclusiveIdentity(pidName,
			[]byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600)
		if publishErr != nil || !created {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: publish worker PID: created=%v error=%v\n", created, publishErr)
			return 1
		}
		pidIdentity, pidOwned = published, true
		err = cmd.Wait()
		if removeErr := removePID(); removeErr != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: remove worker PID: %v\n", removeErr)
			return 1
		}
		if err == nil {
			return 0
		}
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: wait worker: %v\n", err)
			return 1
		}
		status, ok := exitError.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker exited without a recoverable signal: %v\n", err)
			return exitError.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker pid=%d signal=%s restart_attempt=%d\n", cmd.Process.Pid, status.Signal(), attempt+1)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "multikernel shim supervisor: restart budget exhausted")
	return 1
}
