//go:build linux

package main

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestDockerStopsBeforeStorageAndFailedStopRetainsStorage(t *testing.T) {
	var calls []string
	stop := func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded shutdown")
		}
		calls = append(calls, "stop-engine")
		return nil
	}
	quiesce := func() error { calls = append(calls, "quiesce-storage"); return nil }
	if err := stopDockerBeforeStorage(stop, quiesce); err != nil || !slices.Equal(calls, []string{"stop-engine", "quiesce-storage"}) {
		t.Fatal(calls, err)
	}
	calls = nil
	failed := func(context.Context) error {
		calls = append(calls, "stop-engine")
		return errors.New("private daemon output")
	}
	err := stopDockerBeforeStorage(failed, quiesce)
	if err == nil || err.Error() != "Docker service has not quiesced" || !slices.Equal(calls, []string{"stop-engine"}) {
		t.Fatal(calls, err)
	}
}
