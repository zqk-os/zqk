package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCasIndexSparseVersusDisk(t *testing.T) {
	t.Parallel()
	cases := []struct {
		index, disk int
		want        bool
	}{
		{0, 0, false},
		{0, 1, true},
		{20, 87, true},
		{40, 45, false},
		{50, 50, false},
		{10, 25, true},
	}
	for _, tc := range cases {
		got := casIndexSparseVersusDisk(tc.index, tc.disk)
		if got != tc.want {
			t.Fatalf("index=%d disk=%d got %v want %v", tc.index, tc.disk, got, tc.want)
		}
	}
}

func TestExtractObjectIDFromYAMLHead_IgnoresNestedID(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "obj.yaml")
	content := "metadata:\n  id: nested-should-ignore\nid: CRIT-REAL-001\nkind: criteria\n"
	if err := fileutil.WriteStandardFile(path, []byte(content)); err != nil {
		t.Fatal(err)
	}
	id, err := extractObjectIDFromYAMLHead(path, idHeadBytes)
	if err != nil {
		t.Fatal(err)
	}
	if id != "CRIT-REAL-001" {
		t.Fatalf("got %q want CRIT-REAL-001 (nested id must not win)", id)
	}
}

func TestDetectOrphanedFiles_StaleIndexHashIsDriftNotBlockingOrphan(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	kind := objects.KindBacklogItem
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		t.Fatal("backlog_item has no directory")
	}
	kindDir := filepath.Join(tmp, paths.ProcessDir, dirName)
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}

	oldHash := strings.Repeat("cd", 32)
	newHash := strings.Repeat("ef", 32)
	oid := "BLI-DRIFT-001"
	body := "id: " + oid + "\nkind: backlog_item\nstatus: complete\ntitle: drift\n"
	newPath := filepath.Join(kindDir, newHash+".yaml")
	if err := fileutil.WriteStandardFile(newPath, []byte(body)); err != nil {
		t.Fatal(err)
	}
	// Index still points at deleted old hash (classic post-update lag).
	cas := storage.NewContentAddressableStorage(kindDir, kind)
	if err := cas.GetIndex().SetMapping(oid, oldHash); err != nil {
		t.Fatal(err)
	}

	results := detectOrphanedFiles(nil, tmp, nil, false)
	var driftTier []int
	for _, r := range results {
		if r.ObjectID != oid && r.ObjectKind != kind {
			continue
		}
		for _, iss := range r.Issues {
			if strings.Contains(iss.Message, "CAS index out of sync") {
				driftTier = append(driftTier, iss.Tier)
			}
			if strings.Contains(iss.Message, "Orphaned file on disk") {
				t.Fatalf("stale-index newer hash must not be Tier-1 orphan; got %+v", iss)
			}
		}
	}
	if len(driftTier) == 0 {
		t.Fatalf("expected CAS index out of sync issue; results=%+v", results)
	}
	for _, tier := range driftTier {
		if tier != 4 {
			t.Fatalf("expected Tier 4 for index drift, got %d", tier)
		}
	}

	// Autofix should rewrite mapping to the on-disk hash.
	_ = detectOrphanedFiles(nil, tmp, nil, true)
	cas2 := storage.NewContentAddressableStorage(kindDir, kind)
	if got := cas2.GetIndex().Mappings[oid]; got != newHash {
		t.Fatalf("after autofix want index %s got %s", newHash, got)
	}
	if !fileutil.Exists(newPath) {
		t.Fatal("disk file must survive autofix")
	}
}

func TestDetectOrphanedFiles_EmptyIndexReindexesSoleCriteria(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	kind := objects.KindCriteria
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		t.Fatal("criteria has no directory")
	}
	kindDir := filepath.Join(tmp, paths.ProcessDir, dirName)
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}

	hash := strings.Repeat("ab", 32) // 64 hex chars
	fileName := hash + ".yaml"
	filePath := filepath.Join(kindDir, fileName)
	body := "id: CRIT-SOLE-001\nkind: criteria\nstatus: validated\ntitle: sole survivor\n"
	if err := fileutil.WriteStandardFile(filePath, []byte(body)); err != nil {
		t.Fatal(err)
	}

	// Empty CAS index (no mappings) — historically treated this file as orphan and deleted it.
	cas := storage.NewContentAddressableStorage(kindDir, kind)
	if cas == nil || cas.GetIndex() == nil {
		t.Fatal("expected CAS index")
	}
	if len(cas.GetIndex().Mappings) != 0 {
		t.Fatalf("expected empty index, got %d", len(cas.GetIndex().Mappings))
	}

	results := detectOrphanedFiles(nil, tmp, nil, true)
	if !fileutil.Exists(filePath) {
		t.Fatal("sole CAS criteria file was deleted under empty index autofix")
	}

	reindexed := false
	for _, r := range results {
		for _, af := range r.AutoFixed {
			if strings.Contains(af, "reindexed orphaned CAS file") && strings.Contains(af, "CRIT-SOLE-001") {
				reindexed = true
			}
		}
	}
	if !reindexed {
		t.Fatalf("expected reindex autofix for sole criteria; results=%+v", results)
	}
	if got := cas.GetIndex().Mappings["CRIT-SOLE-001"]; got != hash {
		// Reload index from disk in case SetMapping persisted separately
		cas2 := storage.NewContentAddressableStorage(kindDir, kind)
		if cas2.GetIndex().Mappings["CRIT-SOLE-001"] != hash {
			t.Fatalf("expected index mapping CRIT-SOLE-001 -> %s, got %#v", hash, cas2.GetIndex().Mappings)
		}
	}
}

func TestCleanupHashDuplicates_RefusesMisgroupedNestedID(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	processDir := filepath.Join(tmp, paths.ProcessDir, "criteria")
	if err := fileutil.EnsureDir(processDir); err != nil {
		t.Fatal(err)
	}
	hashA := strings.Repeat("11", 32)
	hashB := strings.Repeat("22", 32)
	pathA := filepath.Join(processDir, hashA+".yaml")
	pathB := filepath.Join(processDir, hashB+".yaml")
	// Nested id first would previously false-group both under "SHARED" with indented regex.
	bodyA := "metadata:\n  id: SHARED\nid: CRIT-A\nkind: criteria\n"
	bodyB := "metadata:\n  id: SHARED\nid: CRIT-B\nkind: criteria\n"
	if err := fileutil.WriteStandardFile(pathA, []byte(bodyA)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(pathB, []byte(bodyB)); err != nil {
		t.Fatal(err)
	}

	logger := logging.GetLoggerFromProfile("test")
	primary, dups := scanHashFiles(processDir, false, logger)
	if len(dups) != 0 {
		t.Fatalf("top-level id extract must not group CRIT-A/CRIT-B as duplicates; primary=%v dups=%v", primary, dups)
	}
	if primary["CRIT-A"] == "" || primary["CRIT-B"] == "" {
		t.Fatalf("expected both criteria as primary keys; got %v", primary)
	}
}

func TestDetectOrphanedFiles_NeverDeletesTraditionalFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	kind := objects.KindCriteria
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		t.Fatal("criteria has no directory")
	}
	kindDir := filepath.Join(tmp, paths.ProcessDir, dirName)
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kindDir, "CRIT-TRAD-001.yaml")
	body := "id: CRIT-TRAD-001\nkind: criteria\nstatus: validated\ntitle: traditional\n"
	if err := fileutil.WriteStandardFile(path, []byte(body)); err != nil {
		t.Fatal(err)
	}
	_ = detectOrphanedFiles(nil, tmp, nil, true)
	if !fileutil.Exists(path) {
		t.Fatal("traditional process file must survive orphan auto-fix")
	}
}
