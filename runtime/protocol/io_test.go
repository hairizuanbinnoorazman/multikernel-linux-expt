package protocol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type observedCloser struct{ closed chan struct{} }

func (c observedCloser) Close() error {
	close(c.closed)
	return nil
}

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

func TestCloseOnContextCancelsActiveAndUnregistersCompletedOperations(t *testing.T) {
	t.Run("active", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		closed := make(chan struct{})
		cleanup := CloseOnContext(ctx, observedCloser{closed: closed})
		cancel()
		cleanup()
		select {
		case <-closed:
		default:
			t.Fatal("active closer was not invoked")
		}
	})
	t.Run("completed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		closed := make(chan struct{})
		cleanup := CloseOnContext(ctx, observedCloser{closed: closed})
		cleanup()
		cancel()
		select {
		case <-closed:
			t.Fatal("completed closer remained registered")
		case <-time.After(25 * time.Millisecond):
		}
	})
}
