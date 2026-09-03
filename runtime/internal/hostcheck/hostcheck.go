// Package hostcheck performs a read-only qualification of a Multikernel host.
package hostcheck

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = 1

type Options struct {
	Root              string
	Kerf              string
	MinPrimaryCPUs    int
	MinPrimaryMemByte uint64
	Timeout           time.Duration
}

type CPU struct {
	Logical  int  `json:"logical"`
	APIC     int  `json:"apic"`
	Physical int  `json:"physical_id"`
	Core     int  `json:"core_id"`
	NUMA     int  `json:"numa_node"`
	Online   bool `json:"online"`
}

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Report struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Qualified     bool              `json:"qualified"`
	Architecture  string            `json:"architecture"`
	KernelRelease string            `json:"kernel_release"`
	KernelConfig  map[string]string `json:"kernel_config"`
	MultikernelFS bool              `json:"multikernel_fs"`
	KerfVersion   string            `json:"kerf_version,omitempty"`
	CPUs          []CPU             `json:"cpus"`
	OnlineCPUs    string            `json:"online_cpus"`
	MemoryBytes   uint64            `json:"memory_bytes"`
	Lockdown      string            `json:"lockdown"`
	SecureBoot    string            `json:"secure_boot"`
	KexecLoaded   string            `json:"kexec_loaded"`
	GuestAgent    string            `json:"guest_agent"`
	Instances     []string          `json:"instances"`
	PCIClasses    map[string]string `json:"pci_classes"`
	Findings      []Finding         `json:"findings"`
}

func DefaultOptions() Options {
	return Options{Root: "/", Kerf: "kerf", MinPrimaryCPUs: 4, MinPrimaryMemByte: 8 << 30, Timeout: 5 * time.Second}
}

func path(root, name string) string {
	if root == "" || root == "/" {
		return name
	}
	return filepath.Join(root, strings.TrimPrefix(name, "/"))
}

func read(root, name string) string {
	b, err := os.ReadFile(path(root, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func Check(ctx context.Context, o Options) Report {
	if o.Root == "" {
		o.Root = "/"
	}
	if o.MinPrimaryCPUs == 0 {
		o.MinPrimaryCPUs = 4
	}
	if o.MinPrimaryMemByte == 0 {
		o.MinPrimaryMemByte = 8 << 30
	}
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Second
	}
	r := Report{SchemaVersion: SchemaVersion, GeneratedAt: time.Now().UTC(), Architecture: runtime.GOARCH, KernelConfig: map[string]string{}, PCIClasses: map[string]string{}}
	r.KernelRelease = read(o.Root, "/proc/sys/kernel/osrelease")
	r.OnlineCPUs = read(o.Root, "/sys/devices/system/cpu/online")
	r.MemoryBytes = memBytes(read(o.Root, "/proc/meminfo"))
	r.Lockdown = valueOrUnknown(read(o.Root, "/sys/kernel/security/lockdown"))
	r.KexecLoaded = valueOrUnknown(read(o.Root, "/sys/kernel/kexec_loaded"))
	r.SecureBoot = secureBoot(o.Root)
	r.MultikernelFS = dirExists(path(o.Root, "/sys/fs/multikernel"))
	r.CPUs = parseCPUs(read(o.Root, "/proc/cpuinfo"), r.OnlineCPUs, o.Root)
	logicalIDs := map[int]bool{}
	apicIDs := map[int]bool{}
	for _, cpu := range r.CPUs {
		if logicalIDs[cpu.Logical] {
			r.add("CPU_LOGICAL_DUPLICATE", "error", fmt.Sprintf("logical CPU %d is reported more than once", cpu.Logical))
		}
		if apicIDs[cpu.APIC] {
			r.add("CPU_APIC_DUPLICATE", "error", fmt.Sprintf("APIC ID %d is reported more than once", cpu.APIC))
		}
		logicalIDs[cpu.Logical] = true
		apicIDs[cpu.APIC] = true
	}
	r.Instances = directoryNames(path(o.Root, "/sys/fs/multikernel/instances"))
	r.PCIClasses = pciClasses(o.Root)
	r.KernelConfig = kernelConfig(o.Root, r.KernelRelease)
	r.GuestAgent = guestAgent(o.Root)
	if o.Root == "/" && o.Kerf != "" {
		cctx, cancel := context.WithTimeout(ctx, o.Timeout)
		defer cancel()
		out, err := exec.CommandContext(cctx, o.Kerf, "--version").CombinedOutput()
		if err == nil {
			r.KerfVersion = strings.TrimSpace(string(out))
		}
	}
	required := []string{"CONFIG_MULTIKERNEL", "CONFIG_KEXEC_CORE", "CONFIG_KEXEC_FILE", "CONFIG_MKTTY"}
	allConfig := r.KernelConfig
	r.KernelConfig = map[string]string{}
	for _, k := range required {
		r.KernelConfig[k] = allConfig[k]
	}
	if r.Architecture != "amd64" {
		r.add("ARCH_UNSUPPORTED", "error", "only amd64 is qualified")
	}
	if !strings.Contains(r.KernelRelease, "mk2") {
		r.add("KERNEL_UNSUPPORTED", "error", "running kernel is not a qualified mk2 release")
	}
	for _, k := range required {
		if r.KernelConfig[k] != "y" {
			r.add("KCONFIG_MISSING", "error", k+"=y is required")
		}
	}
	if !r.MultikernelFS {
		r.add("MULTIKERNEL_FS_MISSING", "error", "/sys/fs/multikernel is unavailable")
	}
	onlineCPUCount := 0
	for _, cpu := range r.CPUs {
		if cpu.Online {
			onlineCPUCount++
		}
	}
	if onlineCPUCount < o.MinPrimaryCPUs {
		r.add("PRIMARY_CPU_HEADROOM", "error", fmt.Sprintf("need at least %d online primary CPUs", o.MinPrimaryCPUs))
	}
	if r.MemoryBytes < o.MinPrimaryMemByte {
		r.add("PRIMARY_MEMORY_HEADROOM", "error", fmt.Sprintf("need at least %d bytes primary memory", o.MinPrimaryMemByte))
	}
	for _, c := range r.CPUs {
		if c.APIC == 0 && !c.Online {
			r.add("APIC0_OFFLINE", "error", "APIC ID 0 must remain with the primary")
		}
	}
	if len(r.Instances) != 0 {
		r.add("EXISTING_INSTANCES", "error", "existing Multikernel instances require operator reconciliation")
	}
	if r.GuestAgent != "active" {
		r.add("GUEST_AGENT", "warning", "Google guest agent is not confirmed active")
	}
	if r.SecureBoot == "enabled" {
		r.add("SECURE_BOOT", "error", "unsigned child loading is incompatible with Secure Boot")
	}
	r.Qualified = true
	for _, f := range r.Findings {
		if f.Severity == "error" {
			r.Qualified = false
		}
	}
	return r
}

func (r *Report) add(code, severity, message string) {
	r.Findings = append(r.Findings, Finding{code, severity, message})
}
func valueOrUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
func dirExists(p string) bool { s, err := os.Stat(p); return err == nil && s.IsDir() }

func memBytes(s string) uint64 {
	for _, l := range strings.Split(s, "\n") {
		var kb uint64
		if _, err := fmt.Sscanf(l, "MemTotal: %d kB", &kb); err == nil {
			return kb * 1024
		}
	}
	return 0
}

func parseCPUs(s, online, root string) []CPU {
	on := cpuSet(online)
	var out []CPU
	for _, block := range strings.Split(strings.TrimSpace(s), "\n\n") {
		m := map[string]int{}
		scanner := bufio.NewScanner(strings.NewReader(block))
		for scanner.Scan() {
			p := strings.SplitN(scanner.Text(), ":", 2)
			if len(p) != 2 {
				continue
			}
			n, e := strconv.Atoi(strings.TrimSpace(p[1]))
			if e == nil {
				m[strings.TrimSpace(p[0])] = n
			}
		}
		logical, ok := m["processor"]
		if !ok {
			continue
		}
		numa := -1
		nodes, _ := filepath.Glob(path(root, fmt.Sprintf("/sys/devices/system/cpu/cpu%d/node*", logical)))
		if len(nodes) > 0 {
			fmt.Sscanf(filepath.Base(nodes[0]), "node%d", &numa)
		}
		out = append(out, CPU{Logical: logical, APIC: m["apicid"], Physical: m["physical id"], Core: m["core id"], NUMA: numa, Online: on[logical]})
	}
	return out
}

func cpuSet(s string) map[int]bool {
	r := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		a := strings.SplitN(strings.TrimSpace(part), "-", 2)
		lo, e := strconv.Atoi(a[0])
		if e != nil {
			continue
		}
		hi := lo
		if len(a) == 2 {
			hi, _ = strconv.Atoi(a[1])
		}
		for i := lo; i <= hi; i++ {
			r[i] = true
		}
	}
	return r
}

func directoryNames(p string) []string {
	es, _ := os.ReadDir(p)
	var r []string
	for _, e := range es {
		if e.IsDir() {
			r = append(r, e.Name())
		}
	}
	sort.Strings(r)
	return r
}

func kernelConfig(root, release string) map[string]string {
	r := map[string]string{}
	candidates := []string{path(root, "/boot/config-"+release), path(root, "/proc/config")}
	for _, p := range candidates {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "CONFIG_") {
				x := strings.SplitN(l, "=", 2)
				if len(x) == 2 {
					r[x[0]] = x[1]
				}
			}
		}
		break
	}
	return r
}

func secureBoot(root string) string {
	files, _ := filepath.Glob(path(root, "/sys/firmware/efi/efivars/SecureBoot-*"))
	if len(files) == 0 {
		return "unknown"
	}
	b, e := os.ReadFile(files[0])
	if e != nil || len(b) < 5 {
		return "unknown"
	}
	if b[4] == 1 {
		return "enabled"
	}
	return "disabled"
}

func guestAgent(root string) string {
	if root != "/" {
		return valueOrUnknown(read(root, "/run/google-guest-agent.status"))
	}
	c := exec.Command("systemctl", "is-active", "google-guest-agent")
	b, e := c.Output()
	if e != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(b))
}

func pciClasses(root string) map[string]string {
	r := map[string]string{}
	files, _ := filepath.Glob(path(root, "/sys/bus/pci/devices/*/class"))
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e == nil {
			r[filepath.Base(filepath.Dir(f))] = strings.TrimSpace(string(b))
		}
	}
	return r
}

func Encode(r Report) ([]byte, error) {
	b, e := json.MarshalIndent(r, "", "  ")
	return append(b, '\n'), e
}
func ValidateRequestedAPICs(r Report, ids []int, minPrimary int) error {
	seen := map[int]bool{}
	online := map[int]bool{}
	for _, c := range r.CPUs {
		if c.Online {
			online[c.APIC] = true
		}
	}
	for _, id := range ids {
		if id == 0 {
			return errors.New("APIC ID 0 is forbidden")
		}
		if seen[id] {
			return fmt.Errorf("duplicate APIC ID %d", id)
		}
		seen[id] = true
		if !online[id] {
			return fmt.Errorf("APIC ID %d is not online", id)
		}
	}
	if len(online)-len(ids) < minPrimary {
		return errors.New("allocation violates primary CPU headroom")
	}
	return nil
}

// IsReadOnlyOperation is a testable inventory of all host-check operations.
func IsReadOnlyOperation(name string) bool {
	return !bytes.Contains([]byte(name), []byte("write")) && name != "kerf init" && name != "kerf create"
}
