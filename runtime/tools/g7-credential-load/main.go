// g7-credential-load exercises the production Kerf credential handoff on an
// explicitly disposable lab host. It is not installed as a runtime service.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func main() {
	var binary, kernel, bundle, generation, storageConfig, storageGeneration string
	flag.StringVar(&binary, "kerf", "", "pinned Kerf executable")
	flag.StringVar(&kernel, "kernel", "", "pinned kernel")
	flag.StringVar(&bundle, "bundle", "", "private diagnostic load bundle")
	flag.StringVar(&generation, "generation", "", "diagnostic generation")
	flag.StringVar(&storageConfig, "storage-config", "", "private mediated-root StorageConfig JSON for disposable qualification")
	flag.StringVar(&storageGeneration, "storage-generation", "", "active fixture storage export generation")
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
	if storageConfig != "" {
		decoded, err := hex.DecodeString(storageGeneration)
		if err != nil || len(decoded) != 16 {
			fmt.Fprintln(os.Stderr, "invalid fixture storage generation")
			os.Exit(2)
		}
		fd, err := unix.Open(storageConfig, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fixture storage config unavailable")
			os.Exit(2)
		}
		file := os.NewFile(uintptr(fd), "fixture storage configuration")
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != 0 || st.Mode&0777 != 0600 || st.Nlink != 1 || st.Size <= 0 || st.Size > 65536 {
			file.Close()
			fmt.Fprintln(os.Stderr, "unsafe fixture storage config")
			os.Exit(2)
		}
		decoder := json.NewDecoder(io.LimitReader(file, 65537))
		decoder.DisallowUnknownFields()
		var config protocol.StorageConfig
		err = decoder.Decode(&config)
		var trailing any
		if err != nil || decoder.Decode(&trailing) != io.EOF {
			file.Close()
			fmt.Fprintln(os.Stderr, "invalid fixture storage config")
			os.Exit(2)
		}
		file.Close()
		sandbox.Config.Storage = &config
		sandbox.Storage = &protocol.StorageStatus{State: "ACTIVE", ExportGeneration: storageGeneration}
	} else if storageGeneration != "" {
		fmt.Fprintln(os.Stderr, "fixture storage config required")
		os.Exit(2)
	}
	cli := &kerf.CLI{Path: binary}
	err = cli.Load(context.Background(), sandbox, kernel, "", "rdinit=/init console=mktty0 panic=-1", kerf.LoadFiles{RuntimeDir: runtimeDir, Initramfs: initrd, Kernel: image})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PRIVATE_CREDENTIAL_LOAD_COMPLETED port=" + strconv.Itoa(port))
}
