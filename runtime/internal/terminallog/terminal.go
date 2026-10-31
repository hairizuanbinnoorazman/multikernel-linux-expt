// Package terminallog emits one bounded, fixed-code terminal daemon diagnostic.
// It never accepts error text or workload metadata. Inspectors retain their own
// explicit output contracts; this is not a readiness or durable logging API.
package terminallog

import (
	"encoding/json"
	"io"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const DeliveryBudget = 50 * time.Millisecond
const RecordLimit = 1024

type Code uint8

const (
	RuntimeHostConfig Code = iota + 1
	RuntimePoolCPUs
	RuntimeForbiddenCPU
	RuntimePoolMemory
	RuntimePoolReserve
	RuntimeStateOpen
	RuntimeIdlePoolRecover
	RuntimeHostPolicy
	RuntimeStorageStateOpen
	RuntimeRootfsStateOpen
	RuntimeRootfsService
	RuntimeReconcile
	RuntimeRootfsReconcile
	RuntimeListen
	RuntimeIdlePoolRelease
	NetworkEgress
	NetworkStateOpen
	NetworkBootID
	NetworkService
	NetworkReconcile
	NetworkSocketParent
	NetworkListen
	NetworkUnexpected
)

var specs = [...]struct{ component, code, operation string }{
	{},
	{"mkruntimed", "RUNTIME_HOST_CONFIG_FAILED", "load-host-config"},
	{"mkruntimed", "RUNTIME_POOL_CPUS_INVALID", "validate-pool-cpus"},
	{"mkruntimed", "RUNTIME_POOL_CPU_FORBIDDEN", "validate-pool-cpus"},
	{"mkruntimed", "RUNTIME_POOL_MEMORY_INVALID", "validate-pool-memory"},
	{"mkruntimed", "RUNTIME_POOL_RESERVE_INVALID", "validate-pool-reserve"},
	{"mkruntimed", "RUNTIME_STATE_OPEN_FAILED", "open-lifecycle-state"},
	{"mkruntimed", "RUNTIME_IDLE_POOL_RECOVERY_FAILED", "recover-idle-pool"},
	{"mkruntimed", "RUNTIME_HOST_POLICY_REFUSED", "validate-host-policy"},
	{"mkruntimed", "RUNTIME_STORAGE_STATE_OPEN_FAILED", "open-storage-state"},
	{"mkruntimed", "RUNTIME_ROOTFS_STATE_OPEN_FAILED", "open-rootfs-state"},
	{"mkruntimed", "RUNTIME_ROOTFS_SERVICE_FAILED", "initialize-rootfs-service"},
	{"mkruntimed", "RUNTIME_RECONCILE_FAILED", "reconcile-lifecycle"},
	{"mkruntimed", "RUNTIME_ROOTFS_RECONCILE_FAILED", "reconcile-rootfs"},
	{"mkruntimed", "RUNTIME_LISTEN_FAILED", "serve-runtime"},
	{"mkruntimed", "RUNTIME_IDLE_POOL_RELEASE_FAILED", "release-idle-pool"},
	{"mknetd", "NETWORK_EGRESS_CHECK_FAILED", "validate-egress"},
	{"mknetd", "NETWORK_STATE_OPEN_FAILED", "open-network-state"},
	{"mknetd", "NETWORK_BOOT_ID_READ_FAILED", "read-primary-boot-id"},
	{"mknetd", "NETWORK_SERVICE_FAILED", "initialize-network-service"},
	{"mknetd", "NETWORK_RECONCILE_FAILED", "reconcile-network"},
	{"mknetd", "NETWORK_SOCKET_PARENT_FAILED", "prepare-network-socket"},
	{"mknetd", "NETWORK_LISTEN_FAILED", "serve-network"},
	{"mknetd", "NETWORK_TERMINAL_FAILED", "run-network-daemon"},
}

type record struct {
	Version    int    `json:"version"`
	Component  string `json:"component"`
	Scope      string `json:"scope"`
	Code       string `json:"code"`
	Operation  string `json:"operation"`
	PID        int    `json:"pid"`
	OwnerKnown bool   `json:"owner_known"`
	At         string `json:"at"`
}

type reporter struct {
	writer    io.Writer
	attempted atomic.Bool
}

// The production reporter owns at most one write goroutine, one record, and no
// queue or retries for the entire process. A timed-out write can remain blocked
// until stderr is released or the process exits. Later calls do not add workers.
var processReporter = reporter{writer: stderrWriter{}}

type stderrWriter struct{}

func (stderrWriter) Write(data []byte) (int, error) {
	// Go gives writes to descriptors 1/2 special SIGPIPE termination behavior.
	// A private CLOEXEC descriptor >=3 reports EPIPE instead. Do not change the
	// inherited descriptor's flags or the process-wide signal disposition.
	fd, err := unix.FcntlInt(2, unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return 0, err
	}
	file := os.NewFile(uintptr(fd), "terminal-stderr")
	defer file.Close()
	return file.Write(data)
}

func Report(code Code) bool { return processReporter.report(code) }

func (r *reporter) report(code Code) bool {
	if r == nil || code == 0 || int(code) >= len(specs) || !r.attempted.CompareAndSwap(false, true) {
		return false
	}
	if r.writer == nil {
		return false
	}
	spec := specs[code]
	data, err := json.Marshal(record{Version: 1, Component: spec.component, Scope: "daemon-terminal", Code: spec.code, Operation: spec.operation, PID: os.Getpid(), At: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil || len(data)+1 > RecordLimit {
		return false
	}
	data = append(data, '\n')
	done := make(chan bool, 1)
	timer := time.NewTimer(DeliveryBudget)
	defer timer.Stop()
	go func() { n, err := r.writer.Write(data); done <- err == nil && n == len(data) }()
	select {
	case ok := <-done:
		return ok
	case <-timer.C:
		return false
	}
}
