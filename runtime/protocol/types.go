// Package protocol contains the versioned daemon and agent contracts.
package protocol

import (
	"encoding/json"
	"time"
)

const Version = 1

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	OperationID string `json:"operation_id,omitempty"`
	Retryable   bool   `json:"retryable"`
}
type Request struct {
	Version        int             `json:"version"`
	RequestID      string          `json:"request_id"`
	Method         string          `json:"method"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	SandboxID      string          `json:"sandbox_id,omitempty"`
	Generation     string          `json:"generation,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
}
type Response struct {
	Version   int    `json:"version"`
	RequestID string `json:"request_id"`
	Body      any    `json:"body,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

type SandboxConfig struct {
	SchemaVersion  int               `json:"schema_version"`
	ID             string            `json:"id"`
	CPUs           []int             `json:"cpus"`
	MemoryBytes    uint64            `json:"memory_bytes"`
	KernelManifest string            `json:"kernel_manifest"`
	Bundle         string            `json:"bundle"`
	AgentPort      uint32            `json:"agent_port"`
	ChildCID       uint32            `json:"child_cid"`
	Labels         map[string]string `json:"labels,omitempty"`
}
type Sandbox struct {
	ID         string        `json:"id"`
	Generation string        `json:"generation"`
	State      string        `json:"state"`
	Error      *Error        `json:"error,omitempty"`
	Config     SandboxConfig `json:"config"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}
type MutationResult struct {
	Sandbox  Sandbox `json:"sandbox"`
	Replayed bool    `json:"replayed"`
}
