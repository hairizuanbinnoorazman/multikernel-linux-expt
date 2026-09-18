package kernelmanifest

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

const (
	multikernelRevision = "3bdd35b64413da0b4e089ce931bfc2e8b031cbf7"
	kerfRevision        = "8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec"
)

type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Compatibility struct {
	MultikernelRevision string `json:"multikernel_revision"`
	KerfVersion         string `json:"kerf_version"`
	KerfRevision        string `json:"kerf_revision"`
}
type Protocol struct {
	Min int `json:"min"`
	Max int `json:"max"`
}
type Transport struct {
	Module       Artifact `json:"module"`
	ModuleName   string   `json:"module_name"`
	SocketOption int      `json:"socket_option"`
	TransportID  int      `json:"transport_id"`
	PrimaryRole  string   `json:"primary_role"`
	ChildRole    string   `json:"child_role"`
}
type Manifest struct {
	SchemaVersion  int           `json:"schema_version"`
	Name           string        `json:"name"`
	Architecture   string        `json:"architecture"`
	KernelRelease  string        `json:"kernel_release"`
	Compatibility  Compatibility `json:"compatibility"`
	Kernel         Artifact      `json:"kernel"`
	Initramfs      Artifact      `json:"initramfs"`
	Agent          Artifact      `json:"agent"`
	Relay          Artifact      `json:"relay"`
	Transport      Transport     `json:"transport"`
	Protocol       Protocol      `json:"protocol"`
	RequiredConfig []string      `json:"required_config"`
	Modules        []Artifact    `json:"modules,omitempty"`
	OCIFeatures    []string      `json:"oci_features"`
}
type Resolver struct {
	Directory   string
	RequiredUID uint32
}

func (r Resolver) Resolve(name string) (lifecycle.Artifacts, error) {
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return lifecycle.Artifacts{}, errors.New("invalid approved manifest name")
	}
	path := filepath.Join(r.Directory, name+".json")
	if filepath.Dir(path) != filepath.Clean(r.Directory) {
		return lifecycle.Artifacts{}, errors.New("manifest path escaped approved directory")
	}
	raw, err := readStableRegular(path, r.RequiredUID, 1<<20)
	if err != nil {
		return lifecycle.Artifacts{}, err
	}
	var manifest Manifest
	if err = protocol.StrictDecode(raw, &manifest); err != nil {
		return lifecycle.Artifacts{}, err
	}
	if err = validate(manifest, name, r.RequiredUID); err != nil {
		return lifecycle.Artifacts{}, err
	}
	kernel, err := openVerifiedArtifact(manifest.Kernel, r.RequiredUID)
	if err != nil {
		return lifecycle.Artifacts{}, err
	}
	initrd, err := openVerifiedArtifact(manifest.Initramfs, r.RequiredUID)
	if err != nil {
		_ = kernel.Close()
		return lifecycle.Artifacts{}, err
	}
	return lifecycle.Artifacts{Kernel: manifest.Kernel.Path, Initrd: manifest.Initramfs.Path,
		KernelFile: kernel, InitrdFile: initrd}, nil
}

func validate(manifest Manifest, name string, uid uint32) error {
	if manifest.SchemaVersion != 1 || manifest.Name != name || manifest.Architecture != runtime.GOARCH || runtime.GOARCH != "amd64" {
		return errors.New("manifest identity or architecture is incompatible")
	}
	if manifest.KernelRelease == "" || manifest.Compatibility.MultikernelRevision != multikernelRevision ||
		manifest.Compatibility.KerfVersion != "v0.2.0" || manifest.Compatibility.KerfRevision != kerfRevision {
		return errors.New("manifest release or compatibility pin is incompatible")
	}
	if manifest.Protocol.Min > 1 || manifest.Protocol.Max < 1 {
		return errors.New("manifest does not support daemon/agent protocol v1")
	}
	if manifest.Transport.ModuleName != "mk_transport" || manifest.Transport.SocketOption != 9 ||
		manifest.Transport.TransportID != 1 || manifest.Transport.PrimaryRole != "server" || manifest.Transport.ChildRole != "client" {
		return errors.New("manifest transport module, socket option, ID, or direction is incompatible")
	}
	artifacts := append([]Artifact{manifest.Kernel, manifest.Initramfs, manifest.Agent, manifest.Relay, manifest.Transport.Module}, manifest.Modules...)
	for _, artifact := range artifacts {
		if err := verifyArtifact(artifact, uid); err != nil {
			return err
		}
	}
	for _, executable := range []Artifact{manifest.Kernel, manifest.Agent, manifest.Relay} {
		file, err := openVerifiedArtifact(executable, uid)
		if err != nil {
			return err
		}
		binary, err := elf.NewFile(file)
		if err != nil {
			_ = file.Close()
			return fmt.Errorf("artifact %s is not ELF: %w", executable.Path, err)
		}
		machine := binary.FileHeader.Machine
		closeErr := errors.Join(binary.Close(), file.Close())
		if closeErr != nil {
			return closeErr
		}
		if machine != elf.EM_X86_64 {
			return fmt.Errorf("artifact %s has incompatible ELF machine %s", executable.Path, machine)
		}
	}
	configPath := filepath.Join(filepath.Dir(manifest.Kernel.Path), "config-"+manifest.KernelRelease)
	config, err := readStableRegular(configPath, uid, 16<<20)
	if err != nil {
		return fmt.Errorf("read kernel config: %w", err)
	}
	lines := map[string]bool{}
	for _, line := range strings.Split(string(config), "\n") {
		lines[strings.TrimSpace(line)] = true
	}
	for _, required := range manifest.RequiredConfig {
		if !lines[required] {
			return fmt.Errorf("kernel config is missing %s", required)
		}
	}
	if len(manifest.OCIFeatures) == 0 {
		return errors.New("manifest OCI feature set is empty")
	}
	return nil
}

func stableIdentity(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	return identity, ok
}

func sameIdentity(before, after *syscall.Stat_t) bool {
	return before.Dev == after.Dev && before.Ino == after.Ino && before.Size == after.Size &&
		before.Mtim == after.Mtim && before.Ctim == after.Ctim
}

func openStableRegular(path string, uid uint32, maximum int64) (*os.File, *syscall.Stat_t, error) {
	if !filepath.IsAbs(path) {
		return nil, nil, fmt.Errorf("artifact path %q is not absolute", path)
	}
	beforeInfo, err := os.Lstat(path)
	before, ok := stableIdentity(beforeInfo)
	if err != nil || !ok || before.Uid != uid || !beforeInfo.Mode().IsRegular() || beforeInfo.Mode().Perm()&0022 != 0 ||
		beforeInfo.Size() <= 0 || beforeInfo.Size() > maximum {
		return nil, nil, fmt.Errorf("artifact %s has unsafe type, owner, mode, or size", path)
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	afterInfo, err := file.Stat()
	after, afterOK := stableIdentity(afterInfo)
	if err != nil || !afterOK || !os.SameFile(beforeInfo, afterInfo) || !sameIdentity(before, after) {
		_ = file.Close()
		return nil, nil, errors.New("approved artifact identity changed while opening")
	}
	return file, before, nil
}

func verifyStableRegular(file *os.File, before *syscall.Stat_t) error {
	afterInfo, err := file.Stat()
	after, ok := stableIdentity(afterInfo)
	if err != nil || !ok || !sameIdentity(before, after) {
		return errors.New("approved artifact changed while reading")
	}
	return nil
}

func readStableRegular(path string, uid uint32, maximum int64) ([]byte, error) {
	file, before, err := openStableRegular(path, uid, maximum)
	if err != nil {
		return nil, err
	}
	value, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	stableErr := verifyStableRegular(file, before)
	closeErr := file.Close()
	if err = errors.Join(readErr, stableErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(value)) > maximum {
		return nil, errors.New("approved artifact exceeds size limit")
	}
	return value, nil
}

func openVerifiedArtifact(artifact Artifact, uid uint32) (*os.File, error) {
	if len(artifact.SHA256) != 64 {
		return nil, errors.New("artifact digest is malformed")
	}
	file, before, err := openStableRegular(artifact.Path, uid, 16<<30)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	stableErr := verifyStableRegular(file, before)
	_, seekErr := file.Seek(0, io.SeekStart)
	if err = errors.Join(copyErr, stableErr, seekErr); err != nil {
		_ = file.Close()
		return nil, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		_ = file.Close()
		return nil, fmt.Errorf("artifact %s digest mismatch", artifact.Path)
	}
	return file, nil
}

func verifyArtifact(artifact Artifact, uid uint32) error {
	file, err := openVerifiedArtifact(artifact, uid)
	if err != nil {
		return err
	}
	return file.Close()
}
