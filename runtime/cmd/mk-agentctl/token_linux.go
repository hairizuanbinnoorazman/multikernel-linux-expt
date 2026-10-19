package main

import (
	"encoding/hex"
	"errors"
	"path/filepath"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

// Read only: lab builders and post-run redaction share this primary-side file.
// Unlike the child bootstrap token, its lifetime belongs to the harness.
func readControlToken(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("private credential path must be absolute")
	}
	directory, err := safefile.OpenDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	value, _, err := directory.ReadRegularIdentity(filepath.Base(path), 0600, 65)
	if err != nil {
		return nil, err
	}
	if len(value) != 65 || value[64] != '\n' {
		return nil, errors.New("invalid private credential format")
	}
	key, err := hex.DecodeString(string(value[:64]))
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid private credential format")
	}
	return key, nil
}
