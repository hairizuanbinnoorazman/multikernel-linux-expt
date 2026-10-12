package agent

import (
	"errors"
	"fmt"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Error categories are assigned where the failure occurs. Diagnostic wording
// never determines the wire code, and causes remain available to local callers.
type errorKind uint8

const (
	invalidArgument errorKind = iota
	unsupported
	notFound
	alreadyExists
	failedPrecondition
	unauthenticated
	frameTooLarge
)

type operationError struct {
	kind  errorKind
	cause error
}

func (e *operationError) Error() string { return e.cause.Error() }
func (e *operationError) Unwrap() error { return e.cause }

func errorf(kind errorKind, format string, args ...any) error {
	return &operationError{kind: kind, cause: fmt.Errorf(format, args...)}
}

// stageError retains an execution failure's cause without exposing paths,
// command output, or tokens to a remote caller.
type stageError struct {
	operation string
	stage     string
	cause     error
}

func (e *stageError) Error() string {
	return fmt.Sprintf("guest %s stage %s failed: %v", e.operation, e.stage, e.cause)
}
func (e *stageError) Unwrap() error { return e.cause }

func (e *stageError) publicMessage() string {
	// Only fixed implementation stages may cross the wire. Unknown stages
	// fall back to INTERNAL rather than publishing arbitrary diagnostic text.
	switch e.operation {
	case "process start":
		switch e.stage {
		case "executable", "constraints", "terminal", "stdio", "exec":
			return "guest process start failed at " + e.stage
		}
	case "process create":
		switch e.stage {
		case "bundle-load", "root-policy":
			return "guest process creation failed at " + e.stage
		}
	case "network setup":
		switch e.stage {
		case "tun-open", "tun-request", "tun-create", "address", "link", "route", "dns":
			return "guest network setup failed at " + e.stage
		}
	}
	return "agent operation failed"
}

func safeAgentError(sequence uint64, err error) *protocol.Error {
	code, message := "INTERNAL", "agent operation failed"
	var stage *stageError
	var operation *operationError
	// A failed execution stage remains INTERNAL even when its cause describes
	// a validation failure; the stage is the stable public diagnostic.
	if errors.As(err, &stage) {
		message = stage.publicMessage()
	} else if errors.As(err, &operation) {
		switch operation.kind {
		case invalidArgument:
			code, message = "INVALID_ARGUMENT", "invalid agent request"
		case unsupported:
			code, message = "UNSUPPORTED", "agent operation is unsupported"
		case notFound:
			code, message = "NOT_FOUND", "managed process was not found"
		case alreadyExists:
			code, message = "ALREADY_EXISTS", "managed process already exists"
		case failedPrecondition:
			code, message = "FAILED_PRECONDITION", "managed process is in the wrong state"
		case unauthenticated:
			code, message = "UNAUTHENTICATED", "agent authentication failed"
		case frameTooLarge:
			code, message = "INVALID_ARGUMENT", "agent frame is too large"
		}
	}
	return &protocol.Error{Code: code, Message: message, OperationID: fmt.Sprintf("agent-%d", sequence), Retryable: false}
}

func (r *Reply) setError(err error) {
	r.cause = err
	r.Error = err.Error()
	r.Body = nil
}
