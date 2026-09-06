package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostcheck"
)

func cpuList(value string) ([]int, error) {
	if value == "" {
		return nil, nil
	}
	var values []int
	for _, part := range strings.Split(value, ",") {
		parsed, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
	}
	return values, nil
}

func main() {
	if buildinfo.PrintRequested(os.Stdout, "mk-host-check", os.Args[1:]) {
		return
	}
	o := hostcheck.DefaultOptions()
	flag.StringVar(&o.Root, "root", "/", "alternate read-only fixture root")
	flag.StringVar(&o.Kerf, "kerf", "kerf", "Kerf executable")
	flag.IntVar(&o.MinPrimaryCPUs, "min-primary-cpus", 4, "minimum CPUs retained by primary")
	mem := flag.Uint64("min-primary-memory-bytes", 8<<30, "minimum primary memory")
	probeCPUs := flag.String("probe-pool-cpus", "", "comma-separated APIC IDs for a read-only Kerf pool dry-run")
	flag.StringVar(&o.ProbeMemory, "probe-pool-memory", "", "memory quantity for a read-only Kerf pool dry-run")
	flag.DurationVar(&o.Timeout, "timeout", 5*time.Second, "read-only command timeout")
	flag.Parse()
	o.MinPrimaryMemByte = *mem
	var e error
	o.ProbeCPUs, e = cpuList(*probeCPUs)
	if e != nil || (len(o.ProbeCPUs) == 0) != (o.ProbeMemory == "") {
		fmt.Fprintln(os.Stderr, "both valid --probe-pool-cpus and --probe-pool-memory are required")
		os.Exit(2)
	}
	r := hostcheck.Check(context.Background(), o)
	b, e := hostcheck.Encode(r)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	os.Stdout.Write(b)
	if !r.Qualified {
		os.Exit(1)
	}
}
