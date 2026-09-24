package bldr_cli_cmd_v1

import (
	"testing"
)

func TestCoreAgentCommandBuilders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buildCmd    func() any
		expectedUse string
	}{
		{"AgentCommand", func() any { return NewAgentCommandBuilder() }, "agent"},
		{"AgentOrchestrate", func() any { return NewAgentOrchestrateCommandBuilder() }, "orchestrate"},
		{"AgentBuildPrompt", func() any { return NewAgentBuildPromptCommandBuilder() }, "build-prompt"},
		{"AgentEvaluateRun", func() any { return NewAgentEvaluateRunCommandBuilder() }, "evaluate-run"},
		{"AgentExecute", func() any { return NewAgentExecuteCommandBuilder() }, "execute"},
		{"AgentGuidingStep", func() any { return NewAgentGuidingStepCommandBuilder() }, "guiding-step"},
		{"AgentNew", func() any { return NewAgentNewCommandBuilder() }, "new"},
		{"AgentNext", func() any { return NewAgentNextCommandBuilder() }, "next"},
		{"AgentRecover", func() any { return NewAgentRecoverCommandBuilder() }, "recover"},
		{"AgentSeatWorker", func() any { return NewAgentSeatWorkerCommandBuilder() }, "seat-worker"},
		{"AgentStatus", func() any { return NewAgentStatusCommandBuilder() }, "status"},
		{"AgentSyncLoop", func() any { return NewAgentSyncLoopCommandBuilder() }, "sync-loop"},
		{"AgentSynthesizeSkill", func() any { return NewAgentSynthesizeSkillCommandBuilder() }, "synthesize-skill"},
		{"AgentValidate", func() any { return NewAgentValidateCommandBuilder() }, "validate"},
		{"AgentScoreboard", func() any { return NewAgentScoreboardCommandBuilder() }, "scoreboard"},
		{"AgentSwarmInit", func() any { return NewAgentSwarmInitCommandBuilder() }, "swarm-init"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := tt.buildCmd()
			if val == nil {
				t.Fatalf("%s returned nil", tt.name)
			}
			if cmd, ok := val.(interface{ Use() string }); ok {
				if cmd.Use() != tt.expectedUse {
					t.Errorf("expected Use=%s, got %s", tt.expectedUse, cmd.Use())
				}
			} else if cobraCmd, ok := val.(*struct {
				Use string
			}); ok {
				if cobraCmd.Use != tt.expectedUse {
					t.Errorf("expected Use=%s, got %s", tt.expectedUse, cobraCmd.Use)
				}
			}
		})
	}
}

func TestCoreWorkflowCommandBuilders(t *testing.T) {
	t.Parallel()

	cmd := NewWorkflowWhatsNextCommandBuilder()
	if cmd == nil {
		t.Fatal("NewWorkflowWhatsNextCommandBuilder returned nil")
	}
	if cmd.Use != "whats-next" {
		t.Errorf("expected Use='whats-next', got '%s'", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}

	chk := NewWorkflowCheckCommandBuilder()
	if chk == nil {
		t.Fatal("NewWorkflowCheckCommandBuilder returned nil")
	}
	if chk.Use != "check" {
		t.Errorf("expected Use='check', got '%s'", chk.Use)
	}
}

func TestCoreSystemCommandBuilders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buildCmd    func() any
		expectedUse string
	}{
		{"SystemRoot", func() any { return NewSystemCommandBuilder() }, "system"},
		{"SystemCheck", func() any { return NewSystemCheckCommandBuilder() }, "check"},
		{"SystemInit", func() any { return NewSystemInitCommandBuilder() }, "init"},
		{"SystemAgentOnboard", func() any { return NewSystemAgentOnboardCommandBuilder() }, "agent-onboard"},
		{"SystemWhoami", func() any { return NewSystemWhoamiCommandBuilder() }, "whoami"},
		{"SystemAlign", func() any { return NewSystemAlignCommandBuilder() }, "align"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := tt.buildCmd()
			if val == nil {
				t.Fatalf("%s returned nil", tt.name)
			}
			if cmd, ok := val.(interface{ Use() string }); ok {
				if cmd.Use() != tt.expectedUse {
					t.Errorf("expected Use=%s, got %s", tt.expectedUse, cmd.Use())
				}
			}
		})
	}
}

func TestCoreSchedulerCommandBuilders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buildCmd    func() any
		expectedUse string
	}{
		{"SchedulerRoot", func() any { return NewSchedulerCommandBuilder() }, "scheduler"},
		{"SchedulerStart", func() any { return NewSchedulerStartCommandBuilder() }, "start"},
		{"SchedulerStop", func() any { return NewSchedulerStopCommandBuilder() }, "stop"},
		{"SchedulerStatus", func() any { return NewSchedulerStatusCommandBuilder() }, "status"},
		{"SchedulerService", func() any { return NewSchedulerServiceCommandBuilder() }, "service"},
		{"SchedulerHealth", func() any { return NewSchedulerHealthCommandBuilder() }, "health"},
		{"SchedulerIssues", func() any { return NewSchedulerIssuesCommandBuilder() }, "issues"},
		{"SchedulerAnalyze", func() any { return NewSchedulerAnalyzeCommandBuilder() }, "analyze"},
		{"SchedulerRerun", func() any { return NewSchedulerRerunCommandBuilder() }, "rerun"},
		{"SchedulerSubmit", func() any { return NewSchedulerSubmitCommandBuilder() }, "submit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := tt.buildCmd()
			if val == nil {
				t.Fatalf("%s returned nil", tt.name)
			}
			if cmd, ok := val.(interface{ Use() string }); ok {
				if cmd.Use() != tt.expectedUse {
					t.Errorf("expected Use=%s, got %s", tt.expectedUse, cmd.Use())
				}
			}
		})
	}
}

func TestCoreTestCommandBuilders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buildCmd    func() any
		expectedUse string
	}{
		{"TestRoot", func() any { return NewTestCommandBuilder() }, "test"},
		{"TestRun", func() any { return NewTestRunCommandBuilder() }, "run"},
		{"TestDashboard", func() any { return NewTestDashboardCommandBuilder() }, "dashboard"},
		{"TestBind", func() any { return NewTestBindCommandBuilder() }, "bind"},
		{"TestFailures", func() any { return NewTestFailuresCommandBuilder() }, "failures"},
		{"TestDiscover", func() any { return NewTestDiscoverCommandBuilder() }, "discover"},
		{"TestCheckContamination", func() any { return NewTestCheckContaminationCommandBuilder() }, "check-contamination"},
		{"TestStream", func() any { return NewTestStreamCommandBuilder() }, "stream"},
		{"TestIdentifyParallel", func() any { return NewTestIdentifyParallelCommandBuilder() }, "identify-parallel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := tt.buildCmd()
			if val == nil {
				t.Fatalf("%s returned nil", tt.name)
			}
			if cmd, ok := val.(interface{ Use() string }); ok {
				if cmd.Use() != tt.expectedUse {
					t.Errorf("expected Use=%s, got %s", tt.expectedUse, cmd.Use())
				}
			}
		})
	}
}

func TestCoreObjectCommandBuilders(t *testing.T) {
	t.Parallel()

	cmd := NewObjectGetCommandBuilder()
	if cmd == nil {
		t.Fatal("NewObjectGetCommandBuilder returned nil")
	}
	if cmd.Use != "get" {
		t.Errorf("expected Use='get', got '%s'", cmd.Use)
	}

	listCmd := NewObjectListCommandBuilder()
	if listCmd == nil {
		t.Fatal("NewObjectListCommandBuilder returned nil")
	}
	if listCmd.Use != "list" {
		t.Errorf("expected Use='list', got '%s'", listCmd.Use)
	}

	draftCmd := NewObjectDraftCommandBuilder()
	if draftCmd == nil {
		t.Fatal("NewObjectDraftCommandBuilder returned nil")
	}
	if draftCmd.Use != "draft" {
		t.Errorf("expected Use='draft', got '%s'", draftCmd.Use)
	}
}

func TestCoreAmbientCommandBuilders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buildCmd    func() any
		expectedUse string
	}{
		{"AmbientRoot", func() any { return NewAmbientCommandBuilder() }, "ambient"},
		{"AmbientAutomerge", func() any { return NewAmbientAutomergeCommandBuilder() }, "automerge"},
		{"AmbientIngest", func() any { return NewAmbientIngestCommandBuilder() }, "ingest"},
		{"AmbientStatus", func() any { return NewAmbientStatusCommandBuilder() }, "status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := tt.buildCmd()
			if val == nil {
				t.Fatalf("%s returned nil", tt.name)
			}
			if cmd, ok := val.(interface{ Use() string }); ok {
				if cmd.Use() != tt.expectedUse {
					t.Errorf("expected Use=%s, got %s", tt.expectedUse, cmd.Use())
				}
			}
		})
	}
}

func TestCoreFeedCommandBuilders(t *testing.T) {
	t.Parallel()

	cmd := NewFeedSteerCommandBuilder()
	if cmd == nil {
		t.Fatal("NewFeedSteerCommandBuilder returned nil")
	}
	if cmd.Use != "steer" {
		t.Errorf("expected Use='steer', got '%s'", cmd.Use)
	}

	docCmd := NewFeedDoctorCommandBuilder()
	if docCmd == nil {
		t.Fatal("NewFeedDoctorCommandBuilder returned nil")
	}
	if docCmd.Use != "doctor" {
		t.Errorf("expected Use='doctor', got '%s'", docCmd.Use)
	}
}
