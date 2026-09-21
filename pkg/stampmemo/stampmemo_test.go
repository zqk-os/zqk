package stampmemo

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTable_loadOnceUntilStampChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stamp.txt")
	if err := fileutil.WriteStandardFile(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	var loads int
	var tab Table[string]
	load := func() (string, error) {
		loads++
		data, err := fileutil.ReadFile(path)
		return string(data), err
	}
	got, err := tab.Load("k", Of(path), load)
	if err != nil || got != "one" || loads != 1 {
		t.Fatalf("first: got=%q loads=%d err=%v", got, loads, err)
	}
	again, err := tab.Load("k", Of(path), load)
	if err != nil || again != "one" || loads != 1 {
		t.Fatalf("hit: got=%q loads=%d err=%v", again, loads, err)
	}
	if err := fileutil.WriteStandardFile(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	updated, err := tab.Load("k", Of(path), load)
	if err != nil || updated != "two" || loads != 2 {
		t.Fatalf("after stamp: got=%q loads=%d err=%v", updated, loads, err)
	}
}

func TestOf_missingIsZero(t *testing.T) {
	if Of(filepath.Join(t.TempDir(), "missing")) != 0 {
		t.Fatal("missing path must be Stamp(0)")
	}
}

func TestView_reusesUntilBackingChanges(t *testing.T) {
	raw := []byte("id: ACC-1\n")
	var views View[string]
	var parses int
	parse := func(b []byte) (string, error) {
		parses++
		return string(b), nil
	}
	first, err := views.Get("acc", raw, parse)
	if err != nil || first != string(raw) || parses != 1 {
		t.Fatalf("first=%q parses=%d err=%v", first, parses, err)
	}
	again, err := views.Get("acc", raw, parse)
	if err != nil || again != first || parses != 1 {
		t.Fatalf("same backing parses=%d", parses)
	}
	copyRaw := append([]byte(nil), raw...)
	_, err = views.Get("acc", copyRaw, parse)
	if err != nil || parses != 2 {
		t.Fatalf("new backing parses=%d err=%v", parses, err)
	}
}

func TestFingerprints_skipUntilChanged(t *testing.T) {
	var fp Fingerprints
	if fp.Unchanged("p", "a") {
		t.Fatal("empty table is not unchanged")
	}
	fp.Remember("p", "a")
	if !fp.Unchanged("p", "a") {
		t.Fatal("remembered fingerprint")
	}
	if fp.Unchanged("p", "b") {
		t.Fatal("changed fingerprint")
	}
}

func TestFirstExistingAndWalkYAML(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.yaml")
	present := filepath.Join(dir, "hit.yaml")
	if err := fileutil.WriteStandardFile(present, []byte("id: x\n")); err != nil {
		t.Fatal(err)
	}
	if got := FirstExisting([]string{missing, present}); got != present {
		t.Fatalf("FirstExisting=%q", got)
	}
	if Combine(Of(missing), Of(present)) != Of(present) {
		t.Fatal("Combine should keep newest existing stamp")
	}
	var ids []string
	if err := WalkYAML(dir, func(fileID, _ string, _ []byte) {
		ids = append(ids, fileID)
	}); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "hit" {
		t.Fatalf("WalkYAML ids=%v", ids)
	}
}
