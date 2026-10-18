package kernelmanifest

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

type kernelSection struct {
	name                         uint32
	kind                         uint32
	flags, address, offset, size uint64
	link                         uint32
	entrySize                    uint64
}

// verifyKernel binds the manifest release to a held, hash-verified ELF's
// linux_banner. Parse only bounded metadata; a general ELF reader can load
// unrelated string tables or scan arbitrarily long symbol names.
// This does not establish provenance or host/child wire ABI compatibility.
func verifyKernel(artifact Artifact, uid uint32, release string) (result error) {
	file, err := openVerifiedArtifact(artifact, uid)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	before, ok := stableIdentity(info)
	if err != nil || !ok {
		return errors.New("cannot identify verified kernel")
	}
	defer func() { result = errors.Join(result, verifyStableRegular(file, before)) }()
	read := func(offset, size, maximum uint64) ([]byte, error) {
		if size > maximum || offset > uint64(before.Size) || size > uint64(before.Size)-offset {
			return nil, errors.New("kernel ELF range is outside bounded file metadata")
		}
		data := make([]byte, int(size))
		if _, err := file.ReadAt(data, int64(offset)); err != nil {
			return nil, fmt.Errorf("read kernel ELF: %w", err)
		}
		return data, nil
	}
	header, err := read(0, 64, 64)
	if err != nil {
		return err
	}
	order := binary.LittleEndian
	if !bytes.Equal(header[:7], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1}) || order.Uint16(header[16:]) != uint16(elf.ET_EXEC) || order.Uint16(header[18:]) != uint16(elf.EM_X86_64) || order.Uint32(header[20:]) != 1 || order.Uint16(header[52:]) != 64 {
		return errors.New("kernel ELF architecture or type is incompatible")
	}
	count := order.Uint16(header[60:])
	// Canonical vmlinux uses ordinary numbering. Refuse extended numbering.
	if order.Uint16(header[58:]) != 64 || count == 0 || count >= uint16(elf.SHN_LORESERVE) {
		return errors.New("kernel section table is incompatible")
	}
	raw, err := read(order.Uint64(header[40:]), uint64(count)*64, 4<<20)
	if err != nil {
		return err
	}
	sections := make([]kernelSection, int(count))
	var table *kernelSection
	for i := range sections {
		b := raw[i*64 : (i+1)*64]
		sections[i] = kernelSection{name: order.Uint32(b), kind: order.Uint32(b[4:]), flags: order.Uint64(b[8:]), address: order.Uint64(b[16:]), offset: order.Uint64(b[24:]), size: order.Uint64(b[32:]), link: order.Uint32(b[40:]), entrySize: order.Uint64(b[56:])}
		if sections[i].kind == uint32(elf.SHT_SYMTAB) {
			if table != nil {
				return errors.New("kernel symbol table is ambiguous")
			}
			table = &sections[i]
		}
	}
	// Preserve general ELF header validation only after preflighting the
	// section-name table and program-header geometry used by debug/elf.
	nameIndex := order.Uint16(header[62:])
	if nameIndex == 0 || int(nameIndex) >= len(sections) {
		return errors.New("kernel section names are incompatible")
	}
	sectionNames := sections[nameIndex]
	if sectionNames.kind != uint32(elf.SHT_STRTAB) || sectionNames.flags&uint64(elf.SHF_COMPRESSED) != 0 || sectionNames.size == 0 {
		return errors.New("kernel section names are incompatible")
	}
	sectionNameData, err := read(sectionNames.offset, sectionNames.size, 32<<20)
	if err != nil {
		return err
	}
	for _, section := range sections {
		if uint64(section.name) >= uint64(len(sectionNameData)) || bytes.IndexByte(sectionNameData[section.name:min(len(sectionNameData), int(section.name)+4096)], 0) < 0 {
			return errors.New("kernel section name is out of bounds")
		}
	}
	programCount := order.Uint16(header[56:])
	if programCount == 0xffff || (programCount > 0 && order.Uint16(header[54:]) != 56) {
		return errors.New("kernel program table is incompatible")
	}
	if programCount > 0 {
		if _, err := read(order.Uint64(header[32:]), uint64(programCount)*56, 4<<20); err != nil {
			return err
		}
	}
	image, err := elf.NewFile(file)
	if err != nil {
		return fmt.Errorf("invalid kernel ELF: %w", err)
	}
	if err := image.Close(); err != nil {
		return err
	}
	if table == nil || table.flags&uint64(elf.SHF_COMPRESSED) != 0 || table.entrySize != 24 || table.size == 0 || table.size > 32<<20 || table.size%24 != 0 || int(table.link) >= len(sections) {
		return errors.New("kernel symbol table has unsafe type or size")
	}
	names := sections[table.link]
	if names.kind != uint32(elf.SHT_STRTAB) || names.flags&uint64(elf.SHF_COMPRESSED) != 0 || names.size == 0 || names.size > 32<<20 {
		return errors.New("kernel symbol names have unsafe type or size")
	}
	symbols, err := read(table.offset, table.size, 32<<20)
	if err != nil {
		return err
	}
	nameData, err := read(names.offset, names.size, 32<<20)
	if err != nil {
		return err
	}
	if !bytes.Equal(symbols[:24], make([]byte, 24)) {
		return errors.New("kernel reserved symbol is incompatible")
	}
	var banner *elf.Symbol
	for offset := 24; offset < len(symbols); offset += 24 {
		b := symbols[offset : offset+24]
		name := int(order.Uint32(b))
		if name >= len(nameData) {
			return errors.New("kernel symbol name is out of bounds")
		}
		end := bytes.IndexByte(nameData[name:min(len(nameData), name+4096)], 0)
		if end < 0 {
			return errors.New("kernel symbol name is unterminated or oversized")
		}
		if !bytes.Equal(nameData[name:name+end], []byte("linux_banner")) {
			continue
		}
		if banner != nil {
			return errors.New("kernel release symbol is ambiguous")
		}
		banner = &elf.Symbol{Info: b[4], Section: elf.SectionIndex(order.Uint16(b[6:])), Value: order.Uint64(b[8:]), Size: order.Uint64(b[16:])}
	}
	if banner == nil || banner.Info != byte(elf.STB_GLOBAL)<<4|byte(elf.STT_OBJECT) || banner.Size == 0 || banner.Size > 4096 || banner.Section == elf.SHN_UNDEF || banner.Section >= elf.SHN_LORESERVE || int(banner.Section) >= len(sections) {
		return errors.New("kernel release symbol is missing or incompatible")
	}
	section := sections[banner.Section]
	if section.kind != uint32(elf.SHT_PROGBITS) || section.flags&uint64(elf.SHF_ALLOC) == 0 || section.flags&uint64(elf.SHF_COMPRESSED) != 0 || banner.Value < section.address {
		return errors.New("kernel release section is incompatible")
	}
	offset := banner.Value - section.address
	if offset > section.size || banner.Size > section.size-offset || section.offset > uint64(before.Size) || section.size > uint64(before.Size)-section.offset {
		return errors.New("kernel release symbol is outside its file section")
	}
	data, err := read(section.offset+offset, banner.Size, 4096)
	if err != nil {
		return err
	}
	if data[len(data)-1] != 0 || bytes.IndexByte(data[:len(data)-1], 0) >= 0 || !strings.HasPrefix(string(data), "Linux version "+release+" ") {
		return errors.New("kernel embedded release is incompatible")
	}
	return nil
}
