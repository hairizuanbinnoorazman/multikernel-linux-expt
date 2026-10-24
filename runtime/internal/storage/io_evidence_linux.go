//go:build linux

package storage

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

var storageIOPhases = [...]string{"write-payload", "read-payload", "image-write", "image-read", "image-sync", "reply"}

type storageIOCounts struct{ calls, errors, slow, maximum uint64 }

func storageIOPhase(name string) int {
	for i, phase := range storageIOPhases {
		if name == phase {
			return i
		}
	}
	return -1
}
func saturatedIOAdd(a, b uint64) uint64 {
	if b > ^uint64(0)-a {
		return ^uint64(0)
	}
	return a + b
}

// Timing records are observations, never close authority. Validate their whole
// bounded history before handing the remaining lifecycle to the existing exact
// readiness/acceptance/counter validator. Legacy helpers emit no timing records.
func validatedStorageIOLifecycle(body []byte) ([]byte, error) {
	bad := errors.New("storage I/O timing evidence has invalid semantics or ordering")
	var lifecycle bytes.Buffer
	var details, samples [len(storageIOPhases)]storageIOCounts
	detailCount, summaries := 0, 0
	accepted, refusalDone, suppressedSeen, anyTiming := false, false, false, false
	var suppressed uint64
	for len(body) > 0 {
		line, rest, complete := bytes.Cut(body, []byte("\n"))
		if !complete {
			return nil, bad
		}
		text := string(line)
		switch {
		case strings.HasPrefix(text, "MKNBD_SERVER_IO phase="):
			anyTiming = true
			var phase string
			var duration uint64
			var result int64
			var number uint32
			n, err := fmt.Sscanf(text, "MKNBD_SERVER_IO phase=%s duration_us=%d result=%d errno=%d", &phase, &duration, &result, &number)
			index := storageIOPhase(phase)
			if err != nil || n != 4 || index < 0 || summaries != 0 || detailCount >= 16 ||
				((!accepted || refusalDone) && phase != "image-sync") ||
				result < -1 || number > 4095 || (result < 0) != (number != 0) ||
				(result >= 0 && duration < 250000) ||
				text != fmt.Sprintf("MKNBD_SERVER_IO phase=%s duration_us=%d result=%d errno=%d", phase, duration, result, number) {
				return nil, bad
			}
			maxResult := int64(1)
			if phase == "image-write" || phase == "image-read" {
				maxResult = storageRequestLimit
			}
			if phase == "image-sync" {
				maxResult = 0
			}
			if result > maxResult {
				return nil, bad
			}
			d := &details[index]
			d.calls++
			if result < 0 {
				d.errors++
			}
			if duration >= 250000 {
				d.slow++
			}
			d.maximum = max(d.maximum, duration)
			detailCount++
		case strings.HasPrefix(text, "MKNBD_SERVER_IO_SUMMARY "):
			anyTiming = true
			var phase string
			var sample storageIOCounts
			n, err := fmt.Sscanf(text, "MKNBD_SERVER_IO_SUMMARY phase=%s calls=%d errors=%d slow=%d max_us=%d", &phase, &sample.calls, &sample.errors, &sample.slow, &sample.maximum)
			if err != nil || n != 5 || summaries >= len(storageIOPhases) || phase != storageIOPhases[summaries] ||
				sample.errors > sample.calls || sample.slow > sample.calls ||
				(sample.calls == 0 && sample.maximum != 0) || (sample.slow > 0) != (sample.maximum >= 250000) ||
				text != fmt.Sprintf("MKNBD_SERVER_IO_SUMMARY phase=%s calls=%d errors=%d slow=%d max_us=%d", phase, sample.calls, sample.errors, sample.slow, sample.maximum) {
				return nil, bad
			}
			d := details[summaries]
			if sample.calls < d.calls || sample.errors < d.errors || sample.slow < d.slow || sample.maximum < d.maximum {
				return nil, bad
			}
			if !accepted && phase != "image-sync" && sample.calls != 0 {
				return nil, bad
			}
			samples[summaries] = sample
			summaries++
		case strings.HasPrefix(text, "MKNBD_SERVER_IO_SUPPRESSED "):
			anyTiming = true
			n, err := fmt.Sscanf(text, "MKNBD_SERVER_IO_SUPPRESSED count=%d", &suppressed)
			if err != nil || n != 1 || suppressed == 0 || suppressedSeen || summaries != len(storageIOPhases) || detailCount != 16 || text != fmt.Sprintf("MKNBD_SERVER_IO_SUPPRESSED count=%d", suppressed) {
				return nil, bad
			}
			suppressedSeen = true
		default:
			if summaries != 0 {
				return nil, bad
			}
			if text == "MKNBD_SERVER_CLIENT_ACCEPTED" {
				accepted = true
			}
			if strings.HasPrefix(text, "MKNBD_SERVER_REFUSAL_SUMMARY ") {
				refusalDone = true
			}
			lifecycle.Write(line)
			lifecycle.WriteByte('\n')
		}
		body = rest
	}
	if anyTiming {
		if summaries != len(storageIOPhases) {
			return nil, bad
		}
		var lower, upper uint64
		for _, sample := range samples {
			// Errors and slow calls overlap. Their union must explain every event;
			// arithmetic matches the native helper's saturating counters.
			lower = saturatedIOAdd(lower, max(sample.errors, sample.slow))
			upper = saturatedIOAdd(upper, min(sample.calls, saturatedIOAdd(sample.errors, sample.slow)))
		}
		events := saturatedIOAdd(uint64(detailCount), suppressed)
		if events < lower || events > upper {
			return nil, bad
		}
	}
	return lifecycle.Bytes(), nil
}
