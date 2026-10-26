package state

import (
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommitFailureRetainsAbsentOwnerAndReplaysAfterReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	failure := &protocol.Error{Code: "OPERATOR_ACTION", Message: "unsafe session", Retryable: false, OperationID: "operation"}
	sb := protocol.Sandbox{ID: "box", Generation: "11111111222233334444555555555555", State: "ABSENT", Error: failure}
	if err = store.CommitFailure(sb, "delete", "fingerprint", failure); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, ok := store.Sandbox("box")
	if !ok || !reflect.DeepEqual(owner, sb) {
		t.Fatal("failed deletion discarded ownership")
	}
	result, ok := store.Result("delete")
	if !ok || result.Fingerprint != "fingerprint" || !reflect.DeepEqual(result.Error, failure) {
		t.Fatal("terminal error replay lost after reopen")
	}
}
func TestCommitFailureRollsBackBothMapsOnPersistenceRefusal(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "state")
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := &protocol.Error{Code: "OPERATOR_ACTION", Message: "first", Retryable: false}
	if err = store.CommitFailure(protocol.Sandbox{ID: "box", State: "RELEASING"}, "key", "first", first); err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(base, "held")
	if err = os.Rename(dir, held); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = store.CommitFailure(protocol.Sandbox{ID: "other", State: "ABSENT"}, "key", "second", &protocol.Error{Code: "OPERATOR_ACTION"}); err == nil {
		t.Fatal("replacement directory accepted")
	}
	if !reflect.DeepEqual(before, store.Snapshot()) {
		t.Fatal("failed persistence changed ownership or replay")
	}
	after, err := os.ReadFile(filepath.Join(held, "state.json"))
	if err != nil || string(after) != string(data) {
		t.Fatal("original durable state changed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("replacement directory mutated")
	}
}
