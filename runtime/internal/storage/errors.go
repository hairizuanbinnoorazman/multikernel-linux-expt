package storage

import "errors"

// ErrUnsafeSession requires operator diagnosis. Process absence alone cannot
// authorize a new session or release storage without authenticated synced close.
var ErrUnsafeSession = errors.New("OPERATOR_ACTION: storage session lacks safe recovery or release evidence")
