package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostcheck"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostconfig"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kernelmanifest"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func validatePoolReport(report hostcheck.Report, ids, forbidden []int, minPrimaryCPUs int, poolMemory, minPrimaryMemory uint64) error {
	if !report.Qualified {
		return fmt.Errorf("host qualification failed: %+v", report.Findings)
	}
	if err := hostcheck.ValidateRequestedAPICs(report, ids, minPrimaryCPUs); err != nil {
		return err
	}
	blocked := map[int]bool{}
	for _, id := range forbidden {
		blocked[id] = true
	}
	for _, id := range ids {
		if blocked[id] {
			return fmt.Errorf("APIC ID %d is forbidden by host policy", id)
		}
	}
	if poolMemory > report.MemoryBytes || report.MemoryBytes-poolMemory < minPrimaryMemory {
		return errors.New("pool memory violates primary memory headroom")
	}
	return nil
}

func hasLiveSandboxes(snapshot state.Snapshot) bool {
	for _, sandbox := range snapshot.Sandboxes {
		if sandbox.State != "ABSENT" {
			return true
		}
	}
	return false
}

func recoverableIdlePool(report hostcheck.Report) bool {
	return report.PoolConfigured && len(report.Instances) == 0 &&
		len(report.StaleResources) == 1 &&
		report.StaleResources[0] == "configured Kerf pool has no matching instance"
}

func cpus(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	var r []int
	for _, p := range strings.Split(s, ",") {
		n, e := strconv.Atoi(p)
		if e != nil {
			return nil, e
		}
		r = append(r, n)
	}
	return r, nil
}
func memoryBytes(value string) (uint64, error) {
	upper := strings.ToUpper(strings.TrimSpace(value))
	multipliers := []struct {
		suffix string
		value  uint64
	}{
		{"TIB", 1 << 40}, {"GIB", 1 << 30}, {"MIB", 1 << 20}, {"KIB", 1 << 10},
		{"TB", 1_000_000_000_000}, {"GB", 1_000_000_000}, {"MB", 1_000_000}, {"KB", 1_000}, {"B", 1},
	}
	for _, multiplier := range multipliers {
		if !strings.HasSuffix(upper, multiplier.suffix) {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(strings.TrimSuffix(upper, multiplier.suffix)), 10, 64)
		if err != nil || n > ^uint64(0)/multiplier.value {
			return 0, fmt.Errorf("invalid memory quantity %q", value)
		}
		return n * multiplier.value, nil
	}
	n, err := strconv.ParseUint(upper, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory quantity %q", value)
	}
	return n, nil
}
func main() {
	if buildinfo.PrintRequested(os.Stdout, "mkruntimed", os.Args[1:]) {
		return
	}
	var configPath, pool, poolmem, poolmemreserve, cmdline string
	var storageState, storageRuntime, storageServer, e2fsck string
	var rootfsState, rootfsStorage, rootfsBuilder string
	inspectNetwork := flag.Bool("doctor-network", false, "print bounded lifecycle/network ownership and kernel presence diagnostics and exit")
	networkSocket := flag.String("network-socket", "/run/mknetd.sock", "protected mknetd socket for network inspection")
	inspectStorage := flag.Bool("inspect-storage", false, "print bounded read-only durable storage ownership JSON and exit")
	inspectRuntime := flag.Bool("doctor", false, "print bounded read-only resource ownership/kernel presence diagnostics and exit")
	inspectOwnership := flag.Bool("inspect-ownership", false, "print bounded read-only durable ownership JSON and exit")
	flag.StringVar(&configPath, "config", "/etc/mkruntime/config.json", "strict root-owned host configuration")
	flag.StringVar(&pool, "pool-cpus", "", "comma-separated pool APIC IDs")
	flag.StringVar(&poolmem, "pool-memory", "16GB", "Kerf pool memory")
	flag.StringVar(&poolmemreserve, "pool-memory-reserve", "1GB", "memory retained as Kerf allocator slack")
	flag.StringVar(&cmdline, "cmdline", "rdinit=/init console=mktty0 panic=-1", "child command line")
	flag.StringVar(&storageState, "storage-state-dir", "/var/lib/mkruntimed/storage", "durable storage export state")
	flag.StringVar(&storageRuntime, "storage-runtime-dir", "/run/mkstorage", "storage server lease and log directory")
	flag.StringVar(&storageServer, "storage-server", "/usr/local/libexec/multikernel/mkvsock-nbd", "primary mediated NBD server")
	flag.StringVar(&e2fsck, "e2fsck", "/usr/sbin/e2fsck", "offline ext4 checker")
	flag.StringVar(&rootfsState, "rootfs-state-dir", "/var/lib/mkruntimed/rootfs", "durable rootfs preparation state")
	flag.StringVar(&rootfsStorage, "rootfs-storage-dir", "/srv/multikernel-storage/runtime", "prepared mediated root images")
	flag.StringVar(&rootfsBuilder, "rootfs-builder", "/usr/local/libexec/multikernel/build-runtime-container-initramfs.sh", "privileged rootfs builder")
	flag.Parse()
	modes := 0
	for _, selected := range []bool{*inspectRuntime, *inspectOwnership, *inspectStorage, *inspectNetwork} {
		if selected {
			modes++
		}
	}
	if modes > 1 {
		fmt.Fprintln(os.Stderr, "select one inspection mode")
		os.Exit(2)
	}
	if *inspectNetwork && !filepath.IsAbs(*networkSocket) {
		fmt.Fprintln(os.Stderr, "network inspection requires an absolute network socket")
		os.Exit(2)
	}
	hostConfig, e := hostconfig.Load(configPath)
	if e != nil {
		fmt.Fprintln(os.Stderr, "host configuration:", e)
		os.Exit(2)
	}
	socket, dir, kpath, sysfs := hostConfig.SocketPath, hostConfig.StateDirectory, hostConfig.KerfExecutable, hostConfig.MultikernelSysfsRoot
	timeout := hostConfig.BackendTimeout()
	if *inspectNetwork {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		report := doctor.InspectNetwork(ctx, doctor.NetworkInputs{
			Expectations: func(ctx context.Context) (protocol.NetworkExpectationsReport, error) {
				return daemon.InspectNetworkExpectations(ctx, daemon.Client{Path: socket, Timeout: timeout})
			},
			Network: func(ctx context.Context) (mknetwork.OwnershipReport, error) {
				return mknetwork.InspectOwnership(ctx, mknetwork.Client{Path: *networkSocket, Timeout: timeout})
			},
			Kernel: func(ctx context.Context) ([]string, error) { return doctor.KernelInventory(ctx, sysfs) }, Boot: doctor.BootIdentity,
		}, buildinfo.Revision)
		if json.NewEncoder(os.Stdout).Encode(report) != nil {
			os.Exit(1)
		}
		if report.Status != "consistent" {
			os.Exit(1)
		}
		return
	}
	if *inspectStorage {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		report, err := daemon.InspectStorageOwnership(ctx, daemon.Client{Path: socket, Timeout: timeout})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if json.NewEncoder(os.Stdout).Encode(report) != nil {
			os.Exit(1)
		}
		return
	}
	if *inspectRuntime {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		report := doctor.InspectNativeResources(ctx, doctor.NativeInputs{
			Native: func(ctx context.Context) (protocol.NativeResourceReport, error) {
				return daemon.InspectNativeResources(ctx, daemon.Client{Path: socket, Timeout: timeout})
			},
			Kernel: func(ctx context.Context) ([]string, error) { return doctor.KernelInventory(ctx, sysfs) }, Boot: doctor.BootIdentity,
		}, buildinfo.Revision)
		if json.NewEncoder(os.Stdout).Encode(report) != nil {
			os.Exit(1)
		}
		if report.Status != "consistent" {
			os.Exit(1)
		}
		return
	}
	if *inspectOwnership {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		report, err := daemon.InspectOwnership(ctx, daemon.Client{Path: socket, Timeout: timeout})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err = json.NewEncoder(os.Stdout).Encode(report); err != nil {
			os.Exit(1)
		}
		return
	}
	ids, e := cpus(pool)
	if e != nil || len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "--pool-cpus is required and must be comma-separated integers")
		os.Exit(2)
	}
	for _, n := range ids {
		if n == 0 {
			fmt.Fprintln(os.Stderr, "APIC ID 0 is forbidden")
			os.Exit(2)
		}
	}
	poolBytes, e := memoryBytes(poolmem)
	if e != nil {
		fmt.Fprintln(os.Stderr, "--pool-memory:", e)
		os.Exit(2)
	}
	reserveBytes, e := memoryBytes(poolmemreserve)
	if e != nil || reserveBytes >= poolBytes {
		fmt.Fprintln(os.Stderr, "--pool-memory-reserve must be a valid quantity smaller than --pool-memory")
		os.Exit(2)
	}
	st, e := state.Open(dir)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer st.Close()
	b := &kerf.CLI{Path: kpath, Sysfs: sysfs, PoolCPUs: ids, PoolMemory: poolmem, Timeout: timeout}
	if !hasLiveSandboxes(st.Snapshot()) {
		hostOptions := hostcheck.DefaultOptions()
		hostOptions.Kerf = kpath
		hostOptions.MinPrimaryCPUs = 1
		hostOptions.MinPrimaryMemByte = 1
		hostOptions.Timeout = timeout
		hostOptions.ProbeCPUs = ids
		hostOptions.ProbeMemory = poolmem
		report := hostcheck.Check(context.Background(), hostOptions)
		// An ungraceful daemon exit may leave only its initialized, idle pool.
		// Durable state plus an empty backend inventory bounds automatic recovery;
		// any other stale resource remains an operator-visible hard failure.
		if recoverableIdlePool(report) {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), timeout)
			e = b.ReleasePool(cleanupCtx)
			cleanupCancel()
			if e != nil {
				fmt.Fprintln(os.Stderr, "recover idle Kerf pool:", e)
				os.Exit(1)
			}
			report = hostcheck.Check(context.Background(), hostOptions)
		}
		if e = validatePoolReport(report, ids, hostConfig.ForbiddenAPICIDs, hostConfig.MinPrimaryCPUs, poolBytes, hostConfig.MinPrimaryMemoryBytes); e != nil {
			encoded, _ := hostcheck.Encode(report)
			_, _ = os.Stderr.Write(encoded)
			fmt.Fprintln(os.Stderr, "host allocation policy:", e)
			os.Exit(1)
		}
	}
	svc := lifecycle.New(st, b, lifecycle.Artifacts{Cmdline: cmdline})
	storageStore, e := storage.OpenStore(storageState)
	if e != nil {
		fmt.Fprintln(os.Stderr, "storage state:", e)
		os.Exit(1)
	}
	storageBackend := &storage.LinuxBackend{Binary: storageServer, CheckBinary: e2fsck, RuntimeDir: storageRuntime, RequiredUID: 0, ReadyTimeout: timeout, StopTimeout: timeout}
	svc.SetStorage(storage.NewService(storageStore, storageBackend))
	rootfsStore, e := rootfspkg.OpenStore(rootfsState)
	if e != nil {
		fmt.Fprintln(os.Stderr, "rootfs state:", e)
		os.Exit(1)
	}
	rootfsService, e := rootfspkg.NewService(rootfsStore, &rootfspkg.LinuxBackend{Builder: rootfsBuilder}, rootfsStorage)
	if e != nil {
		fmt.Fprintln(os.Stderr, "rootfs service:", e)
		os.Exit(1)
	}
	svc.SetPreparedBootResolver(rootfsService)
	svc.SetArtifactResolver(kernelmanifest.Resolver{Directory: hostConfig.KernelManifestDirectory, RequiredUID: 0})
	svc.SetPoolCPUs(ids)
	svc.SetPoolMemory(poolBytes, reserveBytes)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e = svc.Reconcile(ctx); e != nil {
		fmt.Fprintln(os.Stderr, "reconcile:", e)
		os.Exit(1)
	}
	storageOwners := map[string]string{}
	for _, sandbox := range svc.List() {
		if sandbox.Config.Storage != nil {
			storageOwners[sandbox.Config.Storage.Path] = sandbox.Config.Storage.SHA256
		}
	}
	if e = rootfsService.Reconcile(ctx, storageOwners); e != nil {
		fmt.Fprintln(os.Stderr, "rootfs reconcile:", e)
		os.Exit(1)
	}
	srv := &daemon.Server{Service: svc, Rootfs: rootfsService, MaxFrame: hostConfig.MaxFrameSizeBytes}
	listenErr := srv.Listen(ctx, socket)
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), timeout)
	poolErr := svc.ReleaseIdlePool(cleanupCtx)
	cleanupCancel()
	if listenErr != nil {
		fmt.Fprintln(os.Stderr, listenErr)
		os.Exit(1)
	}
	if poolErr != nil {
		fmt.Fprintln(os.Stderr, "release idle Kerf pool:", poolErr)
		os.Exit(1)
	}
}
