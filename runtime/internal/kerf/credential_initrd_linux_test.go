package kerf

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCredentialInitrdPreservesSourceAndUnpacksPrivateFile(t *testing.T) {
	if _, err := exec.LookPath("cpio"); err != nil {
		t.Skip("independent cpio decoder unavailable")
	}
	source, err := os.CreateTemp(t.TempDir(), "approved-initrd")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	prefix := []byte("approved-initramfs\n")
	if _, err = source.Write(prefix); err != nil {
		t.Fatal(err)
	}
	token := append(bytes.Repeat([]byte("ab"), 32), '\n')
	private, err := credentialInitrd(source, token)
	if err != nil {
		t.Fatal(err)
	}
	defer private.Close()
	actual, err := io.ReadAll(private)
	if err != nil || !bytes.HasPrefix(actual, prefix) {
		t.Fatal("source prefix changed", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(actual[len(prefix):]))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	extracted := t.TempDir()
	decode := exec.Command("cpio", "-idm", "--quiet", "--no-preserve-owner")
	decode.Dir = extracted
	decode.Stdin = bytes.NewReader(archive)
	if output, err := decode.CombinedOutput(); err != nil {
		t.Fatalf("independent archive decode: %v: %s", err, output)
	}
	path := filepath.Join(extracted, ".mk-agent", "token")
	value, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(value, token) {
		t.Fatal("credential archive contents differ", err)
	}
	for path, mode := range map[string]os.FileMode{filepath.Dir(path): 0700, path: 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("unsafe credential archive mode", err)
		}
	}
	if _, err := private.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("private initramfs is not sealed")
	}
	unchanged, err := os.ReadFile(source.Name())
	if err != nil || !bytes.Equal(unchanged, prefix) {
		t.Fatal("approved source modified", err)
	}
}
