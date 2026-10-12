// g7-credential-load exercises the production Kerf credential handoff on an
// explicitly disposable lab host. It is not installed as a runtime service.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func main() {
	var binary, kernel, bundle, generation string
	flag.StringVar(&binary, "kerf", "", "pinned Kerf executable")
	flag.StringVar(&kernel, "kernel", "", "pinned kernel")
	flag.StringVar(&bundle, "bundle", "", "private diagnostic load bundle")
	flag.StringVar(&generation, "generation", "", "diagnostic generation")
	flag.Parse()
	if os.Getenv("MK_SECURITY_DISPOSABLE") != "yes" || os.Geteuid() != 0 || binary == "" || kernel == "" || bundle == "" || len(generation) != 32 {
		fmt.Fprintln(os.Stderr, "explicit disposable root fixture and pinned inputs required")
		os.Exit(2)
	}
	runtimeDir, err := os.Open(filepath.Join(bundle, ".multikernel"))
	if err != nil {
		panic(err)
	}
	defer runtimeDir.Close()
	initrd, err := os.Open(filepath.Join(bundle, ".multikernel/initramfs.cpio.gz"))
	if err != nil {
		panic(err)
	}
	defer initrd.Close()
	image, err := os.Open(kernel)
	if err != nil {
		panic(err)
	}
	defer image.Close()
	const port = 7717
	sandbox := protocol.Sandbox{ID: "runtime-g7-authority", Generation: generation,
		Config: protocol.SandboxConfig{Bundle: bundle, AgentPort: port}}
	cli := &kerf.CLI{Path: binary}
	err = cli.Load(context.Background(), sandbox, kernel, "", "rdinit=/init console=mktty0 panic=-1", kerf.LoadFiles{RuntimeDir: runtimeDir, Initramfs: initrd, Kernel: image})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PRIVATE_CREDENTIAL_LOAD_COMPLETED port=" + strconv.Itoa(port))
}
