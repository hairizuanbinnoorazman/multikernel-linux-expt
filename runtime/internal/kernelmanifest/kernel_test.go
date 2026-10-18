package kernelmanifest

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// A real ELF64 executable section/symbol layout with a bounded linux_banner.
func writeKernelFixture(t *testing.T, path, banner string) {
	t.Helper()
	names := "\x00.shstrtab\x00.strtab\x00.symtab\x00.rodata\x00"
	stringsTable := "\x00linux_banner\x00"
	stringsOffset := 64 + len(names)
	symbolOffset := (stringsOffset + len(stringsTable) + 7) &^ 7
	bannerOffset := symbolOffset + 48
	sectionOffset := (bannerOffset + len(banner) + 7) &^ 7
	data := make([]byte, sectionOffset+5*64)
	copy(data, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	put16, put32, put64 := binary.LittleEndian.PutUint16, binary.LittleEndian.PutUint32, binary.LittleEndian.PutUint64
	put16(data[16:18], uint16(elf.ET_EXEC))
	put16(data[18:20], uint16(elf.EM_X86_64))
	put32(data[20:24], 1)
	put64(data[40:48], uint64(sectionOffset))
	put16(data[52:54], 64)
	put16(data[58:60], 64)
	put16(data[60:62], 5)
	put16(data[62:64], 1)
	copy(data[64:], names)
	copy(data[stringsOffset:], stringsTable)
	copy(data[bannerOffset:], banner)
	const address = 0xffffffff81000000
	symbol := data[symbolOffset+24 : symbolOffset+48]
	put32(symbol[0:4], 1)
	symbol[4] = byte(elf.STB_GLOBAL)<<4 | byte(elf.STT_OBJECT)
	put16(symbol[6:8], 4)
	put64(symbol[8:16], address)
	put64(symbol[16:24], uint64(len(banner)))
	for i, entry := range []struct {
		name         uint32
		kind         elf.SectionType
		offset, size uint64
	}{{1, elf.SHT_STRTAB, 64, uint64(len(names))}, {11, elf.SHT_STRTAB, uint64(stringsOffset), uint64(len(stringsTable))}, {19, elf.SHT_SYMTAB, uint64(symbolOffset), 48}, {27, elf.SHT_PROGBITS, uint64(bannerOffset), uint64(len(banner))}} {
		section := data[sectionOffset+(i+1)*64 : sectionOffset+(i+2)*64]
		put32(section[0:4], entry.name)
		put32(section[4:8], uint32(entry.kind))
		put64(section[24:32], entry.offset)
		put64(section[32:40], entry.size)
		if i == 2 {
			put32(section[40:44], 2)
			put64(section[56:64], 24)
		}
		if i == 3 {
			put64(section[8:16], uint64(elf.SHF_ALLOC))
			put64(section[16:24], address)
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveBindsEmbeddedKernelRelease(t *testing.T) {
	for _, item := range []struct {
		name, banner string
		accept       bool
	}{
		{"correct", "Linux version test-release (builder)\n\x00", true},
		{"wrong-release", "Linux version other-release (builder)\n\x00", false},
		{"prefix-alias", "Linux version test-release-extra (builder)\n\x00", false},
		{"arbitrary-ELF-data", "not a Linux kernel\x00", false},
		{"unterminated", "Linux version test-release (builder)\n", false},
		{"embedded-nul", "Linux version test-release \x00forged\x00", false},
	} {
		t.Run(item.name, func(t *testing.T) {
			r, path := manifestFixture(t)
			rewriteFixtureManifest(t, path, func(m *Manifest) {
				writeKernelFixture(t, m.Kernel.Path, item.banner)
				m.Kernel.SHA256 = digest(t, m.Kernel.Path)
			})
			artifacts, err := r.Resolve("test")
			if err == nil {
				defer artifacts.Close()
			}
			if (err == nil) != item.accept {
				t.Fatalf("accept=%v err=%v", item.accept, err)
			}
		})
	}
}

func TestKernelMetadataBounds(t *testing.T) {
	for _, item := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"relocatable", func(b []byte) { binary.LittleEndian.PutUint16(b[16:18], uint16(elf.ET_REL)) }},
		{"wrong-machine", func(b []byte) { binary.LittleEndian.PutUint16(b[18:20], uint16(elf.EM_AARCH64)) }},
		{"oversized-symbol-table", func(b []byte) {
			s := int(binary.LittleEndian.Uint64(b[40:48]))
			binary.LittleEndian.PutUint64(b[s+3*64+32:], 33<<20)
		}},
		{"compressed-symbol-table", func(b []byte) {
			s := int(binary.LittleEndian.Uint64(b[40:48]))
			binary.LittleEndian.PutUint64(b[s+3*64+8:], uint64(elf.SHF_COMPRESSED))
		}},
		{"missing-symbol", func(b []byte) { b[64+len("\x00.shstrtab\x00.strtab\x00.symtab\x00.rodata\x00")+1] = 'x' }},
		{"oversized-banner", func(b []byte) {
			s := int(binary.LittleEndian.Uint64(b[40:48]))
			sym := int(binary.LittleEndian.Uint64(b[s+3*64+24:]))
			binary.LittleEndian.PutUint64(b[sym+24+16:], 4097)
		}},
		{"nobits-release", func(b []byte) {
			s := int(binary.LittleEndian.Uint64(b[40:48]))
			binary.LittleEndian.PutUint32(b[s+4*64+4:], uint32(elf.SHT_NOBITS))
		}},
		{"outside-section", func(b []byte) {
			s := int(binary.LittleEndian.Uint64(b[40:48]))
			sym := int(binary.LittleEndian.Uint64(b[s+3*64+24:]))
			binary.LittleEndian.PutUint64(b[sym+24+8:], 0xffffffff82000000)
		}},
	} {
		t.Run(item.name, func(t *testing.T) {
			r, path := manifestFixture(t)
			rewriteFixtureManifest(t, path, func(m *Manifest) {
				b, err := os.ReadFile(m.Kernel.Path)
				if err != nil {
					t.Fatal(err)
				}
				item.mutate(b)
				if err = os.WriteFile(m.Kernel.Path, b, 0644); err != nil {
					t.Fatal(err)
				}
				m.Kernel.SHA256 = digest(t, m.Kernel.Path)
			})
			if _, err := r.Resolve("test"); err == nil {
				t.Fatal("incompatible hash-approved kernel accepted")
			}
		})
	}
}

func TestInstalledKernelRelease(t *testing.T) {
	path, release := os.Getenv("MK_TEST_KERNEL"), os.Getenv("MK_TEST_KERNEL_RELEASE")
	if path == "" || release == "" {
		t.Skip("installed kernel fixture not configured")
	}
	a := Artifact{Path: path, SHA256: digest(t, path)}
	if err := verifyKernel(a, uint32(os.Getuid()), release); err != nil {
		t.Fatal(err)
	}
	if err := verifyKernel(a, uint32(os.Getuid()), release+"-wrong"); err == nil || !strings.Contains(err.Error(), "embedded release") {
		t.Fatalf("wrong release: %v", err)
	}
}
