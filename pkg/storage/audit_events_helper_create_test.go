package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
)

func TestIsFailClosedAuditEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		options  *AuditEventOptions
		expected bool
	}{
		{
			name:     "nil options",
			options:  nil,
			expected: false,
		},
		{
			name: "low severity cache invalidation",
			options: &AuditEventOptions{
				EventType:  audit.EventTypeCacheInvalidation,
				Severity:   audit.SeverityLow,
				TargetKind: objects.KindBacklogItem,
			},
			expected: false,
		},
		{
			name: "medium severity cache bulk invalidation",
			options: &AuditEventOptions{
				EventType:  audit.EventTypeCacheBulkInvalidation,
				Severity:   audit.SeverityMedium,
				TargetKind: objects.KindBacklogItem,
			},
			expected: true,
		},
		{
			name: "high severity alert",
			options: &AuditEventOptions{
				EventType:  "security_alert",
				Severity:   audit.SeverityHigh,
				TargetKind: "security",
			},
			expected: true,
		},
		{
			name: "critical severity event",
			options: &AuditEventOptions{
				EventType:  "system_failure",
				Severity:   "critical",
				TargetKind: "system",
			},
			expected: true,
		},
		{
			name: "scheduler job started with target_id",
			options: &AuditEventOptions{
				EventType:  audit.EventTypeSchedulerJobStarted,
				Severity:   audit.SeverityLow,
				TargetKind: objects.KindSchedulerJob,
				TargetID:   "SCH-100",
			},
			expected: true,
		},
		{
			name: "scheduler job completed with target_id",
			options: &AuditEventOptions{
				EventType:  audit.EventTypeSchedulerJobCompleted,
				Severity:   audit.SeverityLow,
				TargetKind: objects.KindSchedulerJob,
				TargetID:   "SCH-100",
			},
			expected: true,
		},
		{
			name: "scheduler job prefix with target_id",
			options: &AuditEventOptions{
				EventType:  "scheduler_job_failed",
				Severity:   audit.SeverityLow,
				TargetKind: objects.KindSchedulerJob,
				TargetID:   "SCH-100",
			},
			expected: true,
		},
		{
			name: "non-scheduler event with target_id low severity",
			options: &AuditEventOptions{
				EventType:  audit.EventTypeCacheInvalidation,
				Severity:   audit.SeverityLow,
				TargetKind: objects.KindBacklogItem,
				TargetID:   "BLI-1",
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual := isFailClosedAuditEvent(tc.options)
			if actual != tc.expected {
				t.Errorf("isFailClosedAuditEvent() = %v, want %v", actual, tc.expected)
			}
		})
	}
}

func TestCreateAuditEventWithBuilder_FailClosed_EmptyProjectRoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Non-fail-closed event: should return nil (best-effort)
	lowEvent := &AuditEventOptions{
		EventType:  audit.EventTypeCacheInvalidation,
		Severity:   audit.SeverityLow,
		TargetKind: objects.KindBacklogItem,
	}
	if err := CreateAuditEventWithBuilder(ctx, "", secCtx, nil, lowEvent); err != nil {
		t.Errorf("expected nil for best-effort empty project root, got: %v", err)
	}

	// 2. Fail-closed event (medium severity): should return error
	var recordedErr error
	medEvent := &AuditEventOptions{
		EventType:  audit.EventTypeCacheBulkInvalidation,
		Severity:   audit.SeverityMedium,
		TargetKind: objects.KindBacklogItem,
		OnError: func(err error) {
			recordedErr = err
		},
	}
	err := CreateAuditEventWithBuilder(ctx, "", secCtx, nil, medEvent)
	if err == nil {
		t.Fatal("expected error for fail-closed medium severity event with empty project root, got nil")
	}
	if recordedErr == nil {
		t.Fatal("expected OnError callback to receive error for fail-closed event")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected error message to mention empty root, got: %v", err)
	}

	// 3. Fail-closed scheduler job lifecycle with target_id: should return error
	jobEvent := &AuditEventOptions{
		EventType:  audit.EventTypeSchedulerJobStarted,
		Severity:   audit.SeverityLow,
		TargetKind: objects.KindSchedulerJob,
		TargetID:   "SCH-999",
	}
	if err := CreateAuditEventWithBuilder(ctx, "", secCtx, nil, jobEvent); err == nil {
		t.Fatal("expected error for fail-closed scheduler job lifecycle event with empty project root, got nil")
	}
}
