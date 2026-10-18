//go:build linux

package storage

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

const storageRefusalDetails = 16
const storageRequestLimit = 4 << 20

// Validate every record preceding the terminal close, rather than allowing a
// last matching counter line to override an earlier ambiguous lifecycle.
func validateClosedStorageBody(body []byte, counters Counters, value Export) error {
	details := uint32(0)
	suppressed, accepted, diagnostic, summary := false, false, false, false
	bad := errors.New("storage close evidence has invalid record semantics or ordering")
	for len(body) > 0 {
		line, rest, complete := bytes.Cut(body, []byte("\n"))
		if !complete || summary {
			return bad
		}
		text := string(line)
		switch text {
		case "MKNBD_SERVER_REFUSED reason=identity", "MKNBD_SERVER_REFUSED reason=handshake-io":
			if accepted || suppressed || details >= storageRefusalDetails {
				return bad
			}
			details++
		case "MKNBD_SERVER_REFUSALS_SUPPRESSED further_details=omitted":
			if accepted || suppressed || details != storageRefusalDetails {
				return bad
			}
			suppressed = true
		case "MKNBD_SERVER_CLIENT_ACCEPTED":
			if accepted {
				return bad
			}
			accepted = true
		default:
			switch {
			case strings.HasPrefix(text, "MKNBD_SERVER_REFUSAL_SUMMARY "):
				var total, omitted uint64
				var reported uint32
				n, err := fmt.Sscanf(text, "MKNBD_SERVER_REFUSAL_SUMMARY total=%d details=%d suppressed=%d", &total, &reported, &omitted)
				if err != nil || n != 3 || details == 0 || reported != details || total < uint64(details) || omitted != total-uint64(details) || (omitted > 0) != suppressed || text != fmt.Sprintf("MKNBD_SERVER_REFUSAL_SUMMARY total=%d details=%d suppressed=%d", total, reported, omitted) {
					return bad
				}
				summary = true
			case strings.HasPrefix(text, "MKNBD_SERVER_IO_FAILED "):
				var number uint32
				n, err := fmt.Sscanf(text, "MKNBD_SERVER_IO_FAILED phase=request errno=%d", &number)
				if !accepted || diagnostic || err != nil || n != 1 || number == 0 || number > 4095 || text != fmt.Sprintf("MKNBD_SERVER_IO_FAILED phase=request errno=%d", number) {
					return bad
				}
				diagnostic = true
			case strings.HasPrefix(text, "MKNBD_SERVER_DISCONNECT_DURING_WRITE "):
				var length uint32
				var offset uint64
				n, err := fmt.Sscanf(text, "MKNBD_SERVER_DISCONNECT_DURING_WRITE len=%d offset=%d", &length, &offset)
				if !accepted || diagnostic || err != nil || n != 2 || length == 0 || length > storageRequestLimit || offset > value.SizeBytes || uint64(length) > value.SizeBytes-offset || text != fmt.Sprintf("MKNBD_SERVER_DISCONNECT_DURING_WRITE len=%d offset=%d", length, offset) {
					return bad
				}
				diagnostic = true
			default:
				return bad
			}
		}
		body = rest
	}
	if (details > 0) != summary || !accepted && counters != (Counters{}) {
		return bad
	}
	// Ceil(bytes/max) avoids overflowing count*max and permits the protocol's
	// zero-length operations. No byte total may be attributed to zero operations.
	limit := uint64(storageRequestLimit)
	if value.SizeBytes < limit {
		limit = value.SizeBytes
	}
	if limit == 0 {
		return bad
	}
	for _, pair := range [][2]uint64{{counters.Reads, counters.ReadBytes}, {counters.Writes, counters.WrittenBytes}} {
		needed := pair[1] / limit
		if pair[1]%limit != 0 {
			needed++
		}
		if needed > pair[0] {
			return bad
		}
	}
	return nil
}
