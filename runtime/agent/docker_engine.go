package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var ErrDockerEngineObservationUnsupported = errors.New("agent does not advertise docker-engine-observation-v1")

// InspectDockerEngine negotiates over the existing authenticated connection.
// It neither reconnects/replays nor falls back to executing workload commands.
func (c *Client) InspectDockerEngine(ctx context.Context) (protocol.DockerEngineObservation, error) {
	return InspectDockerEngine(ctx, c)
}

// InspectDockerEngine permits an owner to use its existing narrow agent caller.
// It has the same no-reconnect/no-replay contract as the concrete client method.
func InspectDockerEngine(ctx context.Context, caller interface {
	CallContext(context.Context, string, any, any) error
}) (protocol.DockerEngineObservation, error) {
	if ctx == nil {
		return protocol.DockerEngineObservation{}, errors.New("Docker observation context is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var caps map[string]json.RawMessage
	if err := caller.CallContext(ctx, "Capabilities", map[string]any{}, &caps); err != nil {
		return protocol.DockerEngineObservation{}, err
	}
	var version int
	var features []string
	if json.Unmarshal(caps["protocol"], &version) != nil || version != 1 ||
		json.Unmarshal(caps["protocol_features"], &features) != nil || features == nil || len(features) > 32 {
		return protocol.DockerEngineObservation{}, errors.New("invalid Docker observation capabilities")
	}
	seen := map[string]bool{}
	for _, feature := range features {
		if feature == "" || len(feature) > 64 || seen[feature] {
			return protocol.DockerEngineObservation{}, errors.New("invalid Docker observation capabilities")
		}
		seen[feature] = true
	}
	if !slices.Contains(features, protocol.DockerEngineObservationCapability) {
		return protocol.DockerEngineObservation{}, ErrDockerEngineObservationUnsupported
	}
	var report protocol.DockerEngineObservation
	if err := caller.CallContext(ctx, "InspectDockerEngine", map[string]any{}, &report); err != nil {
		return protocol.DockerEngineObservation{}, err
	}
	if ctx.Err() != nil || report.Validate() != nil {
		return protocol.DockerEngineObservation{}, errors.New("invalid Docker engine observation")
	}
	return report, nil
}
