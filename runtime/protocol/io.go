package protocol

import (
	"context"
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

// CloseOnContext closes a blocking resource when ctx ends. The returned
// cleanup must be called when the operation finishes; it unregisters the
// callback or joins an already-running callback so neither goroutines nor
// resource references survive the operation.
func CloseOnContext(ctx context.Context, closer io.Closer) func() {
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = closer.Close()
		close(closed)
	})
	return func() {
		if !stop() {
			<-closed
		}
	}
}
