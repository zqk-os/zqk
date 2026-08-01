package object

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestIsNonProgressLifecycleProbeCandidate_AllowsSuccessTerminals(t *testing.T) {
	t.Parallel()

	successMeta := objects.Status{Terminal: true}
	for _, status := range []string{
		objects.ObjectStatusComplete,
		objects.ObjectStatusCompleted,
		objects.ObjectStatusImplemented,
		objects.ObjectStatusSuccess,
	} {
		if isNonProgressLifecycleProbeCandidate(status, successMeta) {
			t.Fatalf("expected success terminal %q to remain probeable for promote", status)
		}
	}

	if !isNonProgressLifecycleProbeCandidate(objects.ObjectStatusRejected, objects.Status{Terminal: true}) {
		t.Fatal("expected rejected terminal to be skipped")
	}
	if !isNonProgressLifecycleProbeCandidate("archived", objects.Status{Archive: true, Terminal: true}) {
		t.Fatal("expected archive terminal to be skipped")
	}
	if !isNonProgressLifecycleProbeCandidate(objects.ObjectStatusError, objects.Status{System: true}) {
		t.Fatal("expected system error status to be skipped")
	}
}
