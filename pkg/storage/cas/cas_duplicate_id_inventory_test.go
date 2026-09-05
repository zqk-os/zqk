package cas_test

import (
	"context"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestInventoryCASDuplicateIDs_FindsDualBlob(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	kindDir := filepath.Join(root, "docs", "process", "backlog")
	if err := fileutil.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(kindDir, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.yaml")
	newer := filepath.Join(kindDir, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.yaml")
	body := "id: BLI-DUAL-TEST-001\nkind: backlog_item\n"
	if err := fileutil.WriteFile(older, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := fileutil.WriteFile(newer, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	inv := caspkg.InventoryCASDuplicateIDs(context.Background(), root)
	if inv.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount=%d want 1; hits=%v", inv.DuplicateCount, inv.Hits)
	}
	if inv.Hits[0].ObjectID != "BLI-DUAL-TEST-001" {
		t.Fatalf("object_id=%q", inv.Hits[0].ObjectID)
	}
	if inv.Hits[0].KeeperPath != newer {
		t.Fatalf("keeper=%q want newer %q", inv.Hits[0].KeeperPath, newer)
	}
}

func TestQuarantineCASDuplicateLosers_MovesOlder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	kindDir := filepath.Join(root, "docs", "process", "goals")
	qDir := filepath.Join(root, ".zqk", "system-health", "quarantine", "hash-duplicates")
	if err := fileutil.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(kindDir, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.yaml")
	newer := filepath.Join(kindDir, "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd.yaml")
	body := "id: GOAL-DUAL-TEST-001\nkind: goal\n"
	if err := fileutil.WriteFile(older, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := fileutil.WriteFile(newer, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	n, errs := caspkg.QuarantineCASDuplicateLosers(context.Background(), "goal", kindDir, qDir, false)
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if n != 1 {
		t.Fatalf("quarantined=%d want 1", n)
	}
	if _, err := fileutil.Stat(older); !fileutil.IsNotExist(err) {
		t.Fatalf("older still present: %v", err)
	}
	if _, err := fileutil.Stat(newer); err != nil {
		t.Fatalf("keeper missing: %v", err)
	}
	qPath := filepath.Join(qDir, "goal", filepath.Base(older))
	if _, err := fileutil.Stat(qPath); err != nil {
		t.Fatalf("quarantined file missing: %v", err)
	}

	inv := caspkg.InventoryCASDuplicateIDs(context.Background(), root)
	if inv.DuplicateCount != 0 {
		t.Fatalf("after quarantine DuplicateCount=%d", inv.DuplicateCount)
	}
}
