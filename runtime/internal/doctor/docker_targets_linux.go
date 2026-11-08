//go:build linux

package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// EngineTarget is private operator input, never part of a public report.
type EngineTarget struct {
	Socket string             `json:"socket"`
	Owner  inspection.Request `json:"owner"`
}

func (t EngineTarget) valid() bool {
	path := strings.TrimPrefix(t.Socket, "unix://")
	if t.Owner.Validate() != nil || len(t.Socket) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

func exactTargetKeys(data []byte, keys ...string) bool {
	var fields map[string]json.RawMessage
	if protocol.StrictDecode(data, &fields) != nil || len(fields) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	return true
}

func DecodeEngineTargets(data []byte) ([]EngineTarget, error) {
	fail := func() ([]EngineTarget, error) { return nil, errors.New("invalid private engine target snapshot") }
	if len(data) > 1<<20 || !exactTargetKeys(data, "version", "targets") {
		return fail()
	}
	var wire struct {
		Version int               `json:"version"`
		Targets []json.RawMessage `json:"targets"`
	}
	if protocol.StrictDecode(data, &wire) != nil || wire.Version != 1 || wire.Targets == nil || len(wire.Targets) > protocol.MaxOwnershipRecords {
		return fail()
	}
	targets := make([]EngineTarget, 0, len(wire.Targets))
	seen := map[string]bool{}
	for _, raw := range wire.Targets {
		if !exactTargetKeys(raw, "socket", "owner") {
			return fail()
		}
		var entry struct {
			Socket string          `json:"socket"`
			Owner  json.RawMessage `json:"owner"`
		}
		if protocol.StrictDecode(raw, &entry) != nil {
			return fail()
		}
		owner, err := inspection.DecodeRequest(entry.Owner)
		if err != nil {
			return fail()
		}
		target := EngineTarget{Socket: entry.Socket, Owner: owner}
		if !target.valid() || seen[owner.SandboxID] {
			return fail()
		}
		seen[owner.SandboxID] = true
		targets = append(targets, target)
	}
	return targets, nil
}

// LoadEngineTargets binds a bounded, exclusive-owned snapshot file. It neither
// discovers task sockets nor falls back to configuration/state serialization.
func LoadEngineTargets(ctx context.Context, path string) ([]EngineTarget, error) {
	fail := func() ([]EngineTarget, error) { return nil, errors.New("private engine targets unavailable") }
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fail()
	}
	directory, err := safefile.OpenDirectory(filepath.Dir(path), false)
	if err != nil {
		return fail()
	}
	defer directory.Close()
	data, identity, err := directory.ReadRegularIdentity(filepath.Base(path), 0600, 1<<20)
	if err != nil || identity.Mode&07777 != 0600 || ctx.Err() != nil {
		return fail()
	}
	targets, err := DecodeEngineTargets(data)
	if err != nil || ctx.Err() != nil {
		return fail()
	}
	return targets, nil
}
