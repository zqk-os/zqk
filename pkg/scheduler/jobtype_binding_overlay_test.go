package scheduler

import (
	"context"
	"reflect"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

func TestBuildOverlayFromBindingObjects_DeterministicPriority(t *testing.T) {
	t.Parallel()
	keys := handlerKeyRegistry()
	objs := []map[string]any{
		{
			objects.FieldKeyID:         "B-2",
			objects.FieldKeyJobType:    JobTypeCachePrewarm,
			objects.FieldKeyHandlerKey: "cache_invalidation",
			objects.FieldKeyPriority:   20,
			objects.FieldKeyEnabled:    true,
		},
		{
			objects.FieldKeyID:         "B-1",
			objects.FieldKeyJobType:    JobTypeCachePrewarm,
			objects.FieldKeyHandlerKey: "cache_prewarm",
			objects.FieldKeyPriority:   10,
			objects.FieldKeyEnabled:    true,
		},
	}
	overlay := buildOverlayFromBindingObjects(objs, keys, nil)
	b := overlay[JobTypeCachePrewarm]
	if b == nil {
		t.Fatalf("expected overlay binding for %s", JobTypeCachePrewarm)
	}
	// Priority 10 entry should win and map to cache_prewarm constructor.
	if got := resolveBuilderIdentity(b); got != JobTypeCachePrewarm {
		t.Fatalf("winner identity: got %q", got)
	}
}

func TestBuildOverlayFromBindingObjects_IgnoresUnknownsAndDisabled(t *testing.T) {
	t.Parallel()
	keys := handlerKeyRegistry()
	objs := []map[string]any{
		{objects.FieldKeyID: "x1", objects.FieldKeyJobType: "unknown_job_type", objects.FieldKeyHandlerKey: "cache_prewarm", objects.FieldKeyEnabled: true},
		{objects.FieldKeyID: "x2", objects.FieldKeyJobType: JobTypeCachePrewarm, objects.FieldKeyHandlerKey: "unknown_handler", objects.FieldKeyEnabled: true},
		{objects.FieldKeyID: "x3", objects.FieldKeyJobType: JobTypeCachePrewarm, objects.FieldKeyHandlerKey: "cache_prewarm", objects.FieldKeyEnabled: false},
	}
	overlay := buildOverlayFromBindingObjects(objs, keys, nil)
	if len(overlay) != 0 {
		t.Fatalf("expected empty overlay, got %d", len(overlay))
	}
}

// resolveBuilderIdentity identifies a builder by matching function pointer equivalence
// through known keys in handlerKeyRegistry.
func resolveBuilderIdentity(b jobTypeHandlerBuilder) string {
	for k, v := range handlerKeyRegistry() {
		if funcEqual(b, v) {
			return k
		}
	}
	return ""
}

func funcEqual(a, b jobTypeHandlerBuilder) bool {
	if a == nil || b == nil {
		return false
	}
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func TestPrewarmSchedulerHandlerBindingOverlay_UsesStoredBinding(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	setJobTypeHandlerOverlay(nil)
	t.Cleanup(func() { setJobTypeHandlerOverlay(nil) })

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	binding := map[string]any{
		objects.FieldKeyID:            "SHB-1774509000000000000-abcd1234",
		objects.FieldKeyKind:          objects.KindSchedulerHandlerBinding,
		objects.FieldKeyTitle:         "override cache prewarm to cache invalidation",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyJobType:       JobTypeCachePrewarm,
		objects.FieldKeyHandlerKey:    "cache_invalidation",
		objects.FieldKeyPriority:      1,
		objects.FieldKeyEnabled:       true,
	}
	if err := storage.Create(ctx, sec, binding); err != nil {
		t.Fatalf("create binding: %v", err)
	}

	prewarmSchedulerHandlerBindingOverlay(ctx, storage, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	factory := NewHandlerFactory(
		storage,
		env.SpecLoader,
		env.LifecycleLoader,
		env.TestRoot,
		logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		NewNotificationContext(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil),
		nil,
		NewDefaultSchedulerMetricsCollector(),
		nil,
	)
	h := factory.CreateHandler(&ScheduledJob{ID: "J1", JobType: JobTypeCachePrewarm})
	if _, ok := h.(*CacheInvalidationHandler); !ok {
		t.Fatalf("expected CacheInvalidationHandler from overlay, got %T", h)
	}
}
