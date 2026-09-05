package adapters

import (
	"strings"
	"testing"
)

func TestSchedulerTransceiverAdapterLogEvents_matchParentWireKeys(t *testing.T) {
	t.Parallel()
	want := "scheduler_transceiver_"
	for _, evt := range []string{
		LogEventSchedulerTransceiverHTTPWebhookSucceeded,
		LogEventSchedulerTransceiverHTTPUnknownAuth,
		LogEventSchedulerTransceiverEventEmittedStub,
		LogEventSchedulerTransceiverCommandExecuted,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("adapter log event %q must use scheduler_transceiver_* wire key", evt)
		}
	}
}
