package scheduler

import (
	"context"
	"maps"
	"os"
	"strconv"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const (
	defaultCVSPerTestLedgerMaxFailures = 25

	cvsLedgerMetaLastGreenAt      = "_cvs_last_green_at_rfc3339"
	cvsLedgerEntryLastFailedAtRFC = "last_failed_at_rfc3339"
	cvsLedgerEntryLastJobID       = "last_job_id"
)

func cvsPerTestLedgerMaxFailuresFromEnv() int {
	v := strings.TrimSpace(os.Getenv(zqkenv.CVSPerTestLedgerMaxFailures()))
	if v == "" {
		return defaultCVSPerTestLedgerMaxFailures
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultCVSPerTestLedgerMaxFailures
	}
	if n == 0 {
		return 0
	}
	if n < 0 {
		return defaultCVSPerTestLedgerMaxFailures
	}
	return n
}

func stringKeyedAnyMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				return nil
			}
			out[ks] = val
		}
		return out
	default:
		return nil
	}
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	return maps.Clone(in)
}

// maybePersistCVSPerTestFailureLedger merges per-test failure rows into
// convergence_session.after_state_snapshot.per_test_failure_ledger_v1 when the parent job sets
// CONVERGENCE_SESSION_ID. When markFingerprintGreen is true, records a last-green timestamp for the
// fingerprint and removes per-test rows. Appends are skipped when the parsed failure count exceeds the
// ZQK_CVS_LEDGER_MAX_FAILURES threshold (non-zero), treating large failure sets as harness or config issues.
func (h *RunWrapperHandler) maybePersistCVSPerTestFailureLedger(ctx context.Context, job *ScheduledJob, fingerprint string, failedTests []string, markFingerprintGreen bool) {
	if h == nil || job == nil || h.storage == nil {
		return
	}
	sessionID := strings.TrimSpace(envLookup(job, EnvKeyConvergenceSessionID))
	if sessionID == emptyValue || fingerprint == emptyValue {
		return
	}
	if err := validateConvergenceSessionTickTargetID(sessionID); err != nil {
		return
	}

	maxN := cvsPerTestLedgerMaxFailuresFromEnv()
	if !markFingerprintGreen && maxN > 0 && len(failedTests) > maxN {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCVSLedgerSkippedTooManyFailures).
			JobID(job.ID).
			SessionID(sessionID).
			String(KeyBundleCommandFingerprint, fingerprint).
			Int("failed_test_count", len(failedTests)).
			Int("ledger_max_failures", maxN).
			Log()
		return
	}
	if !markFingerprintGreen && len(failedTests) == 0 {
		return
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := h.storage.Read(ctx, secCtx, sessionID)
	if err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCVSLedgerPersistFailed).
			JobID(job.ID).
			SessionID(sessionID).
			String("reason", "read_session").
			WithError(err).
			Log()
		return
	}
	if k, _ := obj[objects.FieldKeyKind].(string); k != objects.KindConvergenceSession {
		return
	}

	afterSnap := stringKeyedAnyMap(obj[objects.FieldKeyAfterStateSnapshot])
	var ledger map[string]any
	if afterSnap != nil {
		ledger = stringKeyedAnyMap(afterSnap[convSugKeyPerTestFailureLedger])
	}
	if ledger == nil {
		ledger = map[string]any{}
	}

	now := zqktime.NowRFC3339UTC()

	if markFingerprintGreen {
		ledger[fingerprint] = map[string]any{cvsLedgerMetaLastGreenAt: now}
	} else {
		fpMap := cloneStringAnyMap(stringKeyedAnyMap(ledger[fingerprint]))
		delete(fpMap, cvsLedgerMetaLastGreenAt)
		for _, name := range failedTests {
			n := strings.TrimSpace(name)
			if n == "" {
				continue
			}
			fpMap[n] = map[string]any{
				cvsLedgerEntryLastFailedAtRFC: now,
				cvsLedgerEntryLastJobID:       job.ID,
			}
		}
		ledger[fingerprint] = fpMap
	}

	patch := map[string]any{
		objects.FieldKeyAfterStateSnapshot: map[string]any{
			convSugKeyPerTestFailureLedger: ledger,
		},
	}

	if err := h.storage.Update(ctx, secCtx, sessionID, patch); err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCVSLedgerPersistFailed).
			JobID(job.ID).
			SessionID(sessionID).
			String("reason", "update_after_state_snapshot").
			WithError(err).
			Log()
		return
	}
}
