package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSeatWorker_ExtractATKID(t *testing.T) {
	assert.Empty(t, extractATKIDFromSteer(""))
	assert.Empty(t, extractATKIDFromSteer("no atk id here"))

	// Named ATK ID pattern
	steer1 := "Please work on task ATK-1791234567890123456-abcdef12 immediately"
	assert.Equal(t, "ATK-1791234567890123456-abcdef12", extractATKIDFromSteer(steer1))

	// Direct regex pattern match: ATK-[digits]-[hex]
	steer2 := "ATK-12345-abcdef12"
	assert.Equal(t, "ATK-12345-abcdef12", extractATKIDFromSteer(steer2))
}

func TestSeatWorker_WorkClass(t *testing.T) {
	wcCoding := seatWorkerWorkClass("Implement new feature in Go")
	assert.Equal(t, agentprompt.WorkClassCoding, wcCoding)

	wcDocs := seatWorkerWorkClass("docs_eval: evaluate architecture compliance")
	assert.True(t, wcDocs.IsDocsEval())
}

func TestSeatWorker_EnvHelpers(t *testing.T) {
	// firstNonEmptyEnv
	assert.Equal(t, "", firstNonEmptyEnv("", "  "))
	assert.Equal(t, "val", firstNonEmptyEnv("", "val", "val2"))

	// withEnvValue replace
	env := []string{"FOO=bar", "BAZ=qux"}
	updated := withEnvValue(env, "FOO", "newbar")
	assert.Contains(t, updated, "FOO=newbar")
	assert.NotContains(t, updated, "FOO=bar")

	// withEnvValue append
	appended := withEnvValue(env, "NEW", "value")
	assert.Contains(t, appended, "NEW=value")

	// withLocalLLMEnv
	localEnv := withLocalLLMEnv([]string{"PATH=/bin"})
	assert.NotEmpty(t, localEnv)
}

func TestSeatWorker_PlanDirectivesAndMarks(t *testing.T) {
	// validPlanDirectiveID
	assert.True(t, validPlanDirectiveID("PRI-123-abc"))
	assert.False(t, validPlanDirectiveID(""))
	assert.False(t, validPlanDirectiveID("PRI/invalid"))

	// parseOrchestratePlanDirective
	planID, ok, err := parseOrchestratePlanDirective("ORCHESTRATE_PLAN PRI-TEST-001")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "PRI-TEST-001", planID)

	_, okNoMatch, _ := parseOrchestratePlanDirective("unrelated message")
	assert.False(t, okNoMatch)

	// safePlanFileName
	assert.Equal(t, "PRI-TEST-001", safePlanFileName("PRI-TEST-001"))
	assert.Equal(t, "PRI_TEST_001", safePlanFileName("PRI/TEST/001"))

	// planOrchSubmitMarkPath & recentPlanOrchSubmit
	tmp := t.TempDir()
	markPath := planOrchSubmitMarkPath(tmp, "PRI-1")
	assert.Contains(t, markPath, "orch-PRI-1.json")

	isRecent, _ := recentPlanOrchSubmit(tmp, "PRI-1")
	assert.False(t, isRecent)

	require.NoError(t, writePlanOrchSubmitMark(tmp, "PRI-1", "submission detail"))
	isRecentAfter, msg := recentPlanOrchSubmit(tmp, "PRI-1")
	assert.True(t, isRecentAfter)
	assert.Contains(t, msg, "already_submitted")

	// isMarkDispatchedRecently
	assert.True(t, isMarkDispatchedRecently(markPath, 1*time.Hour))
	assert.False(t, isMarkDispatchedRecently(markPath, 0))
}

func TestSeatWorker_WriteResult(t *testing.T) {
	tmp := t.TempDir()
	writeSeatWorkerResult(tmp, "engine-test-1", "SUCCESS")

	resultFile := filepath.Join(tmp, paths.ProjectDataDir, paths.LogsDir, "agent-seat-worker", "engine-test-1.log")
	assert.True(t, fileutil.Exists(resultFile))
	content, err := fileutil.ReadFile(resultFile)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", string(content))
}

func TestSeatWorker_AckSkip(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, datacell.WriteAgentChatChannelConfig(tmp, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModePaste,
	}))

	channelFile := filepath.Join(tmp, paths.ProjectDataDir, paths.LogsDir, "ide-hooks", "agent_chat_channel.jsonl")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(channelFile)))
	require.NoError(t, fileutil.WriteStandardFile(channelFile, []byte("")))

	err := ackSeatWorkerSkip(tmp, "agent-1", "PER-1", "SES-1", "EVT-1", "skip reason")
	require.NoError(t, err)
}

func TestSeatWorker_BuildAgentXPrompts_MissingTask(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	tmp := t.TempDir()

	_, err := buildSeatWorkerAgentXPrompts(ctx, nil, sp, secCtx, tmp, "agent-1", "PER-1", "EVT-1", "no atk here")
	assert.ErrorIs(t, err, errPreparedContextRequired)
}

func TestSeatWorker_InboxProcessing(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	_, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: tempDir,
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeChat,
		AgentID:     "human",
		ToAgentID:   "SEAT-TEST-001",
		Message:     "Hello seat worker",
	})
	require.NoError(t, err)

	out, err := executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "SEAT-TEST-001", "--once")
	require.NoError(t, err)
	assert.Contains(t, out, "processed_comms")
}
