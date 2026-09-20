package transceiver

import (
	"strings"
	"testing"
)

func TestSchedulerTransceiverLogEvents_wirePrefix(t *testing.T) {
	t.Parallel()
	want := schedulerTransceiverWirePrefix + "_"
	for _, evt := range []string{
		LogEventSchedulerTransceiverVerificationRegistered,
		LogEventSchedulerTransceiverVerificationDeliveryVerified,
		LogEventSchedulerTransceiverVerificationDeliveryFailed,
		LogEventSchedulerTransceiverVerificationComplete,
		LogEventSchedulerTransceiverVerificationExpired,
		LogEventSchedulerTransceiverRuleConvertConfigFailed,
		LogEventSchedulerTransceiverRuleLoaderNoDir,
		LogEventSchedulerTransceiverRuleLoaderDirMissing,
		LogEventSchedulerTransceiverRuleLoadFileFailed,
		LogEventSchedulerTransceiverRuleLoadedFromFile,
		LogEventSchedulerTransceiverRouterRegisteredAdapter,
		LogEventSchedulerTransceiverRouterLoadedRules,
		LogEventSchedulerTransceiverRouterRuleMatched,
		LogEventSchedulerTransceiverRouterNoRulesMatched,
		LogEventSchedulerTransceiverRouterActionFailed,
		LogEventSchedulerTransceiverRouterActionSucceeded,
		LogEventSchedulerTransceiverRouterUnknownOperator,
		LogEventSchedulerTransceiverRouterRetryingAction,
		LogEventSchedulerTransceiverProfileLoaded,
		LogEventSchedulerTransceiverProfileCreatedAsyncRouter,
		LogEventSchedulerTransceiverProfileApplied,
		LogEventSchedulerTransceiverAsyncRouterStarting,
		LogEventSchedulerTransceiverAsyncRouterStopping,
		LogEventSchedulerTransceiverAsyncRouterAllWorkersStopped,
		LogEventSchedulerTransceiverAsyncRouterWorkerStopTimeout,
		LogEventSchedulerTransceiverAsyncRouterStopped,
		LogEventSchedulerTransceiverAsyncRouterMessageQueued,
		LogEventSchedulerTransceiverAsyncRouterQueueFailed,
		LogEventSchedulerTransceiverAsyncRouterRoutingFailed,
		LogEventSchedulerTransceiverAsyncRouterRoutingCompleted,
		LogEventSchedulerTransceiverAsyncRouterWorkerIdleShutdown,
	} {
		if !strings.HasPrefix(evt, want) {
			t.Fatalf("log event %q must start with wire prefix %q", evt, want)
		}
	}
}
