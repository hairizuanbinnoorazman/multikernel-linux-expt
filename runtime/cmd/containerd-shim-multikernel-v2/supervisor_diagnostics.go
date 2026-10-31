//go:build linux

package main

import (
	"encoding/json"
	"os"
	"time"
)

type supervisorDiagnosticCode uint8

const (
	supervisorListenerMissing supervisorDiagnosticCode = iota + 1
	supervisorExecutableFailed
	supervisorDirectoryFailed
	supervisorListenerFailed
	supervisorAcceptFailed
	supervisorPIDInspectFailed
	supervisorPIDResidueFailed
	supervisorPrivateListenerFailed
	supervisorStartFailed
	supervisorConnectFailed
	supervisorPIDPublishFailed
	supervisorPIDRemoveFailed
	supervisorWaitFailed
	supervisorExitUnrecoverable
	supervisorProxyFailed
	supervisorRestart
	supervisorBudgetExhausted
)

var supervisorDiagnosticSpecs = [...]struct{ code, operation string }{
	{},
	{"SUPERVISOR_LISTENER_MISSING", "open-inherited-listener"},
	{"SUPERVISOR_EXECUTABLE_FAILED", "find-worker-executable"},
	{"SUPERVISOR_DIRECTORY_FAILED", "open-worker-directory"},
	{"SUPERVISOR_LISTENER_FAILED", "open-inherited-listener"},
	{"SUPERVISOR_ACCEPT_FAILED", "accept-containerd-connection"},
	{"SUPERVISOR_PID_INSPECT_FAILED", "inspect-worker-pid"},
	{"SUPERVISOR_PID_RESIDUE_FAILED", "remove-worker-pid-residue"},
	{"SUPERVISOR_PRIVATE_LISTENER_FAILED", "prepare-worker-listener"},
	{"SUPERVISOR_START_FAILED", "start-worker"},
	{"SUPERVISOR_CONNECT_FAILED", "connect-worker"},
	{"SUPERVISOR_PID_PUBLISH_FAILED", "publish-worker-pid"},
	{"SUPERVISOR_PID_REMOVE_FAILED", "remove-worker-pid"},
	{"SUPERVISOR_WAIT_FAILED", "wait-worker"},
	{"SUPERVISOR_EXIT_UNRECOVERABLE", "wait-worker"},
	{"SUPERVISOR_PROXY_FAILED", "proxy-worker-connection"},
	{"SUPERVISOR_RESTART", "restart-worker"},
	{"SUPERVISOR_BUDGET_EXHAUSTED", "restart-worker"},
}

type supervisorDiagnosticRecord struct {
	Version       int    `json:"version"`
	Component     string `json:"component"`
	Scope         string `json:"scope"`
	Code          string `json:"code"`
	Operation     string `json:"operation"`
	SupervisorPID uint32 `json:"supervisor_pid"`
	OwnerKnown    bool   `json:"owner_known"`
	WorkerPID     uint32 `json:"worker_pid,omitempty"`
	Attempt       uint32 `json:"attempt,omitempty"`
	Signal        uint32 `json:"signal,omitempty"`
	At            string `json:"at"`
}

func validSupervisorDiagnosticRecord(r supervisorDiagnosticRecord) bool {
	if r.Version != 1 || r.Component != "containerd-shim-multikernel-v2" || r.Scope != "worker-supervisor" || r.OwnerKnown || r.SupervisorPID == 0 || r.SupervisorPID >= 1<<31 || r.WorkerPID >= 1<<31 || len(r.At) == 0 || len(r.At) > 35 {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, r.At)
	if err != nil || at.UTC().Format(time.RFC3339Nano) != r.At {
		return false
	}
	for i, spec := range supervisorDiagnosticSpecs[1:] {
		if r.Code == spec.code && r.Operation == spec.operation {
			if supervisorDiagnosticCode(i+1) == supervisorRestart {
				return r.WorkerPID > 0 && r.Attempt > 0 && r.Attempt <= 10 && r.Signal > 0 && r.Signal <= 64
			}
			return r.WorkerPID == 0 && r.Attempt == 0 && r.Signal == 0
		}
	}
	return false
}
func (s *processDiagnosticSink) trySupervisorRecord(r supervisorDiagnosticRecord) bool {
	if s == nil {
		return false
	}
	if s.disabled.Load() || !validSupervisorDiagnosticRecord(r) {
		s.dropped.Add(1)
		return false
	}
	data, err := json.Marshal(r)
	if err != nil {
		s.dropped.Add(1)
		return false
	}
	return s.tryEncodedDiagnostic(data)
}

type supervisorDiagnosticReporter struct {
	sink      *processDiagnosticSink
	attempted uint64
	pid       uint32
}

func newSupervisorDiagnostics(sink *processDiagnosticSink) *supervisorDiagnosticReporter {
	if sink == nil {
		sink = processDiagnosticDefault()
	}
	return &supervisorDiagnosticReporter{sink: sink, pid: uint32(os.Getpid())}
}

// One attempt per code for one supervision invocation, across all crash retries
// and clean containerd handoffs. Only host numeric restart metadata is accepted.
// The supervisor has no validated workload generation; owner_known stays false.
// PID/time metadata is observation, never process-incarnation or owner authority.
func (r *supervisorDiagnosticReporter) emit(code supervisorDiagnosticCode, workerPID, attempt, signal uint32) bool {
	if r == nil || code == 0 || int(code) >= len(supervisorDiagnosticSpecs) || code >= 64 {
		return false
	}
	bit := uint64(1) << code
	if r.attempted&bit != 0 {
		return false
	}
	r.attempted |= bit
	spec := supervisorDiagnosticSpecs[code]
	return r.sink.trySupervisorRecord(supervisorDiagnosticRecord{Version: 1, Component: "containerd-shim-multikernel-v2", Scope: "worker-supervisor", Code: spec.code, Operation: spec.operation, SupervisorPID: r.pid, OwnerKnown: false, WorkerPID: workerPID, Attempt: attempt, Signal: signal, At: time.Now().UTC().Format(time.RFC3339Nano)})
}
