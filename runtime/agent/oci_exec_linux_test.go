//go:build linux

package agent

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOCIExecAppliesNoNewPrivilegesAndRlimit(t *testing.T) {
	marker := t.TempDir() + "/observed"
	value := true
	encoded, err := encodeExecConstraints(ProcessSpec{NoNewPrivileges: &value,
		Rlimits: []Rlimit{{Type: "RLIMIT_NOFILE", Soft: 64, Hard: 64}}})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestOCIExecReexecHelper$")
	command.Env = append(os.Environ(), "MK_OCI_EXEC_CONSTRAINTS="+encoded, "MK_OCI_EXEC_MARKER="+marker)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("constrained re-exec: %v: %s", err, output)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "nnp=1 nofile=64\n" {
		t.Fatalf("observed constraints = %q, %v", data, err)
	}
}

func TestOCIExecReexecHelper(t *testing.T) {
	encoded := os.Getenv("MK_OCI_EXEC_CONSTRAINTS")
	if encoded == "" {
		return
	}
	err := RunOCIExec(encoded, []string{"/proc/self/exe", "-test.run=^TestOCIExecObservedHelper$"})
	t.Fatal(err)
}

func TestOCIExecObservedHelper(t *testing.T) {
	marker := os.Getenv("MK_OCI_EXEC_MARKER")
	if marker == "" {
		return
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	nnp := ""
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "NoNewPrivs:") {
			nnp = strings.TrimSpace(strings.TrimPrefix(line, "NoNewPrivs:"))
		}
	}
	var limit unix.Rlimit
	if err = unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(marker, []byte("nnp="+nnp+" nofile="+strconv.FormatUint(limit.Cur, 10)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestOCIConstraintValidationRejectsUnsafeValues(t *testing.T) {
	for _, process := range []ProcessSpec{
		{Rlimits: []Rlimit{{Type: "RLIMIT_UNKNOWN", Soft: 1, Hard: 1}}},
		{Rlimits: []Rlimit{{Type: "RLIMIT_NOFILE", Soft: 2, Hard: 1}}},
		{Capabilities: map[string][]string{"effective": {"CAP_UNKNOWN"}}},
		{Capabilities: map[string][]string{"unknown": {}}},
		{User: User{UID: 1000}, Capabilities: map[string][]string{"effective": {"CAP_CHOWN"}}},
	} {
		if err := validateExecConstraints(process); err == nil {
			t.Fatalf("unsafe constraints accepted: %+v", process)
		}
	}
}
