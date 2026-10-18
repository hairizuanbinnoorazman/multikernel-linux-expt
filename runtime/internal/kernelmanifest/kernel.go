package kernelmanifest

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"strings"
)

// verifyKernel binds the selected release to the ELF's actual linux_banner,
// rather than trusting the manifest's filename, config or approved byte hash.
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
	image, err := elf.NewFile(file)
	if err != nil {
		return fmt.Errorf("kernel is not ELF: %w", err)
	}
	defer func() { result = errors.Join(result, image.Close()) }()
	if image.Class != elf.ELFCLASS64 || image.Data != elf.ELFDATA2LSB || image.Machine != elf.EM_X86_64 || image.Type != elf.ET_EXEC {
		return errors.New("kernel ELF architecture or type is incompatible")
	}
	var table *elf.Section
	for _, section := range image.Sections {
		if section.Type != elf.SHT_SYMTAB {
			continue
		}
		if table != nil {
			return errors.New("kernel symbol table is ambiguous")
		}
		table = section
	}
	// Bound debug/elf's table/string allocations before asking it for symbols.
	if table == nil || table.Flags&elf.SHF_COMPRESSED != 0 || table.Entsize != 24 || table.Size == 0 || table.Size > 32<<20 || table.Size%24 != 0 || int(table.Link) >= len(image.Sections) {
		return errors.New("kernel symbol table has unsafe type or size")
	}
	stringsTable := image.Sections[table.Link]
	if stringsTable.Type != elf.SHT_STRTAB || stringsTable.Flags&elf.SHF_COMPRESSED != 0 || stringsTable.Size == 0 || stringsTable.Size > 32<<20 {
		return errors.New("kernel symbol names have unsafe type or size")
	}
	symbols, err := image.Symbols()
	if err != nil {
		return fmt.Errorf("read kernel symbols: %w", err)
	}
	var banner *elf.Symbol
	for i := range symbols {
		if symbols[i].Name == "linux_banner" {
			if banner != nil {
				return errors.New("kernel release symbol is ambiguous")
			}
			banner = &symbols[i]
		}
	}
	if banner == nil || elf.ST_TYPE(banner.Info) != elf.STT_OBJECT || elf.ST_BIND(banner.Info) != elf.STB_GLOBAL || banner.Size == 0 || banner.Size > 4096 || banner.Section == elf.SHN_UNDEF || banner.Section >= elf.SHN_LORESERVE || int(banner.Section) >= len(image.Sections) {
		return errors.New("kernel release symbol is missing or incompatible")
	}
	section := image.Sections[banner.Section]
	if section.Type != elf.SHT_PROGBITS || section.Flags&elf.SHF_ALLOC == 0 || section.Flags&elf.SHF_COMPRESSED != 0 || banner.Value < section.Addr {
		return errors.New("kernel release section is incompatible")
	}
	offset := banner.Value - section.Addr
	if offset > section.Size || banner.Size > section.Size-offset || section.Offset > uint64(before.Size) || section.Size > uint64(before.Size)-section.Offset {
		return errors.New("kernel release symbol is outside its file section")
	}
	data := make([]byte, int(banner.Size))
	if _, err = io.ReadFull(io.NewSectionReader(file, int64(section.Offset+offset), int64(banner.Size)), data); err != nil {
		return fmt.Errorf("read kernel release: %w", err)
	}
	if data[len(data)-1] != 0 || bytes.IndexByte(data[:len(data)-1], 0) >= 0 || !strings.HasPrefix(string(data), "Linux version "+release+" ") {
		return errors.New("kernel embedded release is incompatible")
	}
	return nil
}
