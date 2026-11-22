package protocol

import "errors"

const ProcessOutputAccountingCapability = "process-output-accounting-v1"
const ProcessOutputBufferLimit = 4 << 20

// OutputBufferAccounting describes retained payload bytes, not allocated heap,
// RSS, FIFO delivery or Docker log retention. Pending writes are included so a
// concurrent observation can still conserve every admitted input byte.
type OutputBufferAccounting struct {
	ObservedBytes       uint64 `json:"observed_bytes"`
	AcknowledgedBytes   uint64 `json:"acknowledged_bytes"`
	BufferedBytes       uint64 `json:"buffered_bytes"`
	PendingBytes        uint64 `json:"pending_bytes"`
	DroppedBytes        uint64 `json:"dropped_bytes"`
	BufferedPeakBytes   uint64 `json:"buffered_peak_bytes"`
	LimitBytes          uint64 `json:"limit_bytes"`
	Truncated           bool   `json:"truncated"`
	AccountingExhausted bool   `json:"accounting_exhausted"`
}

func (r OutputBufferAccounting) Validate() error {
	if r.AccountingExhausted || r.LimitBytes != ProcessOutputBufferLimit || r.BufferedBytes > r.LimitBytes || r.BufferedPeakBytes > r.LimitBytes || r.BufferedPeakBytes < r.BufferedBytes || r.BufferedPeakBytes > r.ObservedBytes || r.Truncated != (r.DroppedBytes != 0) {
		return errors.New("output buffer accounting is unqualified")
	}
	remaining := r.ObservedBytes
	for _, count := range []uint64{r.AcknowledgedBytes, r.BufferedBytes, r.PendingBytes, r.DroppedBytes} {
		if count > remaining {
			return errors.New("output buffer accounting does not conserve bytes")
		}
		remaining -= count
	}
	if remaining != 0 {
		return errors.New("output buffer accounting does not conserve bytes")
	}
	return nil
}

// Each stream is a coherent snapshot; stdout and stderr are sampled separately.
// This observation never grants process, completion or cleanup authority.
type ProcessOutputAccounting struct {
	Version uint32                 `json:"version"`
	ID      string                 `json:"id"`
	PID     int                    `json:"pid"`
	Status  string                 `json:"status"`
	Stdout  OutputBufferAccounting `json:"stdout"`
	Stderr  OutputBufferAccounting `json:"stderr"`
}

func (r ProcessOutputAccounting) Validate(id string, pid int) error {
	if r.Version != 1 || id == "" || len(id) > 128 || r.ID != id || r.PID != pid || pid < 0 || pid >= 1<<31 {
		return errors.New("output accounting process identity differs")
	}
	switch r.Status {
	case "CREATED":
		if pid != 0 || r.Stdout.ObservedBytes != 0 || r.Stderr.ObservedBytes != 0 {
			return errors.New("created output accounting has a running PID")
		}
	case "RUNNING", "PAUSED", "STOPPED":
		if pid == 0 {
			return errors.New("output accounting omits process PID")
		}
	default:
		return errors.New("output accounting status differs")
	}
	if err := r.Stdout.Validate(); err != nil {
		return err
	}
	return r.Stderr.Validate()
}
