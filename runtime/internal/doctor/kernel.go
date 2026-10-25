package doctor

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// KernelInventory does not execute Kerf. Missing sysfs is unavailable, never an
// empty healthy host. ReadDir's count bounds allocations even on a corrupt tree.
func KernelInventory(ctx context.Context, sysfs string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(sysfs)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open("instances")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(protocol.MaxOwnershipRecords + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > protocol.MaxOwnershipRecords {
		return nil, errors.New("kernel inventory limit")
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !idPattern.MatchString(entry.Name()) {
			return nil, errors.New("invalid kernel inventory")
		}
		ids = append(ids, entry.Name())
	}
	sort.Strings(ids)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
func BootIdentity() (string, error) {
	f, err := os.Open("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 38))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(b))
	if len(b) > 37 || !bootPattern.MatchString(value) {
		return "", errors.New("invalid boot identity")
	}
	return value, nil
}
