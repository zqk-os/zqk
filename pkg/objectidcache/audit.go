package objectidcache

import (
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// CacheAuditFunc records cache mutation audit events. Optional; nil is a no-op.
type CacheAuditFunc func(eventType, targetID, targetKind, targetPath, operation, severity, profile string)

type CacheSaveObserver func(projectRoot string, entryCount int, saveDuration time.Duration)
type CacheBuildObserver func(projectRoot, operation string, entryCount int, forceRebuild bool, buildDuration time.Duration)
type CacheHitObserver func(id string, logger logging.Logger)
type CacheMissObserver func(id string, cacheSize int, logger logging.Logger)

var (
	cacheAudit   CacheAuditFunc
	onCacheSave  CacheSaveObserver
	onCacheBuild CacheBuildObserver
	onCacheHit   CacheHitObserver
	onCacheMiss  CacheMissObserver
)

// SetCacheAuditFunc installs the CLI audit emitter (createCacheAuditEvent).
func SetCacheAuditFunc(fn CacheAuditFunc) {
	cacheAudit = fn
}

// SetCacheLifecycleObservers installs optional CLI coordinator + strategy callbacks.
func SetCacheLifecycleObservers(save CacheSaveObserver, build CacheBuildObserver, hit CacheHitObserver, miss CacheMissObserver) {
	onCacheSave = save
	onCacheBuild = build
	onCacheHit = hit
	onCacheMiss = miss
}

func emitCacheAudit(eventType, targetID, targetKind, targetPath, operation, severity, profile string) {
	if cacheAudit != nil {
		cacheAudit(eventType, targetID, targetKind, targetPath, operation, severity, profile)
	}
}
