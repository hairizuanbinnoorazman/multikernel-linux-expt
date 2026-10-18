package kernelmanifest

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Minimal ELF64 relocatable with real section geometry, rather than a mocked
// metadata extractor. Production evidence separately exercises installed modules.
func writeModuleFixture(t *testing.T, path, metadata string) {
	t.Helper()
	names := []byte("\x00.shstrtab\x00.modinfo\x00")
	metadataOffset := 64 + len(names)
	sectionOffset := (metadataOffset + len(metadata) + 7) &^ 7
	data := make([]byte, sectionOffset+3*64)
	copy(data, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	put16 := binary.LittleEndian.PutUint16
	put32 := binary.LittleEndian.PutUint32
	put64 := binary.LittleEndian.PutUint64
	put16(data[16:18], uint16(elf.ET_REL))
	put16(data[18:20], uint16(elf.EM_X86_64))
	put32(data[20:24], 1)
	put64(data[40:48], uint64(sectionOffset))
	put16(data[52:54], 64)
	put16(data[58:60], 64)
	put16(data[60:62], 3)
	put16(data[62:64], 1)
	copy(data[64:], names)
	copy(data[metadataOffset:], metadata)
	shstr := data[sectionOffset+64 : sectionOffset+128]
	put32(shstr[0:4], 1)
	put32(shstr[4:8], uint32(elf.SHT_STRTAB))
	put64(shstr[24:32], 64)
	put64(shstr[32:40], uint64(len(names)))
	modinfo := data[sectionOffset+128:]
	put32(modinfo[0:4], 11)
	put32(modinfo[4:8], uint32(elf.SHT_PROGBITS))
	put64(modinfo[24:32], uint64(metadataOffset))
	put64(modinfo[32:40], uint64(len(metadata)))
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func rewriteFixtureManifest(t *testing.T, path string, change func(*Manifest)) Manifest {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	change(&manifest)
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestResolveRejectsModuleIdentityMismatch(t *testing.T) {
	for _, item := range []struct{ name, metadata string }{
		{"wrong-release", "name=mk_transport\x00vermagic=other-release SMP\x00"},
		{"release-prefix-alias", "name=mk_transport\x00vermagic=test-release-extra SMP\x00"},
		{"wrong-transport", "name=nbd\x00vermagic=test-release SMP\x00"},
		{"missing-vermagic", "name=mk_transport\x00"},
		{"duplicate-vermagic", "name=mk_transport\x00vermagic=test-release SMP\x00vermagic=other-release SMP\x00"},
		{"duplicate-name", "name=mk_transport\x00name=nbd\x00vermagic=test-release SMP\x00"},
		{"unterminated", "name=mk_transport\x00vermagic=test-release SMP"},
	} {
		t.Run(item.name, func(t *testing.T) {
			resolver, path := manifestFixture(t)
			rewriteFixtureManifest(t, path, func(m *Manifest) {
				writeModuleFixture(t, m.Transport.Module.Path, item.metadata)
				m.Transport.Module.SHA256 = digest(t, m.Transport.Module.Path)
			})
			if _, err := resolver.Resolve("test"); err == nil {
				t.Fatal("hash-approved incompatible module accepted")
			}
		})
	}
}

func TestResolveChecksSupportModuleRelease(t *testing.T) {
	for _, release := range []string{"test-release", "other-release"} {
		t.Run(release, func(t *testing.T) {
			resolver, path := manifestFixture(t)
			extra := filepath.Join(resolver.Directory, "nbd.ko")
			writeModuleFixture(t, extra, "name=nbd\x00vermagic="+release+" SMP\x00")
			rewriteFixtureManifest(t, path, func(m *Manifest) { m.Modules = []Artifact{{Path: extra, SHA256: digest(t, extra)}} })
			artifacts, err := resolver.Resolve("test")
			defer artifacts.Close()
			if (err == nil) != (release == "test-release") {
				t.Fatalf("support module release %s: %v", release, err)
			}
		})
	}
}

func TestModuleMetadataBounds(t *testing.T) {
	for _, item := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"architecture", func(d []byte) { binary.LittleEndian.PutUint16(d[18:20], uint16(elf.EM_AARCH64)) }},
		{"executable", func(d []byte) { binary.LittleEndian.PutUint16(d[16:18], uint16(elf.ET_EXEC)) }},
		{"oversized", func(d []byte) {
			i := int(binary.LittleEndian.Uint64(d[40:48])) + 128
			binary.LittleEndian.PutUint64(d[i+32:i+40], 1<<20+1)
		}},
		{"compressed", func(d []byte) {
			i := int(binary.LittleEndian.Uint64(d[40:48])) + 128
			binary.LittleEndian.PutUint64(d[i+8:i+16], uint64(elf.SHF_COMPRESSED))
		}},
		{"nobits", func(d []byte) {
			i := int(binary.LittleEndian.Uint64(d[40:48])) + 128
			binary.LittleEndian.PutUint32(d[i+4:i+8], uint32(elf.SHT_NOBITS))
		}},
	} {
		t.Run(item.name, func(t *testing.T) {
			resolver, path := manifestFixture(t)
			rewriteFixtureManifest(t, path, func(m *Manifest) {
				d, err := os.ReadFile(m.Transport.Module.Path)
				if err != nil {
					t.Fatal(err)
				}
				item.mutate(d)
				if err = os.WriteFile(m.Transport.Module.Path, d, 0644); err != nil {
					t.Fatal(err)
				}
				m.Transport.Module.SHA256 = digest(t, m.Transport.Module.Path)
			})
			if _, err := resolver.Resolve("test"); err == nil {
				t.Fatal("unsafe ELF metadata accepted")
			}
		})
	}
}

type mutationGuardBackend struct {
	kerf.Backend
	calls int
}

func (b *mutationGuardBackend) EnsurePool(context.Context) error {
	b.calls++
	return errors.New("unexpected pool mutation")
}
func (b *mutationGuardBackend) Create(context.Context, protocol.Sandbox) error {
	b.calls++
	return errors.New("unexpected create mutation")
}
func (b *mutationGuardBackend) Observe(context.Context, string) (string, error) {
	b.calls++
	return "ABSENT", nil
}
func (b *mutationGuardBackend) ReleasePool(context.Context) error {
	b.calls++
	return nil
}

func TestArtifactMismatchRefusesBeforePoolOrStateMutation(t *testing.T) {
	for _, target := range []string{"kernel", "kernel-release", "initramfs", "transport", "support-release"} {
		t.Run(target, func(t *testing.T) {
			resolver, path := manifestFixture(t)
			var manifest Manifest
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			switch target {
			case "kernel":
				err = os.WriteFile(manifest.Kernel.Path, []byte("substituted kernel"), 0755)
			case "kernel-release":
				rewriteFixtureManifest(t, path, func(m *Manifest) {
					writeKernelFixture(t, m.Kernel.Path, "Linux version other-release (fixture)\n\x00")
					m.Kernel.SHA256 = digest(t, m.Kernel.Path)
				})
			case "initramfs":
				err = os.WriteFile(manifest.Initramfs.Path, []byte("substituted initramfs"), 0644)
			case "transport":
				rewriteFixtureManifest(t, path, func(m *Manifest) {
					writeModuleFixture(t, m.Transport.Module.Path, "name=mk_transport\x00vermagic=other-release SMP\x00")
					m.Transport.Module.SHA256 = digest(t, m.Transport.Module.Path)
				})
			case "support-release":
				extra := filepath.Join(resolver.Directory, "nbd.ko")
				writeModuleFixture(t, extra, "name=nbd\x00vermagic=other-release SMP\x00")
				rewriteFixtureManifest(t, path, func(m *Manifest) { m.Modules = []Artifact{{Path: extra, SHA256: digest(t, extra)}} })
			}
			if err != nil {
				t.Fatal(err)
			}
			store, err := state.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			backend := &mutationGuardBackend{}
			service := lifecycle.New(store, backend, lifecycle.Artifacts{})
			service.SetArtifactResolver(resolver)
			config := protocol.SandboxConfig{SchemaVersion: 1, ID: "box-a", CPUs: []int{8}, MemoryBytes: 1 << 30, KernelManifest: "test", Bundle: "/bundle/box-a", BundleIdentity: protocol.DirectoryIdentity{Device: 1, Inode: 2}, AgentPort: 7001, ChildCID: 77}
			_, apiErr := service.Create(context.Background(), config, "create-key")
			if apiErr == nil || apiErr.Code != "FAILED_PRECONDITION" || backend.calls != 0 {
				t.Fatalf("error=%+v backend calls=%d", apiErr, backend.calls)
			}
			entries, journalErr := store.JournalEntries()
			if journalErr != nil || len(entries) != 0 {
				t.Fatalf("rejected artifact changed journal: %v, %v", entries, journalErr)
			}
			if len(store.Snapshot().Sandboxes) != 0 || len(store.Snapshot().Results) != 0 {
				t.Fatal("rejected artifact changed runtime state")
			}
		})
	}
}

func TestInstalledModuleMetadata(t *testing.T) {
	path, release := os.Getenv("MK_TEST_MODULE"), os.Getenv("MK_TEST_MODULE_RELEASE")
	if path == "" || release == "" {
		t.Skip("installed-module fixture not configured")
	}
	artifact := Artifact{Path: path, SHA256: digest(t, path)}
	if err := verifyModule(artifact, uint32(os.Getuid()), release, "mk_transport"); err != nil {
		t.Fatal(err)
	}
	if err := verifyModule(artifact, uint32(os.Getuid()), release+"-wrong", "mk_transport"); err == nil || !strings.Contains(err.Error(), "kernel release") {
		t.Fatalf("wrong-release result: %v", err)
	}
	if support := os.Getenv("MK_TEST_SUPPORT_MODULE"); support != "" {
		if err := verifyModule(Artifact{Path: support, SHA256: digest(t, support)}, uint32(os.Getuid()), release, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstalledManifest(t *testing.T) {
	directory, name := os.Getenv("MK_TEST_MANIFEST_DIRECTORY"), os.Getenv("MK_TEST_MANIFEST_NAME")
	if directory == "" || name == "" {
		t.Skip("installed manifest fixture not configured")
	}
	artifacts, err := (Resolver{Directory: directory, RequiredUID: uint32(os.Getuid())}).Resolve(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifacts.Close(); err != nil {
		t.Fatal(err)
	}
}
