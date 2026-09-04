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
	if err := secureRegular(path, r.RequiredUID); err != nil {
		return lifecycle.Artifacts{}, err
	}
	raw, err := os.ReadFile(path)
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
	return lifecycle.Artifacts{Kernel: manifest.Kernel.Path, Initrd: manifest.Initramfs.Path}, nil
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
	for _, executable := range []string{manifest.Kernel.Path, manifest.Agent.Path, manifest.Relay.Path} {
		binary, err := elf.Open(executable)
		if err != nil {
			return fmt.Errorf("artifact %s is not ELF: %w", executable, err)
		}
		machine := binary.FileHeader.Machine
		binary.Close()
		if machine != elf.EM_X86_64 {
			return fmt.Errorf("artifact %s has incompatible ELF machine %s", executable, machine)
		}
	}
	configPath := filepath.Join(filepath.Dir(manifest.Kernel.Path), "config-"+manifest.KernelRelease)
	config, err := os.ReadFile(configPath)
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

func secureRegular(path string, uid uint32) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("artifact path %q is not absolute", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uid || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("artifact %s has unsafe type, owner, or mode", path)
	}
	return nil
}

func verifyArtifact(artifact Artifact, uid uint32) error {
	if len(artifact.SHA256) != 64 {
		return errors.New("artifact digest is malformed")
	}
	if err := secureRegular(artifact.Path, uid); err != nil {
		return err
	}
	file, err := os.Open(artifact.Path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return fmt.Errorf("artifact %s digest mismatch", artifact.Path)
	}
	return nil
}
