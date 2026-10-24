//go:build linux

package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep the exact native transcript from the GCE pressure/recovery run. Its
// export identity is rebound only in the initial marker to the test lease.
func TestNativeTimingCloseEvidence(t *testing.T) {
	data, err := os.ReadFile("testdata/native-pressure-close.log")
	if err != nil {
		t.Fatal(err)
	}
	body := data[bytes.IndexByte(data, '\n')+1:]
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "server.log")
	if err := os.WriteFile(path, append(readyMarker(value), body...), 0600); err != nil {
		t.Fatal(err)
	}
	counters, err := parseCounters(path, value)
	if err != nil {
		t.Fatal(err)
	}
	if counters != (Counters{Reads: 2033, ReadBytes: 256696320, Writes: 2110, WrittenBytes: 1598365696, Flushes: 798}) {
		t.Fatalf("counter mismatch: %+v", counters)
	}
	for name, replacement := range map[string]string{
		"unknown phase":                  strings.Replace(string(body), "phase=image-sync", "phase=unknown", 1),
		"duplicate phase":                strings.Replace(string(body), "phase=read-payload calls=2033", "phase=write-payload calls=2033", 1),
		"missing summary":                strings.Replace(string(body), "MKNBD_SERVER_IO_SUMMARY phase=reply calls=4941 errors=0 slow=0 max_us=28\n", "", 1),
		"noncanonical":                   strings.Replace(string(body), "calls=2110", "calls=02110", 1),
		"errors exceed calls":            strings.Replace(string(body), "calls=2110 errors=0", "calls=2110 errors=2111", 1),
		"summary contradicts detail":     strings.Replace(string(body), "slow=3 max_us=4712186", "slow=2 max_us=4712186", 1),
		"summary max contradicts detail": strings.Replace(string(body), "max_us=4712186", "max_us=4712185", 1),
		"unexplained suppression":        strings.Replace(string(body), "MKNBD_SERVER_CLOSED ", "MKNBD_SERVER_IO_SUPPRESSED count=1\nMKNBD_SERVER_CLOSED ", 1),
		"detail beyond summaries":        strings.Replace(string(body), "MKNBD_SERVER_CLOSED ", "MKNBD_SERVER_IO phase=image-sync duration_us=4712186 result=0 errno=0\nMKNBD_SERVER_CLOSED ", 1),
		"unknown line":                   strings.Replace(string(body), "MKNBD_SERVER_CLOSED ", "unknown\nMKNBD_SERVER_CLOSED ", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, append(readyMarker(value), []byte(replacement)...), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := parseCounters(path, value); err == nil {
				t.Fatal("invalid timing history authorized close")
			}
		})
	}
}

func TestTimingBoundsPreserveLifecycleAuthority(t *testing.T) {
	summary := func(calls, slow uint64) string {
		var result strings.Builder
		for _, phase := range storageIOPhases {
			c, s, m := uint64(0), uint64(0), uint64(0)
			if phase == "image-sync" {
				c, s, m = calls, slow, 300000
			}
			fmt.Fprintf(&result, "MKNBD_SERVER_IO_SUMMARY phase=%s calls=%d errors=0 slow=%d max_us=%d\n", phase, c, s, m)
		}
		return result.String()
	}
	detail := "MKNBD_SERVER_IO phase=image-sync duration_us=300000 result=0 errno=0\n"
	prefix := "MKNBD_SERVER_REFUSED reason=identity\nMKNBD_SERVER_REFUSAL_SUMMARY total=1 details=1 suppressed=0\n"
	good := prefix + strings.Repeat(detail, 16) + summary(20, 20) + "MKNBD_SERVER_IO_SUPPRESSED count=4\n"
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	if err := validateClosedStorageBody([]byte(good), Counters{}, value); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(good, "count=4", "count=3", 1),
		strings.Replace(good, "MKNBD_SERVER_IO_SUPPRESSED count=4\n", "", 1),
		strings.Replace(good, "errno=0", "errno=1", 1),
		strings.Replace(good, "result=0 errno=0", "result=-1 errno=4096", 1),
		strings.Replace(good, "duration_us=300000", "duration_us=249999", 1),
		prefix + strings.Repeat(detail, 17) + summary(20, 20) + "MKNBD_SERVER_IO_SUPPRESSED count=3\n",
		good + "MKNBD_SERVER_CLIENT_ACCEPTED\n",
		strings.Replace(good, "phase=image-read calls=0 errors=0 slow=0 max_us=0", "phase=image-read calls=1 errors=0 slow=0 max_us=1", 1),
	} {
		if err := validateClosedStorageBody([]byte(bad), Counters{}, value); err == nil {
			t.Fatal("invalid or reordered timing evidence authorized close")
		}
	}
	if err := validateClosedStorageBody([]byte(good), Counters{Reads: 1, ReadBytes: 1}, value); err == nil {
		t.Fatal("timings authorized IO before acceptance")
	}
	if saturatedIOAdd(^uint64(0)-1, 5) != ^uint64(0) {
		t.Fatal("aggregate counter wrapped")
	}
}
