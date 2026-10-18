package kernelmanifest

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"strings"
)

// verifyModule reads metadata from a held, hash-verified ELF module. A trusted
// digest identifies bytes; it does not establish that those bytes match the
// kernel selected by a separately assembled manifest.
func verifyModule(artifact Artifact, uid uint32, release, expectedName string) error {
	_, err := verifiedModuleName(artifact, uid, release, expectedName)
	return err
}

func verifiedModuleName(artifact Artifact, uid uint32, release, expectedName string) (moduleName string, result error) {
	file, err := openVerifiedArtifact(artifact, uid)
	if err != nil {
		return "", err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	beforeInfo, err := file.Stat()
	before, ok := stableIdentity(beforeInfo)
	if err != nil || !ok {
		return "", errors.New("cannot identify verified module")
	}
	defer func() { result = errors.Join(result, verifyStableRegular(file, before)) }()
	image, err := elf.NewFile(file)
	if err != nil {
		return "", fmt.Errorf("module %s is not ELF: %w", artifact.Path, err)
	}
	defer func() { result = errors.Join(result, image.Close()) }()
	if image.Class != elf.ELFCLASS64 || image.Data != elf.ELFDATA2LSB || image.Machine != elf.EM_X86_64 || image.Type != elf.ET_REL {
		return "", errors.New("module ELF architecture or type is incompatible")
	}
	var info *elf.Section
	for _, section := range image.Sections {
		if section.Name != ".modinfo" {
			continue
		}
		if info != nil {
			return "", errors.New("module has duplicate metadata sections")
		}
		info = section
	}
	if info == nil || info.Type != elf.SHT_PROGBITS || info.Flags&elf.SHF_COMPRESSED != 0 || info.Size == 0 || info.Size > 1<<20 {
		return "", errors.New("module metadata has unsafe type or size")
	}
	data, err := info.Data()
	if err != nil {
		return "", fmt.Errorf("read module metadata: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != 0 {
		return "", errors.New("module metadata is unterminated")
	}
	values := make(map[string]string)
	for _, field := range bytes.Split(data, []byte{0}) {
		key, value, found := strings.Cut(string(field), "=")
		if key != "name" && key != "vermagic" {
			continue
		}
		if !found || value == "" {
			return "", errors.New("module identity metadata is empty")
		}
		if _, exists := values[key]; exists {
			return "", errors.New("module identity metadata is ambiguous")
		}
		values[key] = value
	}
	name, magic := values["name"], values["vermagic"]
	if name == "" || strings.ContainsAny(name, "/\\ \t\r\n") || (expectedName != "" && name != expectedName) {
		return "", errors.New("module name is incompatible")
	}
	if !strings.HasPrefix(magic, release+" ") {
		return "", errors.New("module kernel release is incompatible")
	}
	return name, nil
}
