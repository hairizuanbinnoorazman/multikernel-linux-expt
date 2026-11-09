package safefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPendingEntryCreatedBetweenStatAndListingIsObserved(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenOwnedDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	present, err := directory.entryOrRemovalPendingWithHook("marker", 256, func() {
		if err := os.WriteFile(filepath.Join(root, "marker"), []byte("private"), 0600); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil || !present {
		t.Fatal("concurrent marker publication was missed")
	}
}
