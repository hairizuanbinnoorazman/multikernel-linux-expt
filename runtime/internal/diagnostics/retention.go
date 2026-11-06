package diagnostics

import (
	"errors"
	"maps"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

const MaxDirectoryFiles = 256
const MaxDirectoryBytes int64 = 1 << 30

// DirectoryBudget limits completed bundle logical bytes and entry count in a
// dedicated private directory for this capture. It deletes/rotates nothing and
// is not a filesystem reservation or a node-wide retention policy.
type DirectoryBudget struct {
	MaxFiles int
	MaxBytes int64
}

func (b DirectoryBudget) Valid() bool {
	return b.MaxFiles > 0 && b.MaxFiles <= MaxDirectoryFiles && b.MaxBytes > 0 && b.MaxBytes <= MaxDirectoryBytes
}

type directoryInventory struct {
	files map[string]safefile.Identity
	bytes int64
}

func inventory(parent *safefile.Directory) (directoryInventory, error) {
	names, err := parent.EntryNames(MaxDirectoryFiles)
	if err != nil {
		return directoryInventory{}, err
	}
	view := directoryInventory{files: make(map[string]safefile.Identity, len(names))}
	for _, name := range names {
		if !destinationName.MatchString(name) {
			return directoryInventory{}, errors.New("unexpected diagnostic directory entry")
		}
		file, identity, err := parent.OpenPrivateFile(name, MaxBundleBytes, false)
		if err != nil {
			return directoryInventory{}, err
		}
		closeErr := file.Close()
		if closeErr != nil || identity.Mode&0777 != 0600 {
			return directoryInventory{}, errors.New("unsafe diagnostic directory file metadata")
		}
		view.files[name] = identity
		view.bytes += identity.Size
	}
	return view, nil
}
func (a directoryInventory) same(b directoryInventory) bool {
	return a.bytes == b.bytes && maps.Equal(a.files, b.files)
}
