package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestListStreamSegmentsIgnoresGhostsAfterDeletedTruncated pins the retention compact
// contract: stream_deleted is truncated while segment files still contain deleted lines.
// List/count must intersect with the live registry or total_count inflates past max_count.
// TRACK: BLI-1785905541906569000-074e24d7
func TestListStreamSegmentsIgnoresGhostsAfterDeletedTruncated(t *testing.T) {
	root := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, root)
	BuildPathAliasCacheForProject(root)

	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	kind := objects.KindAgentInstruction
	if !StreamStorageEnabledForKind(kind) {
		t.Skip("agent_instruction not stream-backed in this layout")
	}

	segDir := filepath.Join(root, paths.ProjectDataDir, paths.StreamsDir, kind)
	if err := fileutil.EnsureDir(segDir); err != nil {
		t.Fatalf("mkdir segments: %v", err)
	}

	liveID := "AGI-1786000000000000001-live0001"
	ghostID := "AGI-1785000000000000001-ghost001"
	createdAt := time.Unix(1_700_000_000, 0).UTC()

	segPath := filepath.Join(segDir, createdAt.Format("2006-01-02")+"_stream.json")
	var segBody []byte
	for _, obj := range []map[string]any{
		{
			objects.FieldKeyID:          ghostID,
			objects.FieldKeyKind:        kind,
			objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			objects.FieldKeyCreatedAt:   createdAt.Unix(),
			objects.FieldKeyNamespaceID: validationDefaultNamespaceKernel(),
		},
		{
			objects.FieldKeyID:          liveID,
			objects.FieldKeyKind:        kind,
			objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			objects.FieldKeyCreatedAt:   createdAt.Unix(),
			objects.FieldKeyNamespaceID: validationDefaultNamespaceKernel(),
		},
	} {
		b, _ := json.Marshal(obj)
		segBody = append(segBody, b...)
		segBody = append(segBody, '\n')
	}
	if err := fileutil.WriteSecureFile(segPath, segBody); err != nil {
		t.Fatalf("write segment: %v", err)
	}

	// Only live ID remains in registry (simulates compact after soft-delete of ghost).
	if err := AppendStreamLocationToRegistry(root, kind, liveID, FormatStreamLocation(segPath, 1)); err != nil {
		t.Fatalf("registry append: %v", err)
	}
	// Truncated deleted file (compact behavior) — ghost line still in segment.
	delPath := filepath.Join(stateDir, "stream_deleted_"+kind+".jsonl")
	if err := fileutil.WriteSecureFile(delPath, nil); err != nil {
		t.Fatalf("truncate deleted: %v", err)
	}

	live := LiveStreamIDSet(root, kind)
	if !live[liveID] || live[ghostID] {
		t.Fatalf("LiveStreamIDSet=%v want only %s", live, liveID)
	}

	store, err := NewFileObjectStorage(root)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = store.Shutdown(ctx)
	})

	sec := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}
	sctx := pkgctx.NewStorageContext()
	result, err := store.List(context.Background(), sec, sctx, ListFilter{Kind: kind, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	total, _ := result.Meta["total_count"].(int)
	if total != 1 {
		t.Fatalf("list total_count=%d want 1 (ghost segment line must be ignored)", total)
	}
	if len(result.Objects) != 1 {
		t.Fatalf("list objects=%d want 1", len(result.Objects))
	}
	if got, _ := result.Objects[0][objects.FieldKeyID].(string); got != liveID {
		t.Fatalf("list id=%q want %s", got, liveID)
	}

	n, err := store.Count(context.Background(), sec, ListFilter{
		Kind:    kind,
		Filters: map[string]any{objects.FieldKeyNamespaceID: validationDefaultNamespaceKernel()},
	})
	if err != nil {
		t.Fatalf("Count with filter: %v", err)
	}
	if n != 1 {
		t.Fatalf("filtered Count=%d want 1", n)
	}
}

func validationDefaultNamespaceKernel() string {
	return "zqk:kernel"
}
