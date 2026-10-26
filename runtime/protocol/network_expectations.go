package protocol

import "errors"

const NetworkExpectationsCapability = "network-expectations-v1"
const NetworkExpectationLabel = "io.multikernel.network-expectation-v1"

type LifecycleNetworkExpectation struct {
	SandboxID   string `json:"sandbox_id"`
	Generation  string `json:"generation"`
	Expectation string `json:"expectation"`
}
type NetworkExpectationsReport struct {
	Version      int                           `json:"version"`
	Source       string                        `json:"source"`
	Lifecycle    OwnershipReport               `json:"lifecycle"`
	Expectations []LifecycleNetworkExpectation `json:"expectations"`
}

// Only the reserved intent label is interpreted. No other label is copied into
// diagnostics, and absence in legacy configurations remains explicitly unknown.
func NetworkExpectation(config SandboxConfig) (string, error) {
	value, ok := config.Labels[NetworkExpectationLabel]
	if !ok {
		return "unknown", nil
	}
	if value != "required" && value != "none" {
		return "", errors.New("invalid network expectation label")
	}
	return value, nil
}
func (r NetworkExpectationsReport) Validate() error {
	if r.Version != 1 || r.Source != "durable-lifecycle-network" || r.Lifecycle.Validate() != nil || r.Expectations == nil || len(r.Expectations) != len(r.Lifecycle.Sandboxes) {
		return errors.New("invalid lifecycle network expectations")
	}
	for i, e := range r.Expectations {
		owner := r.Lifecycle.Sandboxes[i]
		if e.SandboxID != owner.SandboxID || e.Generation != owner.Generation || (e.Expectation != "unknown" && e.Expectation != "required" && e.Expectation != "none") {
			return errors.New("invalid lifecycle network expectation binding")
		}
	}
	return nil
}
