package scheduler

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type resolvedHandlerOverlay struct {
	byJobType map[string]jobTypeHandlerBuilder
}

var jobTypeHandlerOverlay atomic.Pointer[resolvedHandlerOverlay]

func setJobTypeHandlerOverlay(byJobType map[string]jobTypeHandlerBuilder) {
	if len(byJobType) == 0 {
		jobTypeHandlerOverlay.Store(nil)
		return
	}
	jobTypeHandlerOverlay.Store(&resolvedHandlerOverlay{byJobType: byJobType})
}

func resolveJobTypeHandlerBuilder(jobType string) jobTypeHandlerBuilder {
	if ov := jobTypeHandlerOverlay.Load(); ov != nil {
		if b, ok := ov.byJobType[jobType]; ok && b != nil {
			return b
		}
	}
	if spec, ok := jobTypeHandlerRegistry[jobType]; ok {
		return spec.build
	}
	return nil
}

// prewarmSchedulerHandlerBindingOverlay loads optional scheduler_handler_binding objects and builds
// a resolved overlay map job_type -> handler builder. Unknown kinds/keys are ignored with warnings.
// Fallback remains static defaults from jobTypeHandlerRegistry.
func prewarmSchedulerHandlerBindingOverlay(ctx context.Context, storage storagepkg.ObjectStorageProvider, logger logging.Logger) {
	if storage == nil {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindSchedulerHandlerBinding,
	})
	if err != nil {
		// Kind may not exist yet; keep defaults quietly at debug level.
		if logger != nil {
			CachePrewarmLog(logger).Debug(LogEventCachePrewarmHandlerBindingOverlayListSkipped).
				WithError(err).
				Log()
		}
		return
	}
	if res == nil || len(res.Objects) == 0 {
		setJobTypeHandlerOverlay(nil)
		return
	}

	overlay := buildOverlayFromBindingObjects(res.Objects, handlerKeyRegistry(), logger)
	setJobTypeHandlerOverlay(overlay)
	if logger != nil && len(overlay) > 0 {
		CachePrewarmLog(logger).Info(LogEventCachePrewarmHandlerBindingOverlayLoaded).
			Int("bindings", len(overlay)).
			Log()
	}
}

func buildOverlayFromBindingObjects(bindingObjs []map[string]any, keys map[string]jobTypeHandlerBuilder, logger logging.Logger) map[string]jobTypeHandlerBuilder {
	sort.Slice(bindingObjs, func(i, j int) bool {
		pi := fieldInt(bindingObjs[i][objects.FieldKeyPriority], 1000)
		pj := fieldInt(bindingObjs[j][objects.FieldKeyPriority], 1000)
		if pi != pj {
			return pi < pj
		}
		idi := fieldString(bindingObjs[i][objects.FieldKeyID])
		idj := fieldString(bindingObjs[j][objects.FieldKeyID])
		return idi < idj
	})
	overlay := make(map[string]jobTypeHandlerBuilder)
	for _, obj := range bindingObjs {
		if !fieldBoolDefault(obj[objects.FieldKeyEnabled], true) {
			continue
		}
		// Fail closed: only active lifecycle bindings remaps dispatch. Proposed/draft/test
		// fixtures (status=proposed) previously remapped all cache_prewarm → cache_invalidation
		// and spammed desktop "no event data provided" failures.
		// TRACK: CreateCASVisible must not leak fixtures into live kernel.
		status := strings.TrimSpace(fieldString(obj[objects.FieldKeyStatus]))
		if status != objects.ObjectStatusActive && status != objects.ObjectStatusApproved && status != objects.ObjectStatusImplemented {
			continue
		}
		jobType := strings.TrimSpace(fieldString(obj[objects.FieldKeyJobType]))
		handlerKey := strings.TrimSpace(fieldString(obj[objects.FieldKeyHandlerKey]))
		if jobType == emptyValue || handlerKey == emptyValue {
			continue
		}
		if _, ok := jobTypeHandlerRegistry[jobType]; !ok {
			if logger != nil {
				CachePrewarmLog(logger).Warn(LogEventCachePrewarmHandlerBindingUnknownJobType).
					String("job_type", jobType).
					Log()
			}
			continue
		}
		builder, ok := keys[handlerKey]
		if !ok || builder == nil {
			if logger != nil {
				CachePrewarmLog(logger).Warn(LogEventCachePrewarmHandlerBindingUnknownHandlerKey).
					String("job_type", jobType).
					String("handler_key", handlerKey).
					Log()
			}
			continue
		}
		if _, exists := overlay[jobType]; exists {
			continue
		}
		overlay[jobType] = builder
	}
	return overlay
}

func fieldString(v any) string {
	s, _ := v.(string)
	return s
}

func fieldBoolDefault(v any, d bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return d
}

func fieldInt(v any, d int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return d
	}
}
