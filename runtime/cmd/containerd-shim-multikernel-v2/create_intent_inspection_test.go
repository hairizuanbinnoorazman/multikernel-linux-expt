//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCreateIntentInspectionObservesWithoutReadingOrMutatingContents(t *testing.T) {
	for _, kind := range []string{"absent", "private-file", "symlink", "directory", "fifo", "quarantine", "inventory-bound", "unsafe-bundle"} {
		t.Run(kind, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			path := filepath.Join(bundle, createIntentName)
			code, status := "CREATE_INTENT_PENDING", "pending"
			switch kind {
			case "absent":
				code, status = "CREATE_INTENT_NOT_OBSERVED", "absent"
			case "private-file":
				if err := os.WriteFile(path, []byte("SYNTHETIC_PRIVATE_MARKER_DATA"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("/SYNTHETIC_PRIVATE_TARGET", path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "quarantine":
				digest := sha256.Sum256([]byte(createIntentName))
				if err := os.WriteFile(filepath.Join(bundle, ".mklinux-remove-"+hex.EncodeToString(digest[:8])+"-unknown"), []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "inventory-bound":
				for i := 0; i < 257; i++ {
					if err := os.WriteFile(filepath.Join(bundle, fmt.Sprint(i)), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
				code, status = "CREATE_INTENT_UNAVAILABLE", "inconclusive"
			case "unsafe-bundle":
				if err := os.Chmod(bundle, 0770); err != nil {
					t.Fatal(err)
				}
				code, status = "CREATE_INTENT_UNAVAILABLE", "inconclusive"
			}
			before, statErr := os.Lstat(path)
			r := observeCreateIntent(bundle, nil)
			if r != intentInspection(status, code) {
				t.Fatal("wrong fixed observation")
			}
			var out bytes.Buffer
			if !writeIntentInspection(&out, r) {
				t.Fatal("report emission failed")
			}
			var fields map[string]any
			if json.Unmarshal(out.Bytes(), &fields) != nil || len(fields) != 9 || bytes.Contains(out.Bytes(), []byte("SYNTHETIC_PRIVATE")) || bytes.Contains(out.Bytes(), []byte(bundle)) {
				t.Fatal("unsafe report schema")
			}
			after, afterErr := os.Lstat(path)
			if statErr == nil && (afterErr != nil || !os.SameFile(before, after) || before.Mode() != after.Mode()) {
				t.Fatal("inspection changed entry")
			}
			if kind == "private-file" {
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != "SYNTHETIC_PRIVATE_MARKER_DATA" {
					t.Fatal("inspection changed private contents")
				}
			}
		})
	}
}

func TestCreateIntentInspectionRefusesObservedIdentityDrift(t *testing.T) {
	for _, kind := range []string{"marker", "bundle"} {
		t.Run(kind, func(t *testing.T) {
			base := privateTestDirectory(t)
			bundle := filepath.Join(base, "bundle")
			if err := os.Mkdir(bundle, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bundle, createIntentName)
			if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
				t.Fatal(err)
			}
			report := observeCreateIntent(bundle, func() {
				if kind == "marker" {
					if err := os.Rename(path, path+".held"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("foreign"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Rename(bundle, bundle+".held"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(bundle, 0700); err != nil {
						t.Fatal(err)
					}
				}
			})
			if report != intentInspection("inconclusive", "CREATE_INTENT_CHANGED") {
				t.Fatal("identity drift accepted")
			}
		})
	}
}

type intentShortWriter struct{}

func (intentShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type intentFailureWriter struct{}

func (intentFailureWriter) Write([]byte) (int, error) {
	return 0, errors.New("SYNTHETIC_PRIVATE_ERROR")
}

func TestCreateIntentInspectionBoundsObservationAndOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false
	if r := observeIntentBudget(ctx, func() createIntentInspection {
		called = true
		return intentInspection("absent", "CREATE_INTENT_NOT_OBSERVED")
	}); r.Code != "CREATE_INTENT_TIMEOUT" || called {
		t.Fatal("canceled inspection ran filesystem source")
	}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	result := make(chan createIntentInspection, 1)
	go func() {
		result <- observeIntentBudget(ctx, func() createIntentInspection {
			close(started)
			<-release
			close(finished)
			return intentInspection("absent", "CREATE_INTENT_NOT_OBSERVED")
		})
	}()
	<-started
	cancel()
	select {
	case r := <-result:
		if r.Code != "CREATE_INTENT_TIMEOUT" {
			t.Fatal("blocked observation claimed success")
		}
	case <-time.After(time.Second):
		t.Fatal("observation did not cancel")
	}
	close(release)
	<-finished
	report := intentInspection("pending", "CREATE_INTENT_PENDING")
	if writeIntentInspection(intentShortWriter{}, report) || writeIntentInspection(intentFailureWriter{}, report) {
		t.Fatal("failed output claimed delivery")
	}
	blocked := &diagnosticBlockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan bool, 1)
	go func() { done <- writeIntentInspection(blocked, report) }()
	<-blocked.entered
	select {
	case ok := <-done:
		if ok {
			t.Fatal("blocked output claimed delivery")
		}
	case <-time.After(time.Second):
		t.Fatal("output did not time out")
	}
	close(blocked.release)
}

func TestCreateIntentInspectionCLIArgumentsStayPrivate(t *testing.T) {
	for _, args := range [][]string{nil, {"--bundle"}, {"--bundle", "relative/SYNTHETIC_PRIVATE"}, {"--bundle", "/valid", "--bundle", "/SYNTHETIC_PRIVATE"}, {"--SYNTHETIC_PRIVATE", "/valid"}} {
		var out bytes.Buffer
		if runCreateIntentInspection(args, &out) != 2 || strings.Contains(out.String(), "SYNTHETIC_PRIVATE") {
			t.Fatal("invalid arguments exposed private input or lost status")
		}
	}
	bundle := privateTestDirectory(t)
	var out bytes.Buffer
	if runCreateIntentInspection([]string{"--bundle", bundle}, &out) != 0 || !strings.Contains(out.String(), "CREATE_INTENT_NOT_OBSERVED") {
		t.Fatal("valid CLI inspection failed")
	}
}
