package main

import (
	"encoding/hex"
	"errors"
	"path/filepath"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

func consumeAgentToken(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("private credential path must be absolute")
	}
	dir, err := safefile.OpenDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	value, identity, err := dir.ReadRegularIdentity(filepath.Base(path), 0600, 65)
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
	removed, err := dir.RemoveIfIdentity(filepath.Base(path), identity)
	if err != nil || !removed {
		return nil, errors.New("private credential could not be consumed")
	}
	return key, nil
}
