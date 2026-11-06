//go:build linux

package doctor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"encoding/json"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostconfig"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

const healthCLIMap = "0000000000000000-00000003ffffffff : System RAM\n  0000000100000000-00000002ffffffff : Multikernel Memory Pool\n"

// Only an explicitly spawned CLONE_NEWNS helper may bind the synthetic memory
// map. Parent namespace identity is checked before any mount operation. Exec
// retains only this private namespace; exit releases it without host unmounts.
func TestInstalledHealthCLIChild(t *testing.T) {
	if os.Getenv("MK_TEST_HEALTH_CLI_CHILD") != "1" {
		t.Skip("internal private mount namespace helper")
	}
	root, cli := os.Getenv("MK_TEST_HEALTH_CLI_ROOT"), os.Getenv("MK_TEST_HEALTH_CLI")
	parentNS := os.Getenv("MK_TEST_HEALTH_PARENT_MOUNT_NS")
	ownNS, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil || os.Geteuid() != 0 || parentNS == "" || ownNS == parentNS || !strings.HasPrefix(parentNS, "mnt:[") {
		t.Fatal("private child mount namespace required")
	}
	directory, err := safefile.OpenDirectory(root, false)
	if err != nil {
		t.Fatal("private CLI fixture refused")
	}
	defer directory.Close()
	source, _, err := directory.OpenPrivateFile("iomem.synthetic", 4096, false)
	if err != nil {
		t.Fatal("synthetic memory source refused")
	}
	defer source.Close()
	data, err := io.ReadAll(io.LimitReader(source, 4097))
	if err != nil || string(data) != healthCLIMap {
		t.Fatal("unexpected synthetic memory map")
	}
	if err = unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		t.Fatal("private propagation refused", err)
	}
	if err = unix.Mount(fmt.Sprintf("/proc/self/fd/%d", source.Fd()), "/proc/iomem", "", unix.MS_BIND, ""); err != nil {
		t.Fatal("private memory map bind refused", err)
	}
	if err = unix.Mount("", "/proc/iomem", "", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		t.Fatal("private memory map seal refused", err)
	}
	args := []string{cli, "--config=" + filepath.Join(root, "host.json"), "--network-socket=" + filepath.Join(root, "network.sock")}
	switch mode := os.Getenv("MK_TEST_HEALTH_CLI_FORMAT"); mode {
	case "5", "7", "8":
		args = append(args, "--doctor-all", "--doctor-format="+mode)
	case "default":
		args = append(args, "--doctor-all")
	case "legacy-bundle":
		args = append(args, "--diagnostic-bundle="+filepath.Join(root, "legacy-bundles/legacy.json"))
	case "health-bundle-idle", "health-bundle-pending", "health-bundle-failed", "health-bundle-full":
		args = append(args, "--diagnostic-bundle="+filepath.Join(root, "health-bundles", mode+".json"), "--diagnostic-format=2", "--diagnostic-max-files=3", "--diagnostic-max-bytes=1048576")
	default:
		t.Fatal("unexpected CLI format")
	}
	if err = unix.Exec(cli, args, []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}); err != nil {
		t.Fatal("private CLI exec failed", err)
	}
}

func healthCLICPUTree() []byte {
	var body, names bytes.Buffer
	word := func(v uint32) { _ = binary.Write(&body, binary.BigEndian, v) }
	pad := func() {
		for body.Len()%4 != 0 {
			body.WriteByte(0)
		}
	}
	node := func(name string) { word(1); body.WriteString(name); body.WriteByte(0); pad() }
	prop := func(name string, data []byte) {
		offset := names.Len()
		names.WriteString(name)
		names.WriteByte(0)
		word(3)
		word(uint32(len(data)))
		word(uint32(offset))
		body.Write(data)
		pad()
	}
	node("")
	prop("compatible", []byte("multikernel-v1\x00"))
	prop("id", []byte{0, 0, 0, 0})
	node("resources")
	cells := make([]byte, 16)
	binary.BigEndian.PutUint64(cells, 8)
	binary.BigEndian.PutUint64(cells[8:], 9)
	prop("cpus", cells)
	prop("cpus-available", cells)
	word(2)
	word(2)
	word(9)
	result := append(make([]byte, 56), body.Bytes()...)
	result = append(result, names.Bytes()...)
	header := []uint32{0xd00dfeed, uint32(len(result)), 56, uint32(56 + body.Len()), 40, 17, 16, 0, uint32(names.Len()), uint32(body.Len())}
	for i, v := range header {
		binary.BigEndian.PutUint32(result[i*4:], v)
	}
	return result
}

type healthCLIOutput struct{ bytes.Buffer }

func (b *healthCLIOutput) Write(data []byte) (int, error) {
	if len(data) > (1<<20)-b.Len() {
		return 0, errors.New("CLI fixture output limit")
	}
	return b.Buffer.Write(data)
}

func newInstalledHealthCLIFixture(t *testing.T) (string, *healthServices) {
	t.Helper()
	cli := os.Getenv("MK_TEST_HEALTH_CLI")
	if cli == "" || os.Getenv("MK_TEST_HEALTH_FIXTURE_PARENT") == "" {
		t.Skip("explicit pinned CLI and protected fixture parent required")
	}
	if os.Geteuid() != 0 || !filepath.IsAbs(cli) || buildinfo.Revision == "unknown" {
		t.Fatal("installed CLI qualification requires root and pinned build labels")
	}
	f := newHealthServices(t)
	for _, dir := range []string{"sysfs", "sysfs/instances", "manifests"} {
		if err := os.Mkdir(filepath.Join(f.root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string][]byte{"iomem.synthetic": []byte(healthCLIMap), "sysfs/device_tree": healthCLICPUTree(), "must-not-execute-kerf": []byte("#!/bin/sh\n: > " + filepath.Join(f.root, "unexpected-helper-execution") + "\nexit 77\n")} {
		if err := os.WriteFile(filepath.Join(f.root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(f.root, "must-not-execute-kerf"), 0500); err != nil {
		t.Fatal(err)
	}
	config := hostconfig.Config{SchemaVersion: 1, StateDirectory: filepath.Join(f.root, "must-not-open-local-state"), SocketPath: f.runtimeSocket, KerfExecutable: filepath.Join(f.root, "must-not-execute-kerf"), MultikernelSysfsRoot: filepath.Join(f.root, "sysfs"), KernelManifestDirectory: filepath.Join(f.root, "manifests"), MinPrimaryCPUs: 1, MinPrimaryMemoryBytes: 512 << 20, ForbiddenAPICIDs: []int{0}, BackendTimeoutSeconds: 10, MaxFrameSizeBytes: 1 << 20}
	data, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(f.root, "host.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return cli, f
}

func runHealthCLI(t *testing.T, f *healthServices, cli, format string, wantExit int, wantStderr string) []byte {
	parentNS, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	parentMap, err := os.ReadFile("/proc/iomem")
	if err != nil {
		t.Fatal(err)
	}
	parentHash := sha256.Sum256(parentMap)
	t.Helper()
	before, stateBefore := f.files(t), f.store.Snapshot()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledHealthCLIChild$")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "MK_TEST_HEALTH_CLI_CHILD=1", "MK_TEST_HEALTH_CLI=" + cli, "MK_TEST_HEALTH_CLI_ROOT=" + f.root, "MK_TEST_HEALTH_PARENT_MOUNT_NS=" + parentNS, "MK_TEST_HEALTH_CLI_FORMAT=" + format}
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWNS}
	cmd.WaitDelay = time.Second
	var stdout, stderr healthCLIOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	exit := 0
	var failure *exec.ExitError
	if errors.As(err, &failure) {
		exit = failure.ExitCode()
	} else if err != nil {
		t.Fatal("private CLI helper failed to run", err)
	}
	if exit != wantExit || stderr.String() != wantStderr || ctx.Err() != nil {
		t.Fatalf("CLI format %s exit=%d expected=%d stderr-bytes=%d stdout-bytes=%d", format, exit, wantExit, stderr.Len(), stdout.Len())
	}
	if bytes.Contains(stdout.Bytes(), []byte("SYNTHETIC_HEALTH_PRIVATE_CANARY")) {
		t.Fatal("private fixture metadata leaked")
	}
	if !reflect.DeepEqual(before, f.files(t)) || !reflect.DeepEqual(stateBefore, f.store.Snapshot()) {
		t.Fatal("CLI repaired or mutated service state")
	}
	nowNS, _ := os.Readlink("/proc/self/ns/mnt")
	nowMap, e := os.ReadFile("/proc/iomem")
	if e != nil || nowNS != parentNS || sha256.Sum256(nowMap) != parentHash {
		t.Fatal("child simulation changed parent mount namespace or memory map")
	}
	return append([]byte(nil), stdout.Bytes()...)
}

func TestInstalledHealthCLIProductionRPCAndJournalFailure(t *testing.T) {
	cli, f := newInstalledHealthCLIFixture(t)
	observations := f.history.observations.Load()
	run := func(format string, wantExit int) []byte { return runHealthCLI(t, f, cli, format, wantExit, "") }

	check := func(phase string, wantExit int, wantStatus string) []byte {
		t.Helper()
		data := run("8", wantExit)
		var report HealthReport
		if protocol.StrictDecode(data, &report) != nil || report.Version != 8 || report.Status != wantStatus || report.Journal == nil || report.Network == nil || report.Network.Status != "idle" || report.Operational == nil || report.AdmissionGuaranteed {
			t.Fatalf("CLI v8 phase %s did not preserve report contract", phase)
		}
		pool := report.Operational.Unified.Pool
		if wantStatus == "consistent" {
			if len(pool.CPUCandidates) != 2 || pool.MemoryCandidateUpperBoundBytes != 7<<30 {
				t.Fatal("positive CLI lost observed capacity")
			}
		} else if len(pool.CPUCandidates) != 0 || pool.MemoryCandidateUpperBoundBytes != 0 {
			t.Fatal("CLI exposed candidates during journal trouble")
		}
		if wantStatus == "degraded" && (!report.Journal.JournalFailed || !hasFinding(report.Findings, "JOURNAL_FAILED")) {
			t.Fatal("CLI masked failed journal")
		}
		if phase == "pending" && report.Journal.Status != "pending" {
			t.Fatal("CLI masked pending journal")
		}
		return data
	}
	check("idle", 0, "consistent")
	default5, explicit5 := run("default", 0), run("5", 0)
	if !bytes.Equal(default5, explicit5) {
		t.Fatal("default v5 changed")
	}
	var legacy Report
	if protocol.StrictDecode(default5, &legacy) != nil || legacy.Version != 5 {
		t.Fatal("legacy format did not remain v5")
	}
	f.pending(t)
	check("pending", 1, "inconclusive")
	f.failJournal(t)
	failed := check("failed", 1, "degraded")
	repeat := check("repeat-failed", 1, "degraded")
	if !bytes.Equal(failed, repeat) {
		t.Fatal("repeated failed-journal inspection changed")
	}
	var v7 OperationalReport
	if protocol.StrictDecode(run("7", 0), &v7) != nil || v7.Version != 7 || v7.Status != "consistent" {
		t.Fatal("legacy v7 scope changed")
	}
	if f.history.observations.Load() != observations || f.network.calls.Load() != 0 {
		t.Fatal("CLI invoked a resource backend")
	}
	for _, path := range []string{"must-not-open-local-state", "unexpected-helper-execution", "never-open-storage-runtime"} {
		if _, err := os.Lstat(filepath.Join(f.root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("CLI executed helper or opened local state")
		}
	}
	t.Log("CLI_V8_IDLE_EXIT0_PENDING_EXIT1_FAILED_REPEAT_EXIT1_V7_SCOPE_AND_DEFAULT_V5_PRESERVED")
	t.Log("CHILD_SIMULATED_KERNEL_INPUTS_PRIVATE_PARENT_NAMESPACE_AND_ALL_SERVICE_STATE_UNCHANGED")
}
