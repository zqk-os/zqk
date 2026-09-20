package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestDebugList(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	projectRoot := env.TestRoot
	storagepkg.BuildPathAliasCacheForProject(projectRoot)
	createdAt, _ := time.Parse(time.RFC3339, "2026-03-01T12:00:00Z")
	for i := 1; i <= 2; i++ {
		id := "AUD-DEBUG-" + string(rune('0'+i))
		obj := map[string]any{
			objects.FieldKeyID:        id,
			objects.FieldKeyKind:      "audit_event",
			objects.FieldKeyStatus:    "completed",
			objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339),
		}
		seg, offset, _ := storagepkg.AppendToStream(projectRoot, "audit_event", id, obj, createdAt)
		_ = storagepkg.AppendStreamLocationToRegistry(projectRoot, "audit_event", id, storagepkg.FormatStreamLocation(seg, offset))
	}
	ctx := storagepkg.WithCLIOperation(context.Background())
	filters := map[string]any{objects.FieldKeyStatus: map[string]any{"$nin": []string{"pending"}}}
	listResult, err := env.Storage.(storagepkg.ObjectStorageProvider).List(ctx, pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind:    "audit_event",
		Filters: filters,
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("list err: %v", err)
	}
	t.Logf("List objects: %d", len(listResult.Objects))

	optRes, delErr := env.Storage.(*storagepkg.FileObjectStorage).BulkDeleteOptimized(ctx, env.SecurityContext, []string{"AUD-DEBUG-1", "AUD-DEBUG-2"}, false, 10)
	if delErr != nil {
		t.Fatalf("del err: %v", delErr)
	}
	t.Logf("Del count: %d", optRes.SuccessCount)

	count, _ := env.Storage.(storagepkg.ObjectStorageProvider).Count(ctx, env.SecurityContext, storagepkg.ListFilter{Kind: "audit_event"})
	t.Logf("Count after del: %d", count)
}
