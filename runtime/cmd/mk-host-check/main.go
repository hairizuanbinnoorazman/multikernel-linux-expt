package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostcheck"
)

func main() {
	o := hostcheck.DefaultOptions()
	flag.StringVar(&o.Root, "root", "/", "alternate read-only fixture root")
	flag.StringVar(&o.Kerf, "kerf", "kerf", "Kerf executable")
	flag.IntVar(&o.MinPrimaryCPUs, "min-primary-cpus", 4, "minimum CPUs retained by primary")
	mem := flag.Uint64("min-primary-memory-bytes", 8<<30, "minimum primary memory")
	flag.DurationVar(&o.Timeout, "timeout", 5*time.Second, "read-only command timeout")
	flag.Parse()
	o.MinPrimaryMemByte = *mem
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
