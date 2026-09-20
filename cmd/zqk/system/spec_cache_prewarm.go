package system

import (
	stdcontext "context"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

// prewarmSpecCacheForValidation pre-warms the spec cache by loading all specs in the
// specs directory. This reduces lock contention before many goroutines validate concurrently.
//
// The async system check path uses SpecLoader.EnsureReady(ctx) instead (component loader
// pattern) for standardization, timeouts, and telemetry. Use this function only when
// full-dir prewarm is needed; otherwise prefer EnsureReady.
func prewarmSpecCacheForValidation(checkCtx *AsyncCheckContext) error {
	startTime := time.Now()
	specLoader := objects.GetGlobalSpecLoader()

	// Get specs directory
	specsDir := objects.FindSpecsDir()
	if specsDir == emptyValue {
		// Fallback: pre-warm common specs by name
		return prewarmCommonSpecsByName(specLoader, checkCtx.Logger)
	}

	// Scan directory for all YAML files and load them
	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		// Fallback to common specs
		return prewarmCommonSpecsByName(specLoader, checkCtx.Logger)
	}

	loadedCount := 0
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	for _, entry := range entries {
		// Check context cancellation
		select {
		case <-ctx.Done():
			logging.Fluent(checkCtx.Logger).Debug("Spec cache pre-warming cancelled").
				Int("loaded", loadedCount).
				WithError(ctx.Err()).
				Log()
			return nil
		default:
		}

		if entry.IsDir() {
			continue
		}

		// Only process YAML files
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		// Skip placeholder files
		if strings.HasSuffix(entry.Name(), "_placeholder.yaml") {
			continue
		}

		// Load spec to warm cache (best-effort, errors are OK)
		specFile := entry.Name()
		if _, err := specLoader.LoadSpecWithInheritance(specFile); err == nil {
			loadedCount++
		}
	}

	duration := time.Since(startTime)
	logging.Fluent(checkCtx.Logger).Info("Spec cache pre-warming completed").
		Int("loaded", loadedCount).
		String("duration", duration.String()).
		Log()

	return nil
}

// prewarmCommonSpecsByName pre-warms common specs by name (fallback)
func prewarmCommonSpecsByName(specLoader *objects.SpecLoader, logger logging.Logger) error {
	commonSpecs := []string{
		objects.KindBacklogItem, objects.KindRequirement, objects.KindPolicy, objects.KindAccount,
		objects.KindGoal, objects.KindWorkstream, objects.KindTestCase, objects.KindCriteria, objects.KindDecision,
		objects.KindSchedulerJob, objects.KindAuditEvent, objects.KindChangeJournalEntry,
	}

	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	for _, kind := range commonSpecs {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		specFile := kind + ".yaml"
		if _, err := specLoader.LoadSpecWithInheritance(specFile); err == nil {
			logging.Fluent(logger).Debug("Pre-warmed spec").
				String("spec", specFile).
				Log()
		}
	}

	return nil
}
