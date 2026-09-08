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
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
	"golang.org/x/sys/unix"
)

var pciBDF = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)

const SchemaVersion = 1

type Options struct {
	Root              string
	Kerf              string
	MinPrimaryCPUs    int
	MinPrimaryMemByte uint64
	Timeout           time.Duration
	ProbeCPUs         []int
	ProbeMemory       string
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

type Instance struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Devices []string `json:"devices"`
}

type ProtectedDevice struct {
	Kind        string   `json:"kind"`
	Device      string   `json:"device"`
	PCIFunction string   `json:"pci_function"`
	Ancestry    []string `json:"pci_ancestry"`
}

type Report struct {
	SchemaVersion         int               `json:"schema_version"`
	GeneratedAt           time.Time         `json:"generated_at"`
	Qualified             bool              `json:"qualified"`
	Architecture          string            `json:"architecture"`
	KernelRelease         string            `json:"kernel_release"`
	KernelConfig          map[string]string `json:"kernel_config"`
	MultikernelFS         bool              `json:"multikernel_fs"`
	KerfVersion           string            `json:"kerf_version,omitempty"`
	CPUs                  []CPU             `json:"cpus"`
	SMTPolicy             string            `json:"smt_policy"`
	OnlineCPUs            string            `json:"online_cpus"`
	OfflineCPUs           string            `json:"offline_cpus"`
	MemoryBytes           uint64            `json:"memory_bytes"`
	ContiguousAllocation  string            `json:"contiguous_allocation"`
	ContiguousProbe       string            `json:"contiguous_probe,omitempty"`
	Lockdown              string            `json:"lockdown"`
	SecureBoot            string            `json:"secure_boot"`
	KexecLoaded           string            `json:"kexec_loaded"`
	GuestAgent            string            `json:"guest_agent"`
	SerialConsole         string            `json:"serial_console"`
	KerfState             string            `json:"kerf_state"`
	PoolConfigured        bool              `json:"pool_configured"`
	KimageState           string            `json:"kimage_state"`
	Instances             []string          `json:"instances"`
	InstanceState         []Instance        `json:"instance_state"`
	AssignedDevices       []string          `json:"assigned_devices"`
	StaleResources        []string          `json:"stale_resources"`
	PCIClasses            map[string]string `json:"pci_classes"`
	ProtectedDevices      []ProtectedDevice `json:"protected_devices"`
	ForbiddenPCIFunctions []string          `json:"forbidden_pci_functions"`
	Findings              []Finding         `json:"findings"`
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

const maxHostCommandOutput = 64 << 10

func runHostCommand(ctx context.Context, timeout time.Duration, binary string, arguments ...string) ([]byte, error) {
	return boundedexec.Run(ctx, timeout, binary, arguments,
		[]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}, maxHostCommandOutput)
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
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	r := Report{SchemaVersion: SchemaVersion, GeneratedAt: time.Now().UTC(), Architecture: runtime.GOARCH, KernelConfig: map[string]string{}, PCIClasses: map[string]string{}, SMTPolicy: "whole-core"}
	r.KernelRelease = read(o.Root, "/proc/sys/kernel/osrelease")
	r.OnlineCPUs = read(o.Root, "/sys/devices/system/cpu/online")
	r.OfflineCPUs = read(o.Root, "/sys/devices/system/cpu/offline")
	r.MemoryBytes = memBytes(read(o.Root, "/proc/meminfo"))
	r.Lockdown = valueOrUnknown(read(o.Root, "/sys/kernel/security/lockdown"))
	r.KexecLoaded = valueOrUnknown(read(o.Root, "/sys/kernel/kexec_loaded"))
	r.SecureBoot = secureBoot(o.Root)
	r.SerialConsole = serialConsole(o.Root)
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
	r.InstanceState, r.AssignedDevices = instanceState(o.Root, r.Instances)
	r.KimageState = valueOrUnknown(read(o.Root, "/proc/kimage"))
	r.PCIClasses = pciClasses(o.Root)
	r.ProtectedDevices, r.ForbiddenPCIFunctions = protectedDevices(o.Root)
	r.KernelConfig = kernelConfig(o.Root, r.KernelRelease)
	r.GuestAgent = guestAgent(ctx, o.Root, o.Timeout)
	if o.Root == "/" && o.Kerf != "" {
		out, err := runHostCommand(ctx, o.Timeout, o.Kerf, "--version")
		if err == nil {
			r.KerfVersion = strings.TrimSpace(string(out))
		}
		out, err = runHostCommand(ctx, o.Timeout, o.Kerf, "show")
		if err == nil {
			r.KerfState = strings.TrimSpace(string(out))
		}
		if len(o.ProbeCPUs) != 0 && o.ProbeMemory != "" {
			args := []string{"init", "--cpus=" + intList(o.ProbeCPUs), "--memory=" + o.ProbeMemory, "--devices=none", "--dry-run"}
			out, err = runHostCommand(ctx, o.Timeout, o.Kerf, args...)
			r.ContiguousProbe = strings.TrimSpace(string(out))
			if err == nil {
				r.ContiguousAllocation = "ready"
			} else {
				r.ContiguousAllocation = "unavailable"
			}
		}
	} else {
		r.KerfVersion = read(o.Root, "/run/kerf-version")
		r.KerfState = read(o.Root, "/run/kerf-show")
		r.ContiguousAllocation = read(o.Root, "/run/kerf-dry-run.status")
		r.ContiguousProbe = read(o.Root, "/run/kerf-dry-run.output")
	}
	if r.ContiguousAllocation == "" {
		r.ContiguousAllocation = "unprobed"
	}
	r.KerfState = valueOrUnknown(r.KerfState)
	r.PoolConfigured = r.KerfState != "unknown" && !strings.Contains(r.KerfState, "No memory pool configured")
	if r.PoolConfigured && len(r.Instances) == 0 {
		r.StaleResources = append(r.StaleResources, "configured Kerf pool has no matching instance")
	}
	for _, id := range kimageIDs(r.KimageState) {
		found := false
		for _, instance := range r.Instances {
			found = found || id == instance
		}
		if !found {
			r.StaleResources = append(r.StaleResources, "unmatched /proc/kimage entry "+id)
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
	if !strings.Contains(r.KerfVersion, "0.2.0") {
		r.add("KERF_VERSION", "error", "Kerf 0.2.0 is required and must be observable")
	}
	if r.KerfState == "unknown" {
		r.add("KERF_STATE_UNKNOWN", "error", "Kerf pool and instance state is unavailable")
	}
	if r.ContiguousAllocation != "ready" {
		r.add("CONTIGUOUS_ALLOCATION", "error", "requested Kerf pool must pass a read-only dry-run")
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
	if len(r.StaleResources) != 0 {
		r.add("STALE_RESOURCES", "error", strings.Join(r.StaleResources, "; "))
	}
	if len(r.ProtectedDevices) < 2 {
		r.add("PROTECTED_DEVICE_TOPOLOGY", "error", "boot-disk and primary-NIC PCI ancestry must both be resolved")
	}
	if r.GuestAgent != "active" {
		r.add("GUEST_AGENT", "error", "Google guest agent is not confirmed active")
	}
	if r.SecureBoot == "enabled" {
		r.add("SECURE_BOOT", "error", "unsigned child loading is incompatible with Secure Boot")
	}
	if r.SecureBoot == "unknown" {
		r.add("SECURE_BOOT_UNKNOWN", "error", "Secure Boot state is unknown")
	}
	if r.Lockdown == "unknown" || !strings.Contains(r.Lockdown, "[none]") {
		r.add("LOCKDOWN", "error", "kernel lockdown must be observably inactive")
	}
	if r.KexecLoaded == "unknown" {
		r.add("KEXEC_STATE_UNKNOWN", "error", "kexec readiness state is unknown")
	}
	if r.SerialConsole != "available" {
		r.add("SERIAL_RECOVERY", "error", "serial-console recovery is not confirmed available")
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

func intList(values []int) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.Itoa(value)
	}
	return strings.Join(parts, ",")
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
	if root != "/" {
		if status := read(root, "/run/secure-boot.status"); status != "" {
			return status
		}
	}
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

func serialConsole(root string) string {
	if root != "/" {
		return valueOrUnknown(read(root, "/run/serial-console.status"))
	}
	for _, device := range []string{"/dev/ttyS0", "/dev/ttyAMA0"} {
		if info, err := os.Stat(device); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return "available"
		}
	}
	return "unknown"
}

func instanceState(root string, names []string) ([]Instance, []string) {
	instances := make([]Instance, 0, len(names))
	var assigned []string
	for _, name := range names {
		base := path(root, "/sys/fs/multikernel/instances/"+name)
		instance := Instance{Name: name, Status: valueOrUnknown(read(root, "/sys/fs/multikernel/instances/"+name+"/status"))}
		for _, directory := range []string{"devices", "device"} {
			entries, _ := os.ReadDir(filepath.Join(base, directory))
			for _, entry := range entries {
				instance.Devices = append(instance.Devices, entry.Name())
				assigned = append(assigned, name+":"+entry.Name())
			}
		}
		sort.Strings(instance.Devices)
		instances = append(instances, instance)
	}
	sort.Strings(assigned)
	return instances, assigned
}

func kimageIDs(raw string) []string {
	var ids []string
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "MK_ID" || strings.HasPrefix(fields[0], "-") || strings.HasPrefix(fields[0], "=") {
			continue
		}
		ids = append(ids, fields[0])
	}
	return ids
}

func guestAgent(ctx context.Context, root string, timeout time.Duration) string {
	if root != "/" {
		return valueOrUnknown(read(root, "/run/google-guest-agent.status"))
	}
	b, e := runHostCommand(ctx, timeout, "/usr/bin/systemctl", "is-active", "google-guest-agent")
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

func sysfsPCIAncestry(link string) ([]string, string) {
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return nil, ""
	}
	var ancestry []string
	for _, component := range strings.Split(filepath.Clean(target), string(filepath.Separator)) {
		if pciBDF.MatchString(component) {
			ancestry = append(ancestry, strings.ToLower(component))
		}
	}
	if len(ancestry) == 0 {
		return nil, ""
	}
	return ancestry, ancestry[len(ancestry)-1]
}

func rootDevice(root string) string {
	if root != "/" {
		return read(root, "/run/root-device")
	}
	var stat syscall.Stat_t
	if err := syscall.Stat("/", &stat); err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev)))
}

func defaultInterface(root string) string {
	for _, line := range strings.Split(read(root, "/proc/net/route"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

func protectedDevices(root string) ([]ProtectedDevice, []string) {
	var devices []ProtectedDevice
	if device := rootDevice(root); device != "" {
		ancestry, function := sysfsPCIAncestry(path(root, "/sys/dev/block/"+device))
		if function != "" {
			devices = append(devices, ProtectedDevice{Kind: "boot-disk", Device: device, PCIFunction: function, Ancestry: ancestry})
		}
	}
	if device := defaultInterface(root); device != "" {
		ancestry, function := sysfsPCIAncestry(path(root, "/sys/class/net/"+device+"/device"))
		if function != "" {
			devices = append(devices, ProtectedDevice{Kind: "primary-nic", Device: device, PCIFunction: function, Ancestry: ancestry})
		}
	}
	set := map[string]bool{}
	for _, device := range devices {
		for _, function := range device.Ancestry {
			set[function] = true
		}
	}
	forbidden := make([]string, 0, len(set))
	for function := range set {
		forbidden = append(forbidden, function)
	}
	sort.Strings(forbidden)
	return devices, forbidden
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
	type core struct{ physical, id int }
	selected := map[int]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	cores := map[core][]int{}
	for _, cpu := range r.CPUs {
		if cpu.Online {
			key := core{cpu.Physical, cpu.Core}
			cores[key] = append(cores[key], cpu.APIC)
		}
	}
	for key, siblings := range cores {
		count := 0
		for _, sibling := range siblings {
			if selected[sibling] {
				count++
			}
		}
		if count != 0 && count != len(siblings) {
			return fmt.Errorf("allocation splits SMT siblings on physical package %d core %d", key.physical, key.id)
		}
	}
	return nil
}

func ValidateRequestedPCIFunctions(r Report, functions []string) error {
	forbidden := map[string]bool{}
	for _, function := range r.ForbiddenPCIFunctions {
		forbidden[strings.ToLower(function)] = true
	}
	for _, function := range functions {
		if !pciBDF.MatchString(function) {
			return fmt.Errorf("invalid PCI function %q", function)
		}
		if forbidden[strings.ToLower(function)] {
			return fmt.Errorf("PCI function %s is protected by boot-disk or primary-NIC ancestry", function)
		}
	}
	return nil
}

// IsReadOnlyOperation is a testable inventory of all host-check operations.
func IsReadOnlyOperation(name string) bool {
	return !bytes.Contains([]byte(name), []byte("write")) && name != "kerf init" && name != "kerf create"
}
