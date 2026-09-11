package protocol

import (
	"errors"
	"io"
)

// WriteFull writes every byte or returns an error. It defensively completes a
// short successful write and rejects a writer that makes no progress, so a
// faulty or wrapped transport cannot silently truncate protocol framing.
func WriteFull(writer io.Writer, data []byte) error {
	for len(data) != 0 {
		written, err := writer.Write(data)
		if written < 0 || written > len(data) {
			return errors.New("writer returned an invalid byte count")
		}
		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}
