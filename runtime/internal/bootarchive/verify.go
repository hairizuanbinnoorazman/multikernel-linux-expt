// Package bootarchive verifies the component projection of runtime-owned boot
// archives. It never extracts files or executes archive content.
package bootarchive

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

type Component struct {
	SHA256     string
	Executable bool
}

const maximumBytes = 512 << 20
const maximumEntries = 131072

// Verify accepts one gzip member containing one newc archive with zero padding.
// Critical files must be unique, root-owned, protected regular single-link
// files under protected real directories. Other archive contents remain under
// the administrator's whole-archive approval; this is not provenance validation.
func Verify(ctx context.Context, source io.Reader, required, optional map[string]Component) error {
	for _, policy := range []map[string]Component{required, optional} {
		for name, c := range policy {
			digest, err := hex.DecodeString(c.SHA256)
			if !safeName(name) || name == "." || err != nil || len(digest) != sha256.Size || strings.ToLower(c.SHA256) != c.SHA256 {
				return errors.New("invalid boot component policy")
			}
		}
	}
	for name := range optional {
		if _, ok := required[name]; ok {
			return errors.New("overlapping boot component policies")
		}
	}
	encoded := bufio.NewReader(source)
	compressed, err := gzip.NewReader(encoded)
	if err != nil {
		return fmt.Errorf("boot archive gzip: %w", err)
	}
	defer compressed.Close()
	compressed.Multistream(false)
	limited := &io.LimitedReader{R: compressed, N: maximumBytes + 1}
	read := func(n int) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if n < 0 || n > 1<<20 {
			return nil, errors.New("boot archive read exceeds bound")
		}
		b := make([]byte, n)
		_, err := io.ReadFull(limited, b)
		if limited.N == 0 {
			return nil, errors.New("boot archive exceeds decompressed limit")
		}
		return b, err
	}
	type entry struct{ mode, uid, gid, links uint64 }
	seen := map[string]entry{}
	found := map[string]bool{}
	padding := func(n int) error {
		b, err := read(n)
		if err != nil {
			return err
		}
		if !allZero(b) {
			return errors.New("nonzero boot archive alignment")
		}
		return nil
	}
	for count := 0; ; count++ {
		if count >= maximumEntries {
			return errors.New("boot archive entry limit")
		}
		header, err := read(110)
		if err != nil {
			return fmt.Errorf("read boot archive header: %w", err)
		}
		if string(header[:6]) != "070701" {
			return errors.New("boot archive must use newc without CRC")
		}
		var fields [13]uint64
		for i := range fields {
			for _, b := range header[6+i*8 : 14+i*8] {
				var digit byte
				switch {
				case b >= '0' && b <= '9':
					digit = b - '0'
				case b >= 'a' && b <= 'f':
					digit = b - 'a' + 10
				case b >= 'A' && b <= 'F':
					digit = b - 'A' + 10
				default:
					return errors.New("invalid boot archive hex field")
				}
				fields[i] = fields[i]*16 + uint64(digit)
			}
		}
		size, namesize := fields[6], fields[11]
		if namesize == 0 || namesize > 4096 || size > maximumBytes || fields[12] != 0 {
			return errors.New("boot archive name, size or checksum is invalid")
		}
		raw, err := read(int(namesize))
		if err != nil {
			return err
		}
		if raw[len(raw)-1] != 0 || bytes.IndexByte(raw[:len(raw)-1], 0) >= 0 || !utf8.Valid(raw[:len(raw)-1]) {
			return errors.New("invalid boot archive name")
		}
		name := strings.TrimPrefix(string(raw[:len(raw)-1]), "./")
		if err = padding(int((4 - (110+namesize)%4) % 4)); err != nil {
			return err
		}
		if name == "TRAILER!!!" {
			if string(raw[:len(raw)-1]) != name {
				return errors.New("noncanonical boot archive trailer")
			}
			if size != 0 {
				return errors.New("boot archive trailer has data")
			}
			buffer := make([]byte, 1<<20)
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				n, e := limited.Read(buffer)
				if limited.N == 0 {
					return errors.New("boot archive exceeds decompressed limit")
				}
				if !allZero(buffer[:n]) {
					return errors.New("boot archive has data after trailer")
				}
				if e != nil {
					if e != io.EOF {
						return e
					}
					break
				}
			}
			if _, err = encoded.ReadByte(); err != io.EOF {
				return errors.New("boot archive has trailing encoded data")
			}
			break
		}
		if !safeName(name) {
			return errors.New("unsafe boot archive path")
		}
		if _, ok := seen[name]; ok {
			return errors.New("duplicate normalized boot archive path")
		}
		seen[name] = entry{fields[1], fields[2], fields[3], fields[4]}
		component, critical := required[name]
		if !critical {
			component, critical = optional[name]
		}
		if critical {
			e := seen[name]
			if e.mode&0170000 != 0100000 || e.mode&07022 != 0 || e.uid != 0 || e.gid != 0 || e.links != 1 || e.mode&0400 == 0 || (component.Executable && e.mode&0100 == 0) {
				return fmt.Errorf("boot component %s has unsafe metadata", name)
			}
		}
		digest := sha256.New()
		for remaining := size; remaining > 0; {
			n := min(remaining, 1<<20)
			chunk, err := read(int(n))
			if err != nil {
				return err
			}
			if critical {
				_, _ = digest.Write(chunk)
			}
			remaining -= n
		}
		if critical {
			if hex.EncodeToString(digest.Sum(nil)) != component.SHA256 {
				return fmt.Errorf("boot component %s digest differs from approved artifact", name)
			}
			found[name] = true
		}
		if err = padding(int((4 - size%4) % 4)); err != nil {
			return err
		}
	}
	for name := range required {
		if !found[name] {
			return fmt.Errorf("boot archive lacks required component %s", name)
		}
	}
	for name := range found {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			e, ok := seen[parent]
			if !ok || e.mode&0170000 != 0040000 || e.mode&07022 != 0 || e.uid != 0 || e.gid != 0 {
				return fmt.Errorf("boot component %s has unsafe ancestor", name)
			}
		}
		if e, ok := seen["."]; ok && (e.mode&0170000 != 0040000 || e.mode&07022 != 0 || e.uid != 0 || e.gid != 0) {
			return errors.New("boot archive root directory is unsafe")
		}
	}
	return nil
}
func safeName(name string) bool {
	return name == "." || (name != "" && name != ".." && !strings.HasPrefix(name, "../") && !strings.HasPrefix(name, "/") && !strings.Contains(name, "\\") && path.Clean(name) == name)
}
func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
