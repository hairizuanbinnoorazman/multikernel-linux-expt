//go:build linux

package dockerengine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type probeRoundTripper func(*http.Request) (*http.Response, error)

func (f probeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type completionBody struct {
	io.Reader
	complete func()
}

func (b completionBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.complete()
	}
	return n, err
}

func (completionBody) Close() error { return nil }

// Cancellation at the final EOF deterministically exercises the gap between
// successfully reading /info and publishing readiness, without timing sleeps.
func TestReadinessCompletionBudget(t *testing.T) {
	for _, mode := range []string{"live", "cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			if mode == "deadline" {
				ctx, cancel = context.WithTimeout(t.Context(), time.Second)
			} else {
				ctx, cancel = context.WithCancel(t.Context())
			}
			defer cancel()
			complete := func() {
				switch mode {
				case "cancel":
					cancel()
				case "deadline":
					<-ctx.Done()
				}
			}
			requests, closed := 0, 0
			client := &http.Client{Transport: probeRoundTripper(func(r *http.Request) (*http.Response, error) {
				requests++
				body := "OK"
				if r.URL.Path == "/info" {
					body = validInfo
				}
				finish := func() {
					closed++
					if r.URL.Path == "/info" {
						complete()
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: completionBody{strings.NewReader(body), finish}}, nil
			})}
			err := probeAPI(ctx, client)
			if mode == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("completed canceled probe: %v", err)
				}
			} else if mode == "deadline" {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("completed expired probe: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if requests != 2 || closed != 2 {
				t.Fatalf("incomplete fixed observations: requests=%d EOFs=%d", requests, closed)
			}
		})
	}
}
