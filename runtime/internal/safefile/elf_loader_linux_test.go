package safefile

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func syntheticLoaderELF(path string) []byte {
	data := make([]byte, 128+len(path)+1)
	copy(data, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint64(data[32:40], 64)
	binary.LittleEndian.PutUint16(data[52:54], 64)
	binary.LittleEndian.PutUint16(data[54:56], 56)
	binary.LittleEndian.PutUint16(data[56:58], 1)
	binary.LittleEndian.PutUint32(data[64:68], 3)
	binary.LittleEndian.PutUint64(data[72:80], 128)
	binary.LittleEndian.PutUint64(data[96:104], uint64(len(path)+1))
	copy(data[128:], path)
	return data
}

func TestELFLoaderBoundsAndPaths(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]byte)
		accept bool
	}{
		{"valid", func([]byte) {}, true},
		{"class", func(d []byte) { d[4] = 9 }, false},
		{"byte-order", func(d []byte) { d[5] = 9 }, false},
		{"version", func(d []byte) { d[6] = 9 }, false},
		{"entry-size", func(d []byte) { binary.LittleEndian.PutUint16(d[54:56], 65535) }, false},
		{"count", func(d []byte) { binary.LittleEndian.PutUint16(d[56:58], 129) }, false},
		{"offset-overflow", func(d []byte) { binary.LittleEndian.PutUint64(d[32:40], ^uint64(0)) }, false},
		{"segment-overflow", func(d []byte) { binary.LittleEndian.PutUint64(d[72:80], ^uint64(0)) }, false},
		{"path-size", func(d []byte) { binary.LittleEndian.PutUint64(d[96:104], 4097) }, false},
		{"missing-nul", func(d []byte) { d[len(d)-1] = 'x' }, false},
		{"interior-nul", func(d []byte) { d[130] = 0 }, false},
		{"relative", func(d []byte) { d[128] = 'x' }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			data := syntheticLoaderELF("/bin/sh")
			test.mutate(data)
			path := filepath.Join(root, "program")
			if err := os.WriteFile(path, data, 0700); err != nil {
				t.Fatal(err)
			}
			file, err := OpenTrustedExecutable(path, 1024)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			err = ValidateScriptInterpreter(file, nil)
			if (err == nil) != test.accept {
				t.Fatalf("accept=%v error=%v", test.accept, err)
			}
			prefix := make([]byte, 4)
			if _, err := file.Read(prefix); err != nil || !bytes.Equal(prefix, []byte{0x7f, 'E', 'L', 'F'}) {
				t.Fatal("inspection altered file offset:", err)
			}
		})
	}
}

func TestELFUnsafeLoaderRejectedBeforeCanary(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "g7ld-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	unsafe := filepath.Join(root, "u")
	if err := os.Mkdir(unsafe, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0777); err != nil {
		t.Fatal(err)
	}
	program, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	image, err := elf.NewFile(bytes.NewReader(program))
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	if image.Class != elf.ELFCLASS64 || image.Data != elf.ELFDATA2LSB {
		t.Skip("real loader patch requires local ELF64 little-endian shell")
	}
	var segment *elf.Prog
	var index int
	for i, p := range image.Progs {
		if p.Type == elf.PT_INTERP {
			segment = p
			index = i
			break
		}
	}
	if segment == nil {
		t.Skip("local shell has no dynamic loader")
	}
	loaderPath := filepath.Join(unsafe, "l")
	if uint64(len(loaderPath)+1) > segment.Filesz {
		t.Fatal("short fixture path exceeds loader slot")
	}
	original := string(bytes.TrimRight(program[segment.Off:segment.Off+segment.Filesz], "\x00"))
	loader, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loaderPath, loader, 0700); err != nil {
		t.Fatal(err)
	}
	copy(program[segment.Off:], append([]byte(loaderPath), 0))
	phoff := binary.LittleEndian.Uint64(program[32:40])
	phsize := binary.LittleEndian.Uint16(program[54:56])
	position := phoff + uint64(index)*uint64(phsize)
	binary.LittleEndian.PutUint64(program[position+32:position+40], uint64(len(loaderPath)+1))
	binary.LittleEndian.PutUint64(program[position+40:position+48], uint64(len(loaderPath)+1))
	path := filepath.Join(root, "shell")
	if err := os.WriteFile(path, program, 0700); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(root, "executed")
	// Establish that this patched image is executable without the validator.
	if output, err := exec.Command(path, "-c", "printf baseline > \"$1\"", "fixture", canary).CombinedOutput(); err != nil {
		t.Fatalf("baseline patched shell: %v %s", err, output)
	}
	if data, err := os.ReadFile(canary); err != nil || string(data) != "baseline" {
		t.Fatalf("baseline marker=%q err=%v", data, err)
	}
	if err := os.Remove(canary); err != nil {
		t.Fatal(err)
	}
	file, err := OpenTrustedExecutable(path, 256<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := ValidateScriptInterpreter(file, nil); err == nil {
		t.Fatal("unsafe loader accepted")
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatal("validation mutated canary:", err)
	}
	t.Log("PASS: patched native shell executes with copied unsafe loader; validator rejects it without canary mutation")
}
