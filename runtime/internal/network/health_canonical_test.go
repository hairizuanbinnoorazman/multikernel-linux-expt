package network

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestHealthWireCanonicalNamesCannotMaskDegradedState(t *testing.T) {
	valid := `{"version":1,"source":"host-network-backend-check","primary_boot_id":"00000000-0000-0000-0000-000000000001","status":"checked","endpoints":[{"endpoint_generation":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","descriptor_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","code":"CHECK_PASSED"}]}`
	for _, mode := range []string{"status-alias", "code-alias", "version-alias", "source-alias", "boot-alias", "endpoints-alias", "generation-alias", "descriptor-alias", "null-version", "null-source", "null-boot", "null-status", "null-endpoints", "null-code", "null-generation", "null-descriptor", "missing-code", "unknown-report", "unknown-endpoint", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			data := valid
			switch mode {
			case "status-alias":
				data = strings.Replace(data, `"status":"checked"`, `"status":"degraded","Status":"checked"`, 1)
			case "code-alias":
				data = strings.Replace(data, `"code":"CHECK_PASSED"`, `"code":"BACKEND_CHECK_FAILED","Code":"CHECK_PASSED"`, 1)
			case "version-alias":
				data = strings.Replace(data, `"version":1`, `"Version":1`, 1)
			case "source-alias":
				data = strings.Replace(data, `"source":`, `"Source":`, 1)
			case "boot-alias":
				data = strings.Replace(data, `"primary_boot_id":`, `"PRIMARY_BOOT_ID":`, 1)
			case "endpoints-alias":
				data = strings.Replace(data, `"endpoints":`, `"Endpoints":`, 1)
			case "generation-alias":
				data = strings.Replace(data, `"endpoint_generation":`, `"Endpoint_generation":`, 1)
			case "descriptor-alias":
				data = strings.Replace(data, `"descriptor_sha256":`, `"Descriptor_sha256":`, 1)
			case "null-version":
				data = strings.Replace(data, `"version":1`, `"version":null`, 1)
			case "null-source":
				data = strings.Replace(data, `"source":"host-network-backend-check"`, `"source":null`, 1)
			case "null-boot":
				data = strings.Replace(data, `"primary_boot_id":"00000000-0000-0000-0000-000000000001"`, `"primary_boot_id":null`, 1)
			case "null-status":
				data = strings.Replace(data, `"status":"checked"`, `"status":null`, 1)
			case "null-endpoints":
				data = strings.Replace(data, data[strings.Index(data, `[{`):len(data)-1], `null`, 1)
			case "null-code":
				data = strings.Replace(data, `"code":"CHECK_PASSED"`, `"code":null`, 1)
			case "null-generation":
				data = strings.Replace(data, `"endpoint_generation":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `"endpoint_generation":null`, 1)
			case "null-descriptor":
				data = strings.Replace(data, `"descriptor_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`, `"descriptor_sha256":null`, 1)
			case "missing-code":
				data = strings.Replace(data, `,"code":"CHECK_PASSED"`, "", 1)
			case "unknown-report":
				data = strings.Replace(data, `"version":1`, `"private-SYNTHETIC_TOKEN":1,"version":1`, 1)
			case "unknown-endpoint":
				data = strings.Replace(data, `"code":"CHECK_PASSED"`, `"code":"CHECK_PASSED","private-SYNTHETIC_TOKEN":1`, 1)
			case "oversize":
				data = strings.Replace(data, "{", "{"+strings.Repeat(" ", 16<<10), 1)
			}
			// Preload a healthy value: a failed decode must clear it, not retain a stale
			// previously successful snapshot. Ordinary json.Unmarshal must be safe too.
			var report HealthReport
			if err := json.Unmarshal([]byte(valid), &report); err != nil || report.Validate() != nil {
				t.Fatal("valid health wire rejected", err)
			}
			if !json.Valid([]byte(data)) {
				t.Fatal("invalid regression fixture")
			}
			err := json.Unmarshal([]byte(data), &report)
			if err == nil || report.Version != 0 || report.Endpoints != nil || strings.Contains(err.Error(), "SYNTHETIC_TOKEN") {
				t.Fatal("noncanonical health wire accepted, retained state or leaked", err)
			}
			if protocol.StrictDecode([]byte(data), &report) == nil {
				t.Fatal("strict client accepted ambiguous health wire")
			}
		})
	}
}

func TestHealthWireCanonicalRoundTripAndCardinality(t *testing.T) {
	for _, count := range []int{0, 1, MaxHealthEndpoints} {
		report := HealthReport{Version: 1, Source: "host-network-backend-check", PrimaryBootID: "00000000-0000-0000-0000-000000000001", Status: "idle", Endpoints: []EndpointHealth{}}
		if count > 0 {
			report.Status = "checked"
		}
		for i := range count {
			report.Endpoints = append(report.Endpoints, EndpointHealth{EndpointGeneration: fmt.Sprintf("%032x", i+1), DescriptorSHA256: strings.Repeat("b", 64), Code: "CHECK_PASSED"})
		}
		data, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		var decoded HealthReport
		if err = protocol.StrictDecode(data, &decoded); err != nil || decoded.Validate() != nil || len(decoded.Endpoints) != count {
			t.Fatal("valid bounded health round trip refused", count, err)
		}
	}
}
