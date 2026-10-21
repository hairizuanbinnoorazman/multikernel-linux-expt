package kernelmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// DockerGuest is operator approval of a dedicated root filesystem and fixed
// guest bootstrap. It is never derived from workload annotations or hooks.
// Approval does not imply that a particular builder implements activation.
type DockerGuest struct {
	Kind                 string              `json:"kind"`
	RootfsManifestSHA256 string              `json:"rootfs_manifest_sha256"`
	Bootstrap            Artifact            `json:"bootstrap"`
	Modules              map[string]Artifact `json:"modules"`
}

func (m *Manifest) UnmarshalJSON(data []byte) error {
	type manifestFields Manifest
	var decoded manifestFields
	if err := protocol.StrictDecode(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	// encoding/json accepts case-insensitive struct field aliases. A second
	// DOCKER_GUEST:null must not silently overwrite an explicit approval.
	for name := range fields {
		switch name {
		case "schema_version", "name", "architecture", "kernel_release", "compatibility", "kernel", "initramfs", "agent", "relay", "transport", "protocol", "required_config", "modules", "oci_features", "docker_guest":
		default:
			return errors.New("noncanonical manifest field")
		}
	}
	if value, present := fields["docker_guest"]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errors.New("Docker guest approval must be an object, not null")
	}
	*m = Manifest(decoded)
	return nil
}

func exactDockerObject(data []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := protocol.StrictDecode(data, &fields); err != nil || fields == nil || len(fields) != len(names) {
		return nil, errors.New("invalid Docker approval object")
	}
	for _, name := range names {
		if _, present := fields[name]; !present {
			return nil, errors.New("noncanonical Docker approval field")
		}
	}
	return fields, nil
}

func (p *DockerGuest) UnmarshalJSON(data []byte) error {
	fields, err := exactDockerObject(data, "kind", "rootfs_manifest_sha256", "bootstrap", "modules")
	if err != nil {
		return err
	}
	if _, err = exactDockerObject(fields["bootstrap"], "path", "sha256"); err != nil {
		return err
	}
	modules, err := exactDockerObject(fields["modules"], "llc", "stp", "bridge", "veth")
	if err != nil {
		return err
	}
	for _, artifact := range modules {
		if _, err = exactDockerObject(artifact, "path", "sha256"); err != nil {
			return err
		}
	}
	type approvalFields DockerGuest
	var decoded approvalFields
	if err = protocol.StrictDecode(data, &decoded); err != nil {
		return err
	}
	*p = DockerGuest(decoded)
	return nil
}

func validateDockerGuest(profile *DockerGuest, release string, uid uint32) error {
	if profile == nil {
		return nil
	}
	digest, err := hex.DecodeString(profile.RootfsManifestSHA256)
	if profile.Kind != "docker-rootless-offline-v1" || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != profile.RootfsManifestSHA256 {
		return errors.New("Docker guest profile kind or root filesystem approval is invalid")
	}
	if err := verifyArtifact(profile.Bootstrap, uid); err != nil {
		return errors.New("Docker guest bootstrap artifact is not approved")
	}
	script, err := readStableRegular(profile.Bootstrap.Path, uid, 4<<20)
	actual := sha256.Sum256(script)
	if err != nil || hex.EncodeToString(actual[:]) != profile.Bootstrap.SHA256 || !bytes.HasPrefix(script, []byte("#!/bin/busybox sh\n")) {
		return errors.New("Docker guest bootstrap must use the fixed guest BusyBox interpreter")
	}
	if len(profile.Modules) != 4 {
		return errors.New("Docker guest requires exactly llc, stp, bridge and veth modules")
	}
	for _, name := range []string{"llc", "stp", "bridge", "veth"} {
		artifact, exists := profile.Modules[name]
		if !exists || verifyModule(artifact, uid, release, name) != nil {
			return errors.New("Docker guest module differs from its approved name or child release")
		}
	}
	return nil
}
