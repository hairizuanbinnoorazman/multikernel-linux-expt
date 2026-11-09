//go:build linux

package main

import (
	"errors"
	"testing"
)

func TestNamespaceHolderStartupCompletionRetainsFailedCleanup(t *testing.T) {
	for _, kind := range []string{"success", "failed-clean", "failed-retained"} {
		t.Run(kind, func(t *testing.T) {
			cause := errors.New("private startup cause")
			stopErr := errors.New("private cleanup cause")
			holder := &namespaceDiagnosticHolderFixture{}
			if kind == "success" {
				cause = nil
			}
			if kind == "failed-retained" {
				holder.stop = stopErr
			}
			result, err := finishNamespaceHolderStart(holder, cause)
			if kind == "success" {
				if err != nil || result != holder || holder.stops != 0 {
					t.Fatal("successful startup changed")
				}
				return
			}
			if !errors.Is(err, cause) || holder.stops != 1 {
				t.Fatal("failed startup lost cause or cleanup attempt")
			}
			if kind == "failed-clean" {
				if result != nil {
					t.Fatal("completed cleanup retained ownership")
				}
				return
			}
			if !errors.Is(err, stopErr) || result == nil || result.PID() != 0 || result.Validate("/run/netns/private") == nil {
				t.Fatal("failed cleanup lost unready ownership")
			}
			holder.stop = nil
			if err = result.Stop(); err != nil || holder.stops != 2 {
				t.Fatal("cleanup retry did not use exact retained handle")
			}
		})
	}
}
