package object

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// WithCLIOperation marks a context as a CLI operation (re-export from storage)
var WithCLIOperation = storage.WithCLIOperation

var testIDCounter int64

// setupCLITestEnvironment is now an alias for SetupTestEnvironment for backward compatibility.
// It returns (tmpDir, cliBinary) to match the old signature.
func setupCLITestEnvironment(t *testing.T) (tmpDir, cliBinary string) {
	testEnv := SetupTestEnvironment(t)
	return testEnv.GetTestRoot(), testEnv.CLIBinary
}

func readTestObjectWithRetry(storageProvider *storage.FileObjectStorage, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	var (
		obj map[string]any
		err error
	)
	for attempt := 0; attempt < 10; attempt++ {
		obj, err = storageProvider.Read(pkgctx.NewSystemContext(), secCtx, id)
		if err == nil {
			return obj, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, err
}

// generateTestID generates a simple test ID based on kind.
func generateTestID(kind string) string {
	prefix := getPrefixForKind(kind)
	c := atomic.AddInt64(&testIDCounter, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().Unix(), c)
}

func getPrefixForKind(kind string) string {
	prefixes := map[string]string{
		pplanKindBacklogItem: "BLI",
		"goal":               "GOAL",
		"criteria":           "CRIT",
		"requirement":        "REQ",
	}
	prefix := prefixes[kind]
	if prefix == emptyValue {
		prefix = "TEST"
	}
	return prefix
}

func getInitialStatus(kind string) string {
	// FINDING: this map is snowflake logic for a question the lifecycle already answers, and two of
	// its four rows named statuses the kind does not have -- criteria has no not_started (its origin
	// is awaiting_verification) and requirement has no planned. Both produced "lifecycle status
	// unknown" at create, so every dynamic-CLI test over those kinds died at fixture setup.
	// getInitialStatusForKind in this same package already derives this from the lifecycle YAML by
	// selecting the status marked initial; the rows below are corrected in place rather than
	// delegated because that helper lives in a _test.go file this one cannot import from.
	// TRACK: BLI-REDACTED — collapse onto the lifecycle-derived helper.
	statuses := map[string]string{
		pplanKindBacklogItem: objectStatusExploring,
		"goal":               objectStatusActive,
		"criteria":           objects.ObjectStatusAwaitingVerification,
		"requirement":        objects.ObjectStatusProposed,
	}
	status := statuses[kind]
	if status == emptyValue {
		status = objectStatusActive
	}
	return status
}

// casLeaveStatus picks a CAS-visible leave status for CreateCASVisible.
// Draft-first create parks preliminary statuses on the draft plane; List/Count omit them.
// Empty string lets CreateCASVisible use defaultLeavePreliminaryStatus (kind-aware).
// TRACK: BLI-REDACTED — draft-plane create / promote membrane.
func casLeaveStatus(status string) string {
	switch status {
	// FINDING: this list is a hardcoded stand-in for "is this status preliminary?", a question
	// StatusChecker.IsPreliminary answers per kind -- but it cannot be asked here because this
	// function only receives a status. Anything not listed is returned unchanged, which makes the
	// leave status equal the create status, which makes the promote a no-op that silently leaves the
	// object in the draft plane where List cannot see it. awaiting_verification is listed because
	// criteria's origin status moved onto it above; a kind whose origin status is missing from this
	// list fails that way with no error.
	// TRACK: BLI-REDACTED — take a kind and use StatusChecker.IsPreliminary.
	case objectStatusExploring, objectStatusNotStarted, objects.ObjectStatusAwaitingVerification, emptyValue:
		return ""
	default:
		return status
	}
}
