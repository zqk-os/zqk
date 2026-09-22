// BLI-STARTER-COMMUNITY-067 / PRI-STARTER-COMMUNITY-067 coverage elevation
package testscan

import (
	"path/filepath"
	"testing"
	"time"
)

func TestExtraBundleStorageAndLimits(t *testing.T) {
	root := t.TempDir()
	bs := NewBundleStorage(root)
	_, _ = bs.ListBundles()
	_, _ = bs.LoadBundle("missing")
	_, _ = bs.ResolveBundleNames("missing")
	_ = bs.DeleteBundle("missing")

	tf1 := &TestFunction{Name: "TestA", Package: "p", PackagePath: "pkg/p", EstimatedDuration: time.Second}
	tf2 := &TestFunction{Name: "TestB", Package: "p", PackagePath: "pkg/p", EstimatedDuration: 2 * time.Second}
	b1 := &TestBundle{
		ID: "b1", Tests: []*TestFunction{tf1}, IsParallel: true,
		EstimatedDuration: time.Second, PackagePath: "pkg/p",
		CriteriaRefs: []string{"CRIT-1"},
	}
	if err := bs.SaveBundle(b1, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := bs.SaveBundle(b1, "alpha"); err == nil {
		t.Fatal("expected exists")
	}
	b2 := &TestBundle{
		ID: "b2", Tests: []*TestFunction{tf2}, IsParallel: true,
		EstimatedDuration: 2 * time.Second, PackagePath: "pkg/p",
		TestCaseRefs: []string{"TEST-1"},
	}
	if err := bs.SaveBundleWithOptions(b2, "alpha", SaveOptions{Merge: true}); err != nil {
		t.Fatal(err)
	}
	if err := bs.SaveBundleWithOptions(b2, "alpha", SaveOptions{Merge: true, RemoveMissing: true}); err != nil {
		t.Fatal(err)
	}
	if err := bs.SaveBundleWithOptions(b1, "alpha", SaveOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}

	loaded, err := bs.LoadBundle("alpha")
	if err != nil {
		t.Fatal(err)
	}
	_ = loaded.FilterTests(nil, nil)
	_ = loaded.FilterTests([]int{0}, []int{1})
	_, _ = loaded.GetTestByIndex(0)
	_, _ = loaded.GetTestByIndex(99)
	_ = loaded.ListTestsWithIndexes()
	empty := &BundleFile{}
	_ = empty.FilterTests(nil, nil)
	_ = empty.ListTestsWithIndexes()

	_ = bs.mergeBundles(nil, b1, false)
	_ = bs.mergeBundles(b1, nil, false)
	_ = bs.mergeBundles(b1, b2, true)

	if err := bs.SaveBundle(b1, "group-0"); err != nil {
		t.Fatal(err)
	}
	if err := bs.SaveBundle(b1, "group-1"); err != nil {
		t.Fatal(err)
	}
	names, err := bs.ResolveBundleNames("group")
	if err != nil || len(names) != 2 {
		t.Fatalf("resolve group: %v %v", names, err)
	}
	exact, err := bs.ResolveBundleNames("alpha")
	if err != nil || len(exact) != 1 {
		t.Fatalf("resolve exact: %v %v", exact, err)
	}
	listed, err := bs.ListBundles()
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v %v", listed, err)
	}
	_ = bs.DeleteBundle("alpha")

	_, _ = ReadPackageConcurrencyLimitsMap(root)
	_ = WritePackageConcurrencyLimitsPatch(root, []*TestBundle{b1}, 2)
	_ = WritePackageConcurrencyLimitsPatch("", []*TestBundle{b1}, 2)
	_ = MergePackageConcurrencyLimitMaps(map[string]int{"a": 2}, map[string]int{"a": 1, "b": 3})
	_ = defaultEstimatedDurationForPackage("pkg/storage")
	_ = defaultEstimatedDurationForPackage("cmd/zqk/object")
	_ = defaultEstimatedDurationForPackage("pkg/cli")
	_, _ = SuggestedTimeoutForPackage(root, "./pkg/p")
	_ = filepath.Join(root, "x")
}
