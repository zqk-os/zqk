// BLI-STARTER-COMMUNITY-026 / PRI-STARTER-COMMUNITY-026 coverage elevation
package precommit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPathHelpers(t *testing.T) {
	t.Parallel()
	root := "/tmp/proj"
	if !strings.Contains(CategoryDir(root), paths.PreCommitDir) {
		t.Fatalf("CategoryDir: %s", CategoryDir(root))
	}
	if AggregatedPath(root) != filepath.Join(CategoryDir(root), paths.PreCommitResultsFile) {
		t.Fatalf("AggregatedPath: %s", AggregatedPath(root))
	}
	if !strings.HasSuffix(CategoryPath(root, "lint"), "lint.json") {
		t.Fatalf("CategoryPath: %s", CategoryPath(root, "lint"))
	}
	if !strings.Contains(LastResultPath(root, "policy"), ".last-policy.json") {
		t.Fatalf("LastResultPath: %s", LastResultPath(root, "policy"))
	}
	if !strings.HasSuffix(LintOutputPath(root), "lint-output.txt") {
		t.Fatal(LintOutputPath(root))
	}
	if !strings.HasSuffix(PolicyOutputPath(root), "policy-output.txt") {
		t.Fatal(PolicyOutputPath(root))
	}
	if !strings.HasSuffix(IntegrityOutputPath(root), "integrity-output.txt") {
		t.Fatal(IntegrityOutputPath(root))
	}
}

func TestWriteCategory_RequiresArgs(t *testing.T) {
	t.Parallel()
	if err := WriteCategory("", "lint", CategoryResult{}); err == nil {
		t.Fatal("empty projectRoot")
	}
	if err := WriteCategory(t.TempDir(), "", CategoryResult{}); err == nil {
		t.Fatal("empty category")
	}
}

func TestReadLastResult_RoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	want := freshResult(true, true)
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	dir := CategoryDir(root)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(LastResultPath(root, "lint"), data, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastResult(root, "lint")
	if err != nil {
		t.Fatalf("ReadLastResult: %v", err)
	}
	if got.OK != want.OK || got.Summary != want.Summary {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestReadLastResult_Missing(t *testing.T) {
	t.Parallel()
	if _, err := ReadLastResult(t.TempDir(), "lint"); err == nil {
		t.Fatal("expected missing last result error")
	}
}

func TestAggregate_EmptyDirWritesPass(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got, err := Aggregate(root)
	if err != nil {
		t.Fatalf("Aggregate empty: %v", err)
	}
	if got.Block {
		t.Fatal("empty dir must not block")
	}
	if _, err := ReadAggregated(root); err != nil {
		t.Fatalf("results.json missing: %v", err)
	}
}

func TestAggregate_SkipsStagingAndNonJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := WriteCategory(root, "lint", freshResult(true, true)); err != nil {
		t.Fatal(err)
	}
	dir := CategoryDir(root)
	if err := fileutil.WriteFile(LastResultPath(root, "policy"), []byte(`{"ok":false,"blocking":true}`), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	got, err := Aggregate(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Categories["lint"]; !ok {
		t.Fatalf("lint missing: %+v", got.Categories)
	}
	if _, ok := got.Categories["policy"]; ok {
		t.Fatal("staging last-policy must not merge")
	}
	if got.Block {
		t.Fatal("fresh lint pass must not block")
	}
}

func TestWriteAndReadAggregated_Errors(t *testing.T) {
	t.Parallel()
	if err := WriteAggregated("", &AggregatedResult{}); err == nil {
		t.Fatal("empty root")
	}
	if err := WriteAggregated(t.TempDir(), nil); err == nil {
		t.Fatal("nil aggregate")
	}
	if _, err := ReadAggregated(""); err == nil {
		t.Fatal("empty root read")
	}
	if _, err := ReadAggregated(t.TempDir()); err == nil {
		t.Fatal("missing results.json")
	}
}

func TestReadAggregated_NilCategoriesMap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := CategoryDir(root)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(AggregatedPath(root), []byte(`{"updated_at":"","block":false}`), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAggregated(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Categories == nil {
		t.Fatal("nil categories must be initialized")
	}
}

func TestClear_MissingAndPopulated(t *testing.T) {
	t.Parallel()
	missing := t.TempDir()
	if err := Clear(missing); err != nil {
		t.Fatalf("clear missing dir: %v", err)
	}
	if _, err := ReadAggregated(missing); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := WriteCategory(root, "lint", freshResult(false, true)); err != nil {
		t.Fatal(err)
	}
	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAggregated(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Block {
		t.Fatal("clear must write block=false")
	}
	if _, err := os.Stat(CategoryPath(root, "lint")); !os.IsNotExist(err) {
		t.Fatalf("lint category should be removed, err=%v", err)
	}
}

func TestClear_EmptyProjectRoot(t *testing.T) {
	t.Parallel()
	if err := Clear(""); err == nil {
		t.Fatal("empty root")
	}
	if _, err := Aggregate(""); err == nil {
		t.Fatal("aggregate empty root")
	}
}

func TestBlockingCategoriesSummary_Edges(t *testing.T) {
	t.Parallel()
	if BlockingCategoriesSummary(nil) != nil {
		t.Fatal("nil aggregate")
	}
	a := &AggregatedResult{Categories: map[string]CategoryResult{
		"z-lint": {Blocking: true, OK: false, Summary: "", Stale: false},
		"policy": {Blocking: true, OK: false, Summary: "drift", Stale: true},
		"ok":     {Blocking: true, OK: true, Stale: false},
	}}
	got := BlockingCategoriesSummary(a)
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "policy: drift"+staleSuffix) {
		t.Fatalf("stale summary: %v", got)
	}
	if !strings.Contains(joined, "z-lint: failed") {
		t.Fatalf("empty summary fallback: %v", got)
	}
	if strings.Contains(joined, "ok:") {
		t.Fatalf("fresh pass must be omitted: %v", got)
	}
}

func TestReadCategory_InvalidJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := CategoryDir(root)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(CategoryPath(root, "lint"), []byte("not-json"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCategory(root, "lint"); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestIsStale_FreshWithinWindow(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	r := CategoryResult{UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339)}
	if r.IsStale(now) {
		t.Fatal("one-hour-old verdict must be fresh")
	}
}
