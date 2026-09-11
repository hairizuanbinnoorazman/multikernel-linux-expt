package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type boundedWriter struct {
	bytes.Buffer
	maximum int
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	if len(data) > w.maximum {
		data = data[:w.maximum]
	}
	return w.Buffer.Write(data)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriteFullCompletesShortWritesAndRejectsNoProgress(t *testing.T) {
	writer := &boundedWriter{maximum: 2}
	if err := WriteFull(writer, []byte("complete-frame")); err != nil {
		t.Fatal(err)
	}
	if writer.String() != "complete-frame" {
		t.Fatalf("written frame = %q", writer.String())
	}
	if err := WriteFull(zeroWriter{}, []byte("blocked")); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("zero-progress error = %v", err)
	}
}
