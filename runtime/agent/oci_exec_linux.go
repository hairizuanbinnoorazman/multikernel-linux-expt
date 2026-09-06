//go:build linux

package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

type execConstraints struct {
	NoNewPrivileges bool                `json:"no_new_privileges"`
	Rlimits         []Rlimit            `json:"rlimits,omitempty"`
	CapabilitiesSet bool                `json:"capabilities_set"`
	Capabilities    map[string][]string `json:"capabilities,omitempty"`
}

var linuxCapabilities = map[string]uint{
	"CAP_CHOWN": 0, "CAP_DAC_OVERRIDE": 1, "CAP_DAC_READ_SEARCH": 2, "CAP_FOWNER": 3,
	"CAP_FSETID": 4, "CAP_KILL": 5, "CAP_SETGID": 6, "CAP_SETUID": 7,
	"CAP_SETPCAP": 8, "CAP_LINUX_IMMUTABLE": 9, "CAP_NET_BIND_SERVICE": 10,
	"CAP_NET_BROADCAST": 11, "CAP_NET_ADMIN": 12, "CAP_NET_RAW": 13, "CAP_IPC_LOCK": 14,
	"CAP_IPC_OWNER": 15, "CAP_SYS_MODULE": 16, "CAP_SYS_RAWIO": 17, "CAP_SYS_CHROOT": 18,
	"CAP_SYS_PTRACE": 19, "CAP_SYS_PACCT": 20, "CAP_SYS_ADMIN": 21, "CAP_SYS_BOOT": 22,
	"CAP_SYS_NICE": 23, "CAP_SYS_RESOURCE": 24, "CAP_SYS_TIME": 25, "CAP_SYS_TTY_CONFIG": 26,
	"CAP_MKNOD": 27, "CAP_LEASE": 28, "CAP_AUDIT_WRITE": 29, "CAP_AUDIT_CONTROL": 30,
	"CAP_SETFCAP": 31, "CAP_MAC_OVERRIDE": 32, "CAP_MAC_ADMIN": 33, "CAP_SYSLOG": 34,
	"CAP_WAKE_ALARM": 35, "CAP_BLOCK_SUSPEND": 36, "CAP_AUDIT_READ": 37,
	"CAP_PERFMON": 38, "CAP_BPF": 39, "CAP_CHECKPOINT_RESTORE": 40,
}

var rlimitResources = map[string]int{
	"RLIMIT_AS": unix.RLIMIT_AS, "RLIMIT_CORE": unix.RLIMIT_CORE, "RLIMIT_CPU": unix.RLIMIT_CPU,
	"RLIMIT_DATA": unix.RLIMIT_DATA, "RLIMIT_FSIZE": unix.RLIMIT_FSIZE, "RLIMIT_LOCKS": unix.RLIMIT_LOCKS,
	"RLIMIT_MEMLOCK": unix.RLIMIT_MEMLOCK, "RLIMIT_MSGQUEUE": unix.RLIMIT_MSGQUEUE,
	"RLIMIT_NICE": unix.RLIMIT_NICE, "RLIMIT_NOFILE": unix.RLIMIT_NOFILE,
	"RLIMIT_NPROC": unix.RLIMIT_NPROC, "RLIMIT_RSS": unix.RLIMIT_RSS,
	"RLIMIT_RTPRIO": unix.RLIMIT_RTPRIO, "RLIMIT_RTTIME": unix.RLIMIT_RTTIME,
	"RLIMIT_SIGPENDING": unix.RLIMIT_SIGPENDING, "RLIMIT_STACK": unix.RLIMIT_STACK,
}

func capabilityMask(values []string) ([2]uint32, error) {
	var result [2]uint32
	seen := map[string]bool{}
	for _, name := range values {
		value, ok := linuxCapabilities[name]
		if !ok || seen[name] {
			return result, fmt.Errorf("unknown or duplicate Linux capability %q", name)
		}
		seen[name] = true
		result[value/32] |= uint32(1) << (value % 32)
	}
	return result, nil
}

func validateExecConstraints(process ProcessSpec) error {
	if len(process.Rlimits) > 32 {
		return errors.New("too many process rlimits")
	}
	seenLimits := map[string]bool{}
	for _, limit := range process.Rlimits {
		if _, ok := rlimitResources[limit.Type]; !ok || seenLimits[limit.Type] || limit.Soft > limit.Hard {
			return fmt.Errorf("unsupported, duplicate, or invalid rlimit %q", limit.Type)
		}
		seenLimits[limit.Type] = true
	}
	if process.Capabilities == nil {
		return nil
	}
	for name, values := range process.Capabilities {
		switch name {
		case "bounding", "effective", "inheritable", "permitted", "ambient":
		default:
			return fmt.Errorf("unsupported capability set %q", name)
		}
		if _, err := capabilityMask(values); err != nil {
			return err
		}
	}
	if process.User.UID != 0 {
		return errors.New("non-root processes may not provide a Linux capability contract")
	}
	bounding, _ := capabilityMask(process.Capabilities["bounding"])
	permitted, _ := capabilityMask(process.Capabilities["permitted"])
	effective, _ := capabilityMask(process.Capabilities["effective"])
	inheritable, _ := capabilityMask(process.Capabilities["inheritable"])
	ambient, _ := capabilityMask(process.Capabilities["ambient"])
	for i := range bounding {
		if permitted[i]&^bounding[i] != 0 || effective[i]&^permitted[i] != 0 || ambient[i]&^(permitted[i]&inheritable[i]) != 0 {
			return errors.New("Linux capability sets violate bounding, permitted, effective, or ambient containment")
		}
	}
	return nil
}

func encodeExecConstraints(process ProcessSpec) (string, error) {
	value := execConstraints{Rlimits: process.Rlimits, CapabilitiesSet: process.Capabilities != nil, Capabilities: process.Capabilities}
	if process.NoNewPrivileges != nil {
		value.NoNewPrivileges = *process.NoNewPrivileges
	}
	data, err := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data), err
}

// RunOCIExec applies constraints in a short-lived re-exec of mk-agent and
// replaces itself with the workload. It returns only on validation/syscall
// failure. Keeping this outside the long-lived multi-threaded agent avoids
// unsafe post-fork hooks while retaining Go's audited chroot/credential path.
func RunOCIExec(encoded string, argv []string) error {
	if len(argv) == 0 || !strings.HasPrefix(argv[0], "/") {
		return errors.New("OCI executor requires an absolute workload argv[0]")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(data) > 64<<10 {
		return errors.New("OCI executor constraints are malformed")
	}
	var constraints execConstraints
	if err = json.Unmarshal(data, &constraints); err != nil {
		return errors.New("OCI executor constraints are malformed")
	}
	process := ProcessSpec{NoNewPrivileges: &constraints.NoNewPrivileges, Rlimits: constraints.Rlimits, Capabilities: constraints.Capabilities,
		User: User{UID: uint32(os.Getuid())}}
	if err = validateExecConstraints(process); err != nil {
		return err
	}
	for _, limit := range constraints.Rlimits {
		if err = unix.Setrlimit(rlimitResources[limit.Type], &unix.Rlimit{Cur: limit.Soft, Max: limit.Hard}); err != nil {
			return fmt.Errorf("apply %s: %w", limit.Type, err)
		}
	}
	if constraints.CapabilitiesSet {
		bounding, maskErr := capabilityMask(constraints.Capabilities["bounding"])
		if maskErr != nil {
			return maskErr
		}
		for capability := uint(0); capability <= unix.CAP_LAST_CAP; capability++ {
			if bounding[capability/32]&(uint32(1)<<(capability%32)) == 0 {
				if err = unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(capability), 0, 0, 0); err != nil {
					return fmt.Errorf("drop bounding capability %d: %w", capability, err)
				}
			}
		}
		effective, _ := capabilityMask(constraints.Capabilities["effective"])
		permitted, _ := capabilityMask(constraints.Capabilities["permitted"])
		inheritable, _ := capabilityMask(constraints.Capabilities["inheritable"])
		capabilityData := [2]unix.CapUserData{
			{Effective: effective[0], Permitted: permitted[0], Inheritable: inheritable[0]},
			{Effective: effective[1], Permitted: permitted[1], Inheritable: inheritable[1]},
		}
		if err = unix.Capset(&unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}, &capabilityData[0]); err != nil {
			return fmt.Errorf("apply process capabilities: %w", err)
		}
		ambient, _ := capabilityMask(constraints.Capabilities["ambient"])
		if err = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
			return fmt.Errorf("clear ambient capabilities: %w", err)
		}
		for capability := uint(0); capability <= unix.CAP_LAST_CAP; capability++ {
			if ambient[capability/32]&(uint32(1)<<(capability%32)) != 0 {
				if err = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_RAISE, uintptr(capability), 0, 0); err != nil {
					return fmt.Errorf("raise ambient capability %d: %w", capability, err)
				}
			}
		}
	}
	if constraints.NoNewPrivileges {
		if err = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
			return fmt.Errorf("apply no-new-privileges: %w", err)
		}
	}
	return unix.Exec(argv[0], argv, os.Environ())
}
