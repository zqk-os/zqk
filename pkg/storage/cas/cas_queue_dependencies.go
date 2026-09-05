package cas

import (
	"context"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

type WaitGroupManager interface {
	CreateGroupForGoroutine(id, operation string) *sync.WaitGroup
	Add(id string, delta int)
	Done(id string)
	Wait(id string)
}

type ValidationMetrics interface {
	RecordValidation(durationNs int64, entriesScanned, staleRemoved int)
}

type ValidationStrategy interface {
	ValidateMappings(kindDir string, mappings map[string]string, bucketKeys map[string]string) (validMappings map[string]string, validBucketKeys map[string]string, staleCount int)
}

type ValidationStrategyRegistry interface {
	GetStrategy(kind string) ValidationStrategy
}

type QueueShutdownHandler interface {
	InitiateShutdown() error
	Drain(ctx context.Context) error
	IsDrained() bool
	GetPendingCount() int64
	GetName() string
	IsCritical() bool
}

type ShutdownCoordinator interface {
	RegisterQueue(h QueueShutdownHandler)
	IsShutdownInitiated() bool
}

var (
	GlobalNewWaitGroupManager  func() WaitGroupManager
	GlobalShutdownCoordinator  ShutdownCoordinator
	GlobalValidationRegistry   ValidationStrategyRegistry
	GlobalGetValidationMetrics func(kind string) ValidationMetrics
)

type RetryConfig struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// AuditEventCreator defines the callback for creating audit events from the CAS subpackage
type AuditEventCreator func(ctx context.Context, secCtx *pkgctx.SecurityContext, projectRoot string, storage CASFacade, batchSize, successCount, failureCount int, durationMs int64, failedFiles []string)

var globalAuditCreator AuditEventCreator

// SetAuditCreator injects the audit creation implementation from the root storage package
func SetAuditCreator(creator AuditEventCreator) {
	globalAuditCreator = creator
}

// GetAuditCreator returns the injected audit creator
func GetAuditCreator() AuditEventCreator {
	return globalAuditCreator
}
