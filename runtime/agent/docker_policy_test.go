package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDockerGuestIdentityPolicyAcrossLifecycle(t *testing.T) {
	manager := NewDockerGuestManager()
	if manager.NoChroot || !manager.dockerGuest {
		t.Fatal("Docker policy not fixed at construction")
	}
	// Exercise RPC-facing manager admission without privileged mount operations.
	manager.NoChroot = true
	valid := ProcessSpec{User: User{UID: 1000, GID: 1000}, Args: []string{"/probe"}, Cwd: "/"}
	b := bundle(t, []string{"/probe"}, "")
	write := func(spec ProcessSpec) {
		t.Helper()
		data := `{"ociVersion":"1.1.0","process":` + mustJSON(spec) + `,"root":{"path":"rootfs"}}`
		if e := os.WriteFile(filepath.Join(b, "config.json"), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write(valid)
	if e := manager.Create("init", b); e != nil {
		t.Fatal(e)
	}
	if e := manager.Exec("valid", "init", valid); e != nil {
		t.Fatal(e)
	}
	for _, spec := range []ProcessSpec{
		{User: User{UID: 0, GID: 0}, Args: valid.Args, Cwd: "/"},
		{User: User{UID: 1000, GID: 0}, Args: valid.Args, Cwd: "/"},
		{User: User{UID: 1000, GID: 1000, AdditionalGids: []uint32{0}}, Args: valid.Args, Cwd: "/"},
	} {
		write(spec)
		if e := manager.Create("refused", b); e == nil {
			t.Fatal("privileged create admitted")
		}
		if e := manager.Exec("refused", "init", spec); e == nil {
			t.Fatal("privileged exec admitted")
		}
	}
	// Admission cannot be bypassed by a retained process spec changing before
	// Start, which must refuse before executable validation or process creation.
	manager.processes["valid"].spec.User.UID = 0
	if e := manager.Start("valid"); e == nil || manager.processes["valid"].cmd != nil {
		t.Fatal("privileged start admitted")
	}
	if e := validateDockerGuestProcess(ProcessSpec{User: User{UID: 1000, GID: 1000}, Capabilities: map[string][]string{"effective": {"CAP_NET_RAW"}}}); e == nil {
		t.Fatal("capability request admitted")
	}
	if e := validateDockerGuestProcess(ProcessSpec{User: User{UID: 1000, GID: 1000}, Capabilities: map[string][]string{}}); e != nil {
		t.Fatal("empty non-root capability contract refused", e)
	}
	nnp := false
	if e := validateDockerGuestProcess(ProcessSpec{User: User{UID: 1000, GID: 1000}, NoNewPrivileges: &nnp}); e == nil {
		t.Fatal("NNP disabling admitted")
	}
}
