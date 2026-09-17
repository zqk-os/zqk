package scheduler

import (
	"testing"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func TestJobLoader_HydrateJob(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	secCtx := env.SecurityContext
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	metrics := NewDefaultSchedulerMetricsCollector()

	loader := NewJobLoader(storage, secCtx, logger, metrics)

	tests := []struct {
		name      string
		jobData   map[string]any
		wantError bool
		wantNil   bool
		validate  func(t *testing.T, job *ScheduledJob)
	}{
		{
			name: "Valid timer job",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-001",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyCategory:           "maintenance",
				objects.FieldKeyExecutionMode:      "reusable",
				objects.FieldKeyMaxRuntimeSeconds:  3600,
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.ID != "JOB-001" {
					t.Errorf("Expected ID=JOB-001, got %s", job.ID)
				}
				if job.TriggerType != "timer" {
					t.Errorf("Expected TriggerType=timer, got %s", job.TriggerType)
				}
				if job.ScheduleExpr != "0 */6 * * *" {
					t.Errorf("Expected ScheduleExpr='0 */6 * * *', got %s", job.ScheduleExpr)
				}
			},
		},
		{
			name: "Timer job without schedule expression",
			jobData: map[string]any{
				objects.FieldKeyID:          "JOB-002",
				objects.FieldKeyKind:        "scheduler_job",
				objects.FieldKeyJobType:     JobTypeCachePrewarm,
				objects.FieldKeyTriggerType: "timer",
				objects.FieldKeyEnabled:     true,
			},
			wantError: true, // Should fail validation
			wantNil:   true,
		},
		{
			name: "Job with default category",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-003",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.Category != "maintenance" {
					t.Errorf("Expected default Category=maintenance, got %s", job.Category)
				}
			},
		},
		{
			name: "One-time job already executed",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-004",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyExecutionMode:      "one_time",
				objects.FieldKeyLastRunAt:          zqktime.NowRFC3339UTC(),
			},
			wantError: false,
			wantNil:   true, // Should return nil to skip
		},
		{
			name: "Run wrapper job with command",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-005",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeRunWrapper,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyCommand:            "echo",
				objects.FieldKeyCommandArgs:        []any{"hello", "world"},
				objects.FieldKeyRetryCount:         3,
				objects.FieldKeyRetryDelaySeconds:  5,
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.Command != "echo" {
					t.Errorf("Expected Command=echo, got %s", job.Command)
				}
				if len(job.CommandArgs) != 2 {
					t.Errorf("Expected 2 command args, got %d", len(job.CommandArgs))
				}
				if job.RetryCount != 3 {
					t.Errorf("Expected RetryCount=3, got %d", job.RetryCount)
				}
			},
		},
		{
			name: "run_wrapper job receives environment_variables from YAML (same treatment as other job types)",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-RW-ENV",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeRunWrapper,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyCommand:            "echo",
				objects.FieldKeyCommandArgs:        []any{"hello"},
				objects.FieldKeyEnvironmentVariables: map[string]any{
					zqkenv.ProjectRoot().Name(): "/path/to/project",
					"CUSTOM_VAR":                "value",
				},
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.EnvironmentVariables == nil {
					t.Fatal("EnvironmentVariables should be set for run_wrapper from YAML")
				}
				if v := job.EnvironmentVariables[zqkenv.ProjectRoot().Name()]; v != "/path/to/project" {
					t.Errorf("Expected %s=/path/to/project, got %q", zqkenv.ProjectRoot().Name(), v)
				}
				if v := job.EnvironmentVariables["CUSTOM_VAR"]; v != "value" {
					t.Errorf("Expected CUSTOM_VAR=value, got %q", v)
				}
			},
		},
		{
			name: "Job with log_level",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-006",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyLogLevel:           "verbose",
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.LogLevel != "verbose" {
					t.Errorf("Expected LogLevel=verbose, got %s", job.LogLevel)
				}
			},
		},
		{
			name: "Job with invalid log_level defaults to default",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-007",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyLogLevel:           "invalid",
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.LogLevel != "default" {
					t.Errorf("Expected LogLevel=default, got %s", job.LogLevel)
				}
			},
		},
		{
			name: "Job with priority critical",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-PRI",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyPriority:           "critical",
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.Priority != JobPriorityCritical {
					t.Errorf("Expected Priority=critical, got %s", job.Priority)
				}
			},
		},
		{
			name: "Job with missing priority defaults to normal",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-NOPRI",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeCachePrewarm,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 */6 * * *",
				objects.FieldKeyEnabled:            true,
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.Priority != JobPriorityNormal {
					t.Errorf("Expected default Priority=normal, got %s", job.Priority)
				}
			},
		},
		{
			name: "audit_event_aggregation job receives environment_variables from YAML",
			jobData: map[string]any{
				objects.FieldKeyID:                 "SCH-002",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeAuditEventAggregation,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "*/15 * * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyEnvironmentVariables: map[string]any{
					zqkenv.AggregationMetricCreationTimeout().Name(): "60s",
					EnvKeyAggregationWindow:                          "15m",
					EnvKeyRetentionDuration:                          "24h",
				},
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.EnvironmentVariables == nil {
					t.Fatal("EnvironmentVariables should be set for aggregation jobs from YAML")
				}
				if v := job.EnvironmentVariables[zqkenv.AggregationMetricCreationTimeout().Name()]; v != "60s" {
					t.Errorf("Expected %s=60s, got %q", zqkenv.AggregationMetricCreationTimeout().Name(), v)
				}
				if v := job.EnvironmentVariables[EnvKeyAggregationWindow]; v != "15m" {
					t.Errorf("Expected %s=15m, got %q", EnvKeyAggregationWindow, v)
				}
				if v := job.EnvironmentVariables[EnvKeyRetentionDuration]; v != "24h" {
					t.Errorf("Expected %s=24h, got %q", EnvKeyRetentionDuration, v)
				}
			},
		},
		{
			name: "retention_tolerance job receives environment_variables from YAML (same treatment as other job types)",
			jobData: map[string]any{
				objects.FieldKeyID:                 "JOB-RT-ENV",
				objects.FieldKeyKind:               "scheduler_job",
				objects.FieldKeyJobType:            JobTypeRetentionTolerance,
				objects.FieldKeyTriggerType:        "timer",
				objects.FieldKeyScheduleExpression: "0 0 * * *",
				objects.FieldKeyEnabled:            true,
				objects.FieldKeyEnvironmentVariables: map[string]any{
					EnvKeyBatchSize:  "100",
					EnvKeyMaxBatches: "50",
				},
			},
			wantError: false,
			wantNil:   false,
			validate: func(t *testing.T, job *ScheduledJob) {
				if job.EnvironmentVariables == nil {
					t.Fatal("EnvironmentVariables should be set for retention_tolerance from YAML")
				}
				if v := job.EnvironmentVariables[EnvKeyBatchSize]; v != "100" {
					t.Errorf("Expected %s=100, got %q", EnvKeyBatchSize, v)
				}
				if v := job.EnvironmentVariables[EnvKeyMaxBatches]; v != "50" {
					t.Errorf("Expected %s=50, got %q", EnvKeyMaxBatches, v)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := loader.HydrateJob(tt.jobData)

			if (err != nil) != tt.wantError {
				t.Errorf("HydrateJob() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if (job == nil) != tt.wantNil {
				t.Errorf("HydrateJob() job = %v, wantNil %v", job, tt.wantNil)
				return
			}

			if job != nil && tt.validate != nil {
				tt.validate(t, job)
			}
		})
	}
}

func TestJobLoader_LoadJobs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testRoot, err := setupSchedulerTestEnvironmentRoot(tmpDir)
	if err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}
	mod := moduleRootFromGoEnvSchedulerOrEmpty(t)
	if err := testenvroot.BootstrapRoot(testRoot, mod); err != nil {
		t.Fatalf("failed to bootstrap root: %v", err)
	}

	storage, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)

	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	metrics := NewDefaultSchedulerMetricsCollector()

	loader := NewJobLoader(storage, secCtx, logger, metrics)

	storagepkg.BuildPathAliasCacheForProject(testRoot)
	// Create test jobs (using valid ID format and required fields)
	ctx := pkgctx.NewSystemContext()
	t.Cleanup(func() {
		// Ensure async index updates are flushed before TempDir cleanup.
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})
	job1 := map[string]any{
		objects.FieldKeyID:                 "SCH-001",
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyJobType:            JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 */6 * * *",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  3600,
		objects.FieldKeyTitle:              "Test Cache Prewarm Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyCreatedAt:          zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:       validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	job2 := map[string]any{
		objects.FieldKeyID:                "SCH-002",
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyJobType:           JobTypeLifecycleCheck,
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCategory:          "monitoring",
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 300,
		objects.FieldKeyTitle:             "Test Lifecycle Check Job",
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyCreatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	err = storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, job1)
	if err != nil {
		t.Fatalf("Failed to create job1: %v", err)
	}
	err = storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, job2)
	if err != nil {
		t.Fatalf("Failed to create job2: %v", err)
	}

	// Load jobs
	jobs, err := loader.LoadJobs(ctx)
	if err != nil {
		t.Fatalf("LoadJobs() error = %v", err)
	}

	if len(jobs) < 2 {
		t.Errorf("Expected at least 2 jobs, got %d", len(jobs))
	}

	// Verify metrics were recorded
	snapshot := metrics.GetMetrics()
	if snapshot.Lifecycle.JobLoadErrors > 0 {
		t.Error("Expected no job load errors")
	}
}

func TestJobLoader_LoadJobs_ExcludesMarkedForDeletion(t *testing.T) {
	testRoot, storage := newSchedulerTestStorage(t)
	storagepkg.BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	metrics := NewDefaultSchedulerMetricsCollector()
	loader := NewJobLoader(storage, secCtx, logger, metrics)

	ctx := pkgctx.NewSystemContext()

	base := map[string]any{
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyMaxRuntimeSeconds: 3600,
		objects.FieldKeyCreatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	// Reusable job — should be loaded
	reusable := make(map[string]any)
	for k, v := range base {
		reusable[k] = v
	}
	reusable[objects.FieldKeyID] = "SCH-LOAD-A"
	reusable[objects.FieldKeyJobType] = "lifecycle_check"
	reusable[objects.FieldKeyEnabled] = true
	reusable[objects.FieldKeyExecutionMode] = ExecutionModeReusable
	reusable[objects.FieldKeyCategory] = "maintenance"
	reusable[objects.FieldKeyTitle] = "Reusable"
	reusable[objects.FieldKeyStatus] = StatusActive

	// One-time marked for deletion (enabled=false) — should NOT be loaded
	marked1 := make(map[string]any)
	for k, v := range base {
		marked1[k] = v
	}
	marked1[objects.FieldKeyID] = "SCH-LOAD-B"
	marked1[objects.FieldKeyJobType] = "lifecycle_check"
	marked1[objects.FieldKeyEnabled] = false
	marked1[objects.FieldKeyExecutionMode] = ExecutionModeOneTime
	marked1[objects.FieldKeyCategory] = "maintenance"
	marked1[objects.FieldKeyTitle] = "Marked disabled"
	marked1[objects.FieldKeyStatus] = StatusActive

	// One-time marked for deletion (status=disabled) — should NOT be loaded
	marked2 := make(map[string]any)
	for k, v := range base {
		marked2[k] = v
	}
	marked2[objects.FieldKeyID] = "SCH-LOAD-C"
	marked2[objects.FieldKeyJobType] = "lifecycle_check"
	marked2[objects.FieldKeyEnabled] = true
	marked2[objects.FieldKeyExecutionMode] = ExecutionModeOneTime
	marked2[objects.FieldKeyCategory] = "maintenance"
	marked2[objects.FieldKeyTitle] = "Marked status disabled"
	marked2[objects.FieldKeyStatus] = StatusDisabled

	for _, obj := range []map[string]any{reusable, marked1, marked2} {
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, obj); err != nil {
			t.Fatalf("Failed to create job %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	jobs, err := loader.LoadJobs(ctx)
	if err != nil {
		t.Fatalf("LoadJobs() error = %v", err)
	}

	ids := make([]string, 0, len(jobs))
	for _, j := range jobs {
		if id, ok := j[objects.FieldKeyID].(string); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) != 1 || ids[0] != "SCH-LOAD-A" {
		t.Errorf("LoadJobs() should return only non–marked job; got ids %v", ids)
	}
}

func TestPriorityRank_and_rawJobPriority(t *testing.T) {
	t.Parallel()
	// PriorityRank: higher = run first
	if r := PriorityRank(JobPriorityCritical); r != 2 {
		t.Errorf("PriorityRank(critical) = %d, want 2", r)
	}
	if r := PriorityRank(JobPriorityHigh); r != 1 {
		t.Errorf("PriorityRank(high) = %d, want 1", r)
	}
	if r := PriorityRank(JobPriorityNormal); r != 0 {
		t.Errorf("PriorityRank(normal) = %d, want 0", r)
	}
	if r := PriorityRank(""); r != 0 {
		t.Errorf("PriorityRank(empty) = %d, want 0", r)
	}
	// rawJobPriority from raw map
	if p := rawJobPriority(map[string]any{objects.FieldKeyPriority: "critical"}); p != JobPriorityCritical {
		t.Errorf("rawJobPriority(critical) = %q, want critical", p)
	}
	if p := rawJobPriority(map[string]any{objects.FieldKeyPriority: "high"}); p != JobPriorityHigh {
		t.Errorf("rawJobPriority(high) = %q, want high", p)
	}
	if p := rawJobPriority(map[string]any{}); p != JobPriorityNormal {
		t.Errorf("rawJobPriority(empty) = %q, want normal", p)
	}
	if p := rawJobPriority(map[string]any{objects.FieldKeyPriority: "unknown"}); p != JobPriorityNormal {
		t.Errorf("rawJobPriority(unknown) = %q, want normal", p)
	}
}

func TestJobLoader_RefreshEnvironmentVariablesFromRaw(t *testing.T) {
	t.Parallel()
	jl := &JobLoader{}
	job := &ScheduledJob{
		EnvironmentVariables: map[string]string{"STALE": "yes"},
	}
	raw := map[string]any{
		objects.FieldKeyEnvironmentVariables: map[string]any{EnvKeyConvergenceSessionID: "[REDACTED-ID]"},
	}
	jl.RefreshEnvironmentVariablesFromRaw(job, raw)
	if job.EnvironmentVariables[EnvKeyConvergenceSessionID] != "[REDACTED-ID]" {
		t.Fatalf("missing CVS env, got %#v", job.EnvironmentVariables)
	}
	if _, ok := job.EnvironmentVariables["STALE"]; ok {
		t.Fatal("expected stale keys replaced from storage snapshot")
	}
}
