package kernelmanifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/bootarchive"
)

type archiveEntry struct {
	name             string
	data             []byte
	mode, uid, links uint32
}

func archiveBytes(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var raw bytes.Buffer
	appendEntry := func(e archiveEntry) {
		fields := [13]uint32{1, e.mode, e.uid, 0, e.links, 0, uint32(len(e.data)), 0, 0, 0, 0, uint32(len(e.name) + 1), 0}
		raw.WriteString("070701")
		for _, value := range fields {
			fmt.Fprintf(&raw, "%08x", value)
		}
		raw.WriteString(e.name)
		raw.WriteByte(0)
		for raw.Len()%4 != 0 {
			raw.WriteByte(0)
		}
		raw.Write(e.data)
		for raw.Len()%4 != 0 {
			raw.WriteByte(0)
		}
	}
	for _, e := range entries {
		appendEntry(e)
	}
	appendEntry(archiveEntry{name: "TRAILER!!!"})
	var result bytes.Buffer
	z := gzip.NewWriter(&result)
	if _, err := z.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}
func writeArchive(t *testing.T, output string, entries []archiveEntry) {
	t.Helper()
	if err := os.WriteFile(output, archiveBytes(t, entries), 0644); err != nil {
		t.Fatal(err)
	}
}
func baseEntries(t *testing.T, directory string) []archiveEntry {
	t.Helper()
	var entries []archiveEntry
	for _, name := range []string{"mk-agent", "mkvsock-relay", "mk_transport.ko"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, archiveEntry{name: name, data: data, mode: 0100755, links: 1})
	}
	return entries
}

func TestResolveRejectsHashApprovedArchiveComponentMismatch(t *testing.T) {
	for _, component := range []string{"mk-agent", "mkvsock-relay", "mk_transport.ko"} {
		t.Run(component, func(t *testing.T) {
			resolver, manifestPath := manifestFixture(t)
			rewriteFixtureManifest(t, manifestPath, func(m *Manifest) {
				entries := baseEntries(t, resolver.Directory)
				for i := range entries {
					if entries[i].name == component {
						entries[i].data = []byte("wrong approved component")
					}
				}
				writeArchive(t, m.Initramfs.Path, entries)
				m.Initramfs.SHA256 = digest(t, m.Initramfs.Path)
			})
			artifacts, err := resolver.Resolve("test")
			defer artifacts.Close()
			if err == nil || !strings.Contains(err.Error(), "digest differs") {
				t.Fatalf("component substitution accepted: %v", err)
			}
		})
	}
}

func TestBootArchiveStructureAndAuthority(t *testing.T) {
	resolver, _ := manifestFixture(t)
	entries := baseEntries(t, resolver.Directory)
	required := map[string]bootarchive.Component{"mk-agent": {SHA256: digest(t, filepath.Join(resolver.Directory, "mk-agent")), Executable: true}}
	tests := map[string]func([]archiveEntry) []archiveEntry{
		"missing":              func(e []archiveEntry) []archiveEntry { return e[1:] },
		"duplicate":            func(e []archiveEntry) []archiveEntry { return append(e, e[0]) },
		"normalized-duplicate": func(e []archiveEntry) []archiveEntry { v := e[0]; v.name = "./mk-agent"; return append(e, v) },
		"writable":             func(e []archiveEntry) []archiveEntry { e[0].mode |= 0020; return e },
		"setuid":               func(e []archiveEntry) []archiveEntry { e[0].mode |= 04000; return e },
		"wrong-owner":          func(e []archiveEntry) []archiveEntry { e[0].uid = 1000; return e },
		"hardlink":             func(e []archiveEntry) []archiveEntry { e[0].links = 2; return e },
		"symlink":              func(e []archiveEntry) []archiveEntry { e[0].mode = 0120777; return e },
		"nonexecutable":        func(e []archiveEntry) []archiveEntry { e[0].mode = 0100644; return e },
		"traversal":            func(e []archiveEntry) []archiveEntry { e[0].name = "../mk-agent"; return e },
		"unsafe-root": func(e []archiveEntry) []archiveEntry {
			return append(e, archiveEntry{name: ".", mode: 0040777, links: 2})
		},
	}
	names := make([]string, 0, len(tests))
	for name := range tests {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			copyEntries := append([]archiveEntry(nil), entries...)
			if err := bootarchive.Verify(t.Context(), bytes.NewReader(archiveBytes(t, tests[name](copyEntries))), required, nil); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	valid := archiveBytes(t, entries)
	for name, data := range map[string][]byte{"truncated": valid[:len(valid)-4], "encoded-tail": append(append([]byte(nil), valid...), 0), "second-gzip": append(append([]byte(nil), valid...), valid...)} {
		t.Run(name, func(t *testing.T) {
			if err := bootarchive.Verify(t.Context(), bytes.NewReader(data), required, nil); err == nil {
				t.Fatal("invalid encoding accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := bootarchive.Verify(ctx, bytes.NewReader(valid), required, nil); err == nil {
		t.Fatal("canceled verification succeeded")
	}
}

func TestPreparedArchiveBindsCurrentProfileAndRewinds(t *testing.T) {
	resolver, manifestPath := manifestFixture(t)
	module := filepath.Join(resolver.Directory, "nbd.ko")
	writeModuleFixture(t, module, "name=nbd\x00vermagic=test-release SMP\x00")
	rewriteFixtureManifest(t, manifestPath, func(m *Manifest) { m.Modules = []Artifact{{Path: module, SHA256: digest(t, module)}} })
	artifacts, err := resolver.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.Close()
	transport, err := os.ReadFile(filepath.Join(resolver.Directory, "mk_transport.ko"))
	if err != nil {
		t.Fatal(err)
	}
	nbd, err := os.ReadFile(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"valid", "transport", "nbd", "ancestor", "missing-nbd"} {
		t.Run(target, func(t *testing.T) {
			entries := []archiveEntry{{name: "lib", mode: 0040755, links: 2}, {name: "lib/mk_transport.ko", data: transport, mode: 0100644, links: 1}, {name: "lib/nbd.ko", data: nbd, mode: 0100644, links: 1}}
			switch target {
			case "transport":
				entries[1].data = []byte("wrong")
			case "nbd":
				entries[2].data = []byte("wrong")
			case "ancestor":
				entries[0].mode = 0120777
			case "missing-nbd":
				entries = entries[:2]
			}
			path := filepath.Join(t.TempDir(), "prepared.gz")
			writeArchive(t, path, entries)
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			err = artifacts.VerifyPreparedInitramfs(t.Context(), file)
			if (err == nil) != (target == "valid") {
				t.Fatalf("%s: %v", target, err)
			}
			offset, seekErr := file.Seek(0, 1)
			if seekErr != nil || offset != 0 {
				t.Fatalf("descriptor not rewound: %d %v", offset, seekErr)
			}
		})
	}
}
