package protocol

import (
	"errors"
	"regexp"
	"time"
)

const LifecycleAuditCapability = "lifecycle-audit-v1"
const MaxAuditRecords = 512
const DefaultAuditRecords = 128

type DaemonCapabilities struct {
	ProtocolVersion int      `json:"protocol_version"`
	Features        []string `json:"features"`
}

type AuditQuery struct {
	Version       int    `json:"version"`
	AfterSequence uint64 `json:"after_sequence"`
	Limit         uint32 `json:"limit,omitempty"`
}

// AuditRecord is an allowlisted metadata projection, not a serialized journal
// entry. Do not add configuration, raw errors, arguments or credentials here.
type AuditRecord struct {
	Version     int       `json:"version"`
	Component   string    `json:"component"`
	Sequence    uint64    `json:"sequence"`
	At          time.Time `json:"at"`
	OperationID string    `json:"operation_id"`
	SandboxID   string    `json:"sandbox_id"`
	Generation  string    `json:"generation"`
	Method      string    `json:"method"`
	Phase       string    `json:"phase"`
	Outcome     string    `json:"outcome"`
	State       string    `json:"state,omitempty"`
	ErrorCode   string    `json:"error_code,omitempty"`
}

type AuditPage struct {
	Version           int           `json:"version"`
	Records           []AuditRecord `json:"records"`
	NextSequence      uint64        `json:"next_sequence"`
	HighWaterSequence uint64        `json:"high_water_sequence"`
	HasMore           bool          `json:"has_more"`
}

var auditSandboxID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var auditGeneration = regexp.MustCompile(`^[a-f0-9]{32}$`)
var daemonFeature = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func (c DaemonCapabilities) Validate() error {
	if c.ProtocolVersion != Version || c.Features == nil || len(c.Features) > 32 {
		return errors.New("invalid daemon capabilities")
	}
	seen := map[string]bool{}
	for _, feature := range c.Features {
		if !daemonFeature.MatchString(feature) || seen[feature] {
			return errors.New("invalid daemon capability feature")
		}
		seen[feature] = true
	}
	return nil
}

func (q AuditQuery) Validate() error {
	if q.Version != 1 {
		return errors.New("unsupported audit version")
	}
	if q.Limit > MaxAuditRecords {
		return errors.New("audit limit must be at most 512")
	}
	return nil
}

func (q AuditQuery) PageLimit() uint32 {
	if q.Limit == 0 {
		return DefaultAuditRecords
	}
	return q.Limit
}

func (r AuditRecord) Validate() error {
	if r.Version != 1 || r.Component != "mkruntimed" || r.Sequence == 0 || r.At.IsZero() || r.At.Year() < 1 || r.At.Year() > 9999 || !auditSandboxID.MatchString(r.SandboxID) || !auditGeneration.MatchString(r.Generation) || !auditOperationID(r.OperationID) {
		return errors.New("invalid audit record identity")
	}
	switch r.Method {
	case "CreateSandbox", "LoadSandbox", "StartSandbox", "StopSandbox", "DeleteSandbox":
	default:
		return errors.New("invalid audit method")
	}
	switch r.State {
	case "", "ALLOCATING", "CREATED", "LOADED", "RUNNING", "STOPPING", "STOPPED", "RELEASING", "ABSENT":
	default:
		return errors.New("invalid audit state")
	}
	switch r.Phase {
	case "intent":
		if r.Outcome != "pending" || r.ErrorCode != "" {
			return errors.New("invalid audit intent outcome")
		}
	case "complete":
		if r.Outcome == "succeeded" && r.ErrorCode == "" {
			return nil
		}
		if r.Outcome != "failed" || !auditErrorCode(r.ErrorCode) {
			return errors.New("invalid audit completion outcome")
		}
	default:
		return errors.New("invalid audit phase")
	}
	return nil
}

func auditOperationID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}
	return true
}

func auditErrorCode(value string) bool {
	switch value {
	case "INVALID_ARGUMENT", "UNSUPPORTED", "NOT_FOUND", "ALREADY_EXISTS", "STALE_GENERATION", "IDEMPOTENCY_CONFLICT", "FAILED_PRECONDITION", "RESOURCE_EXHAUSTED", "BACKEND_TIMEOUT", "BACKEND_FAILURE", "UNAUTHENTICATED", "ABORTED", "OPERATOR_ACTION", "INTERNAL":
		return true
	}
	return false
}

// Validate checks cursor continuity as well as field bounds. An empty response
// cannot silently jump the cursor or hide retained records.
func (p AuditPage) Validate(query AuditQuery) error {
	if query.Validate() != nil || p.Version != 1 || p.Records == nil || len(p.Records) > int(query.PageLimit()) || p.HighWaterSequence < query.AfterSequence {
		return errors.New("invalid audit page")
	}
	next := query.AfterSequence
	for _, record := range p.Records {
		if record.Validate() != nil || record.Sequence <= next || record.Sequence != next+1 || record.Sequence > p.HighWaterSequence {
			return errors.New("invalid audit page record or sequence")
		}
		next = record.Sequence
	}
	if p.NextSequence != next || p.HasMore != (next < p.HighWaterSequence) || len(p.Records) < int(query.PageLimit()) && p.HasMore {
		return errors.New("invalid audit page cursor")
	}
	return nil
}
