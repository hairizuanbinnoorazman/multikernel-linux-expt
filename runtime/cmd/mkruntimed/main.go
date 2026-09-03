package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
)

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
	var socket, dir, kpath, sysfs, pool, poolmem, poolmemreserve, kernel, initrd, cmdline string
	var timeout time.Duration
	flag.StringVar(&socket, "socket", "/run/mkruntimed.sock", "Unix API socket")
	flag.StringVar(&dir, "state-dir", "/var/lib/mkruntime", "durable state directory")
	flag.StringVar(&kpath, "kerf", "kerf", "Kerf executable")
	flag.StringVar(&sysfs, "multikernel-root", "/sys/fs/multikernel", "Multikernel sysfs root")
	flag.StringVar(&pool, "pool-cpus", "", "comma-separated pool APIC IDs")
	flag.StringVar(&poolmem, "pool-memory", "16GB", "Kerf pool memory")
	flag.StringVar(&poolmemreserve, "pool-memory-reserve", "1GB", "memory retained as Kerf allocator slack")
	flag.StringVar(&kernel, "kernel", "", "approved vmlinux path")
	flag.StringVar(&initrd, "initrd", "", "approved initramfs path")
	flag.StringVar(&cmdline, "cmdline", "rdinit=/mk-agent console=mktty0 panic=-1", "child command line")
	flag.DurationVar(&timeout, "backend-timeout", 30*time.Second, "Kerf timeout")
	flag.Parse()
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
	svc := lifecycle.New(st, b, lifecycle.Artifacts{Kernel: kernel, Initrd: initrd, Cmdline: cmdline})
	svc.SetPoolCPUs(ids)
	svc.SetPoolMemory(poolBytes, reserveBytes)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e = svc.Reconcile(ctx); e != nil {
		fmt.Fprintln(os.Stderr, "reconcile:", e)
		os.Exit(1)
	}
	srv := &daemon.Server{Service: svc}
	if e = srv.Listen(ctx, socket); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
