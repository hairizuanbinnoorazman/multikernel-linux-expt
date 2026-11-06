package network

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const MaxHealthReportBytes = 16 << 10

// canonicalHealthObject refuses case aliases as well as missing/null fields.
// Go's usual struct decoder folds case and can silently overwrite a failed
// CHECK code or degraded status with a second differently spelled key.
func canonicalHealthObject(data []byte, names ...string) error {
	var fields map[string]json.RawMessage
	if protocol.StrictDecode(data, &fields) != nil || len(fields) != len(names) {
		return errors.New("invalid canonical network health fields")
	}
	for _, name := range names {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("invalid canonical network health fields")
		}
	}
	return nil
}

func (r *HealthReport) UnmarshalJSON(data []byte) error {
	*r = HealthReport{}
	if len(data) > MaxHealthReportBytes {
		return errors.New("network health report exceeds its wire bound")
	}
	if err := canonicalHealthObject(data, "version", "source", "primary_boot_id", "status", "endpoints"); err != nil {
		return err
	}
	type wire HealthReport
	var decoded wire
	if protocol.StrictDecode(data, &decoded) != nil {
		return errors.New("invalid network health document")
	}
	value := HealthReport(decoded)
	if value.Validate() != nil {
		return errors.New("invalid network health values")
	}
	*r = value
	return nil
}

func (e *EndpointHealth) UnmarshalJSON(data []byte) error {
	*e = EndpointHealth{}
	if err := canonicalHealthObject(data, "endpoint_generation", "descriptor_sha256", "code"); err != nil {
		return err
	}
	type wire EndpointHealth
	var decoded wire
	if protocol.StrictDecode(data, &decoded) != nil {
		return errors.New("invalid network health endpoint")
	}
	*e = EndpointHealth(decoded)
	return nil
}
