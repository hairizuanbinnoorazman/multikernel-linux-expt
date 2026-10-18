package safefile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Inspect only the bounded ELF/program-header data required to select a
// loader. Do not parse sections or allocate attacker-supplied table sizes.
func elfLoaderPath(file *os.File, size int64) (string, error) {
	read := func(offset uint64, buffer []byte) error {
		if size < 0 || offset > uint64(size) || uint64(len(buffer)) > uint64(size)-offset {
			return errors.New("ELF field lies outside executable")
		}
		n, err := unix.Pread(int(file.Fd()), buffer, int64(offset))
		if err != nil || n != len(buffer) {
			return errors.Join(errors.New("ELF field read is truncated"), err)
		}
		return nil
	}
	header := make([]byte, 64)
	if size < 16 {
		return "", errors.New("ELF identity is truncated")
	}
	if err := read(0, header[:16]); err != nil {
		return "", err
	}
	if !bytes.Equal(header[:4], []byte{0x7f, 'E', 'L', 'F'}) || header[6] != 1 {
		return "", errors.New("unsupported ELF identity")
	}
	var order binary.ByteOrder
	switch header[5] {
	case 1:
		order = binary.LittleEndian
	case 2:
		order = binary.BigEndian
	default:
		return "", errors.New("unsupported ELF byte order")
	}
	var table uint64
	var count, entrySize, headerSize uint16
	var offsetField, sizeField int
	switch header[4] {
	case 1:
		if err := read(0, header[:52]); err != nil {
			return "", err
		}
		table = uint64(order.Uint32(header[28:32]))
		entrySize = order.Uint16(header[42:44])
		count = order.Uint16(header[44:46])
		headerSize = order.Uint16(header[40:42])
		offsetField, sizeField = 4, 16
		if entrySize != 32 || headerSize != 52 {
			return "", errors.New("unsupported ELF32 header sizes")
		}
	case 2:
		if err := read(0, header); err != nil {
			return "", err
		}
		table = order.Uint64(header[32:40])
		entrySize = order.Uint16(header[54:56])
		count = order.Uint16(header[56:58])
		headerSize = order.Uint16(header[52:54])
		offsetField, sizeField = 8, 32
		if entrySize != 56 || headerSize != 64 {
			return "", errors.New("unsupported ELF64 header sizes")
		}
	default:
		return "", errors.New("unsupported ELF class")
	}
	if order.Uint32(header[20:24]) != 1 || (order.Uint16(header[16:18]) != 2 && order.Uint16(header[16:18]) != 3) {
		return "", errors.New("ELF is not an executable/shared image")
	}
	if count == 0 || count > 128 || table < uint64(headerSize) || table > uint64(size) || uint64(count)*uint64(entrySize) > uint64(size)-table {
		return "", errors.New("ELF program-header table exceeds bounds")
	}
	program := make([]byte, int(entrySize))
	loader := ""
	for index := uint16(0); index < count; index++ {
		if err := read(table+uint64(index)*uint64(entrySize), program); err != nil {
			return "", err
		}
		if order.Uint32(program[:4]) != 3 {
			continue
		} // PT_INTERP
		if loader != "" {
			return "", errors.New("duplicate ELF loader segment")
		}
		var offset, length uint64
		if header[4] == 1 {
			offset = uint64(order.Uint32(program[offsetField : offsetField+4]))
			length = uint64(order.Uint32(program[sizeField : sizeField+4]))
		} else {
			offset = order.Uint64(program[offsetField : offsetField+8])
			length = order.Uint64(program[sizeField : sizeField+8])
		}
		if length < 2 || length > 4096 {
			return "", errors.New("ELF loader path exceeds bounds")
		}
		path := make([]byte, int(length))
		if err := read(offset, path); err != nil {
			return "", err
		}
		if path[len(path)-1] != 0 || bytes.IndexByte(path[:len(path)-1], 0) >= 0 || bytes.ContainsAny(path, "\r\n") {
			return "", errors.New("ELF loader path is not canonical NUL-terminated text")
		}
		loader = string(path[:len(path)-1])
	}
	return loader, nil
}

func validateELFLoader(file *os.File, size int64, environment []string, depth int) error {
	path, err := elfLoaderPath(file, size)
	if err != nil {
		return fmt.Errorf("ELF loader metadata rejected: %w", err)
	}
	if path == "" {
		return nil
	}
	loader, err := openTrustedInterpreter(path, 256<<20)
	if err != nil {
		return fmt.Errorf("ELF loader path rejected: %w", err)
	}
	defer loader.Close()
	header := make([]byte, 4)
	n, err := unix.Pread(int(loader.Fd()), header, 0)
	if err != nil || n != 4 || !bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}) {
		return errors.New("ELF loader must be a native ELF image")
	}
	return validateScriptInterpreter(loader, environment, depth+1)
}
