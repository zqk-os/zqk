package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// agentPromptChatMatrixCycles is how many full matrix passes to run back-to-back (simulates repeated
// chat send cycles without human typing).
const agentPromptChatMatrixCycles = 3

// TestCLI_AgentPromptChatMatrix_MultiCycle runs the real zqk binary against an isolated project:
// for each cycle, exercises every attention-mode scenario and asserts the pasted-markdown envelope
// and body match agentprompt semantics (chat plumbing contract).
// CVS fixture template: integration/fixtures/convergence_session/agent_prompt_chat_matrix.yaml (see integration/README.md).
// A unique CVS id is substituted per run so CAS id indexes cannot collide across test runs or machines.
//
// Not parallel: object.SetupTestEnvironment uses a process-global test root.
func TestCLI_AgentPromptChatMatrix_MultiCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode")
	}

	te := object.SetupTestEnvironment(t)
	root := te.GetTestRoot()

	healthPath := schedpkg.TestBundlesHealthFilePath(root)
	if err := fileutil.MkdirAll(filepath.Dir(healthPath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir health dir: %v", err)
	}
	ts := zqktime.NowRFC3339UTC()
	line := map[string]any{
		schedpkg.KeyTimestamp:                ts,
		schedpkg.KeyBundleCommandFingerprint: "chat-matrix-fp",
		schedpkg.KeyTestOutcome:              "pass",
	}
	b, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("marshal health line: %v", err)
	}
	if err := fileutil.WriteFile(healthPath, append(b, '\n'), paths.FilePerm644); err != nil {
		t.Fatalf("write health.jsonl: %v", err)
	}

	const fixtureTemplateID = "CVS-ITEST-AGENT-PROMPT-CHAT-MATRIX-001"
	cvsID := fmt.Sprintf("CVS-ITEST-AGENT-PROMPT-CHAT-%d", time.Now().UnixNano())
	fixturePath := filepath.Join(te.ProjectRoot, "integration", "fixtures", "convergence_session", "agent_prompt_chat_matrix.yaml")
	raw, err := fileutil.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("integration pillar fixture (see integration/README.md): %v", err)
	}
	patched := strings.ReplaceAll(string(raw), fixtureTemplateID, cvsID)
	tmpYAML := filepath.Join(t.TempDir(), "cvs-chat-matrix.yaml")
	if err := fileutil.WriteFile(tmpYAML, []byte(patched), paths.FilePerm644); err != nil {
		t.Fatalf("write patched cvs yaml: %v", err)
	}
	runZQKCLI(t, te, "object", "create", "convergence_session", "--file", tmpYAML)

	matrix := []struct {
		name            string
		extraArgs       []string
		wantEnvelope    bool
		wantIgnoreND    bool
		wantInterruptDR bool
		mustContain     []string
		mustNotContain  []string
	}{
		{
			name:            "default_no_attention_flag",
			extraArgs:       nil,
			wantEnvelope:    false,
			wantIgnoreND:    false,
			wantInterruptDR: false,
			mustContain:     []string{"# Convergence — agent prompt", "**Session:** `" + cvsID + "`"},
			mustNotContain:  []string{"<!-- zqk:agent_prompt", "Attention — test / non-directive"},
		},
		{
			name:            "attention_test_non_directive",
			extraArgs:       []string{"--attention-mode", "test_non_directive"},
			wantEnvelope:    true,
			wantIgnoreND:    true,
			wantInterruptDR: false,
			mustContain: []string{
				"<!-- zqk:agent_prompt",
				"test / non-directive",
				agentprompt.SchemaV1,
				agentprompt.AttentionTestNonDirective,
			},
			mustNotContain: []string{"Attention — critical interrupt"},
		},
		{
			name:            "attention_interrupt_plumbing",
			extraArgs:       []string{"--attention-mode", "interrupt_plumbing"},
			wantEnvelope:    true,
			wantIgnoreND:    false,
			wantInterruptDR: true,
			mustContain: []string{
				"<!-- zqk:agent_prompt",
				"critical interrupt",
				agentprompt.AttentionInterruptPlumbing,
			},
			mustNotContain: []string{"test / non-directive (ZQK)"},
		},
		{
			name:            "alias_test_maps_to_non_directive",
			extraArgs:       []string{"--attention-mode", "test"},
			wantEnvelope:    true,
			wantIgnoreND:    true,
			wantInterruptDR: false,
			mustContain:     []string{"<!-- zqk:agent_prompt", agentprompt.AttentionTestNonDirective},
			mustNotContain:  nil,
		},
	}

	for cycle := 0; cycle < agentPromptChatMatrixCycles; cycle++ {
		for _, row := range matrix {
			label := fmt.Sprintf("cycle_%d_%s", cycle+1, row.name)
			t.Run(label, func(t *testing.T) {
				args := []string{
					"scheduler", "convergence", "measure",
					"--format", "agent-prompt",
					"--session-id", cvsID,
				}
				args = append(args, row.extraArgs...)
				out := runZQKCLI(t, te, args...)
				s := string(out)

				for _, sub := range row.mustContain {
					if !strings.Contains(s, sub) {
						t.Fatalf("expected stdout to contain %q\n---\n%s", sub, s)
					}
				}
				for _, sub := range row.mustNotContain {
					if sub != "" && strings.Contains(s, sub) {
						t.Fatalf("expected stdout not to contain %q\n---\n%s", sub, s)
					}
				}

				env, _, err := agentprompt.ParseLeadingEnvelope(s)
				if err != nil {
					t.Fatalf("ParseLeadingEnvelope: %v\n---\n%s", err, s)
				}
				if row.wantEnvelope {
					if env == nil {
						t.Fatalf("expected envelope, got nil")
					}
					if env.Schema != agentprompt.SchemaV1 {
						t.Fatalf("schema: got %q", env.Schema)
					}
					if env.IgnoreAsOperationalDirective() != row.wantIgnoreND {
						t.Fatalf("IgnoreAsOperationalDirective: got %v want %v", env.IgnoreAsOperationalDirective(), row.wantIgnoreND)
					}
					if env.InterruptPlumbingDryRun() != row.wantInterruptDR {
						t.Fatalf("InterruptPlumbingDryRun: got %v want %v", env.InterruptPlumbingDryRun(), row.wantInterruptDR)
					}
				} else {
					if env != nil {
						t.Fatalf("expected no envelope, got %+v", env)
					}
				}
			})
		}
	}

	// Audit: each successful agent-prompt run appends one line to agent_prompt_runs.jsonl (FINALIZE).
	wantRuns := len(matrix) * agentPromptChatMatrixCycles
	runsPath := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, agentPromptRunsJSONLFile)
	rawRuns, err := fileutil.ReadFile(runsPath)
	if err != nil {
		t.Fatalf("read agent_prompt_runs.jsonl: %v", err)
	}
	var runLines []string
	for _, line := range strings.Split(strings.TrimSpace(string(rawRuns)), "\n") {
		if strings.TrimSpace(line) != "" {
			runLines = append(runLines, line)
		}
	}
	if len(runLines) != wantRuns {
		t.Fatalf("agent_prompt_runs.jsonl: want %d non-empty lines, got %d", wantRuns, len(runLines))
	}
	for i, ln := range runLines {
		var row map[string]any
		if err := json.Unmarshal([]byte(ln), &row); err != nil {
			t.Fatalf("line %d json: %v", i+1, err)
		}
		if row[schedpkg.KeyTimestamp] == nil || row[schedpkg.KeyTimestamp] == "" {
			t.Fatalf("line %d: missing timestamp", i+1)
		}
		if row[agentPromptRunJSONKeyEventType] != agentPromptRunEventType {
			t.Fatalf("line %d: event_type got %v", i+1, row[agentPromptRunJSONKeyEventType])
		}
		if row[agentPromptRunJSONKeyPipelineKind] != pipelineKindConvergenceAgentPrompt {
			t.Fatalf("line %d: pipeline_kind got %v", i+1, row[agentPromptRunJSONKeyPipelineKind])
		}
		if row[outcomeAgentPromptSessionID] != cvsID {
			t.Fatalf("line %d: convergence_session_id got %v", i+1, row[outcomeAgentPromptSessionID])
		}
		if _, ok := row[outcomeAgentPromptMarkdownBytes]; !ok {
			t.Fatalf("line %d: missing markdown_bytes", i+1)
		}
		if _, ok := row[outcomeAgentPromptDeliverMode]; !ok {
			t.Fatalf("line %d: missing deliver_mode", i+1)
		}
		if _, ok := row[agentprompt.KeyAttentionMode]; !ok {
			t.Fatalf("line %d: missing attention_mode", i+1)
		}
	}
}
