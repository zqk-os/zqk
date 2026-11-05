package system

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestExpansion_InterruptInbox(t *testing.T) {
	cmd := NewInterruptInboxCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "interrupt-inbox", cmd.Use)

	tmpDir := t.TempDir()
	t.Cleanup(func() {
		testkit.ScrubProjectRootForTempCleanup(tmpDir, 40, 25*time.Millisecond)
	})

	dotZqk := filepath.Join(tmpDir, paths.ProjectDataDir)
	_ = fileutil.MkdirAll(dotZqk, paths.DirPerm755)

	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	zqkenv.ProjectRoot().Set(tmpDir)

	// Run with empty inbox
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), buf)
	cmd.SetContext(ctx)

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "empty")
}

func TestExpansion_AuditBufferCommandsAndOps(t *testing.T) {
	rootCmd := NewAuditBufferCmd()
	require.NotNil(t, rootCmd)
	assert.Equal(t, "audit-buffer", rootCmd.Use)

	statsCmd := NewAuditBufferStatsCmd()
	require.NotNil(t, statsCmd)
	assert.Equal(t, "stats", statsCmd.Use)

	flushCmd := NewAuditBufferFlushCmd()
	require.NotNil(t, flushCmd)
	assert.Equal(t, "flush", flushCmd.Use)

	enableCmd := NewAuditBufferEnableCmd()
	require.NotNil(t, enableCmd)
	assert.Equal(t, "enable", enableCmd.Use)

	disableCmd := NewAuditBufferDisableCmd()
	require.NotNil(t, disableCmd)
	assert.Equal(t, "disable", disableCmd.Use)

	// Direct execution of runAuditBuffer* functions
	b := storage.GetGlobalAuditEventBuffer()
	require.NotNil(t, b)

	execCmd := func(runFunc func(*cobra.Command, []string) error, format string) (string, error) {
		c := NewAuditBufferStatsCmd()
		_ = c.Flags().Set("format", format)
		buf := new(bytes.Buffer)
		c.SetOut(buf)
		c.SetErr(buf)
		c.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
		err := runFunc(c, nil)
		return buf.String(), err
	}

	// Stats table and json
	out, err := execCmd(runAuditBufferStats, "table")
	assert.NoError(t, err)
	assert.Contains(t, out, "Audit Event Buffer Statistics")

	out, err = execCmd(runAuditBufferStats, "json")
	assert.NoError(t, err)
	assert.Contains(t, out, "{")

	// Flush
	out, err = execCmd(runAuditBufferFlush, "table")
	assert.NoError(t, err)
	assert.Contains(t, out, "flushed")

	// Enable
	out, err = execCmd(runAuditBufferEnable, "table")
	assert.NoError(t, err)
	assert.Contains(t, out, "enabled")

	// Disable
	out, err = execCmd(runAuditBufferDisable, "table")
	assert.NoError(t, err)
	assert.Contains(t, out, "disabled")
}

func TestExpansion_CheckImplAuditBuffer(t *testing.T) {
	assert.True(t, isObjectKind("goal"))
	assert.True(t, isObjectKind("requirement"))
	assert.False(t, isObjectKind("not_a_valid_kind_xyz"))

	merged := mergeMetadata(map[string]any{"a": 1}, map[string]any{"b": 2})
	assert.Equal(t, 1, merged["a"])
	assert.Equal(t, 2, merged["b"])

	buf := &AuditEventBuffer{
		buffer: make(map[string]*BufferedEvent),
	}
	key := buf.generateKey("TARGET-01", "create")
	assert.Equal(t, "TARGET-01:create", key)

	buf.Add("create", "TARGET-01", "goal", "/path/target.yaml", "create", "info", map[string]any{"foo": "bar"})
	assert.Equal(t, 1, buf.GetBufferCount())

	summary := buf.GetBufferSummary()
	assert.Equal(t, 1, summary["create"])

	// Add hash mismatch fix
	obj := &parser.ParsedObject{
		ID:   "TARGET-01",
		Kind: "goal",
	}
	buf.AddHashMismatchFix(obj, "/path/target.yaml", "goal", "hashA", "hashB")

	// Flush on empty buffer
	emptyBuf := &AuditEventBuffer{
		buffer: make(map[string]*BufferedEvent),
	}
	count, err := emptyBuf.Flush()
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	ResetAuditEventBuffer()
	tmpDir2 := t.TempDir()
	gBuf := GetAuditEventBuffer(cli.ContextForProjectRoot(tmpDir2))
	assert.NotNil(t, gBuf)
}

func TestExpansion_CleanupDuplicatesHelpers(t *testing.T) {
	assert.True(t, isTraditionalYAMLFile("my_goal.yaml", false))
	assert.False(t, isTraditionalYAMLFile(".hidden.yaml", false))
	assert.False(t, isTraditionalYAMLFile("not_yaml.json", false))
	assert.False(t, isTraditionalYAMLFile("my_dir", true))

	builtPath := buildHashFilePath("/base/dir", "abc123hash", "ab")
	assert.Equal(t, filepath.Join("/base/dir", "ab", "abc123hash.yaml"), builtPath)

	tmpDir := t.TempDir()
	sampleYAML := filepath.Join(tmpDir, "sample.yaml")
	err := fileutil.WriteFile(sampleYAML, []byte("id: SAMPLE-001\nkind: goal\ntitle: Sample\n"), 0644)
	require.NoError(t, err)

	id, err := extractObjectIDFromYAMLHead(sampleYAML, 256)
	assert.NoError(t, err)
	assert.Equal(t, "SAMPLE-001", id)

	// Non-existent file
	_, err = extractObjectIDFromYAMLHead(filepath.Join(tmpDir, "missing.yaml"), 256)
	assert.Error(t, err)
}

func TestExpansion_SpecAutoFixerCoercion(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	strVal := coerceToString(12345)
	require.NotNil(t, strVal)
	assert.Equal(t, "12345", *strVal)

	intVal := coerceToInt("42")
	require.NotNil(t, intVal)
	assert.Equal(t, 42, *intVal)

	floatVal := coerceToFloat("3.1415")
	require.NotNil(t, floatVal)
	assert.InDelta(t, 3.1415, *floatVal, 0.0001)

	bTrue := coerceToBool("true")
	require.NotNil(t, bTrue)
	assert.True(t, *bTrue)

	bFalse := coerceToBool(0)
	require.NotNil(t, bFalse)
	assert.False(t, *bFalse)

	dt := coerceToDateTime("2026-11-05T12:00:00Z", logger)
	require.NotNil(t, dt)
	assert.Contains(t, *dt, "2026-11-05")

	d := coerceToDate("2026-11-05", logger)
	require.NotNil(t, d)
	assert.Equal(t, "2026-11-05", *d)

	objVal := coerceToObject(map[string]any{"foo": "bar"}, logger)
	assert.Equal(t, "bar", objVal["foo"])

	assert.Equal(t, "abcde...", truncateForLog("abcdefghij", 5))
	assert.Equal(t, "abc", truncateForLog("abc", 5))

	// coerceType dispatching
	res := coerceType("age", "30", map[string]any{"type": "integer"}, logger)
	assert.Equal(t, 30, res)

	res = coerceType("active", "true", map[string]any{"type": "boolean"}, logger)
	assert.Equal(t, true, res)

	res = coerceType("score", "99.5", map[string]any{"type": "number"}, logger)
	assert.Equal(t, 99.5, res)

	res = coerceType("name", 123, map[string]any{"type": "string"}, logger)
	assert.Equal(t, "123", res)
}

func TestExpansion_SpecAutoFixerLengthAndPattern(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Length constraints
	fMapMin := map[string]any{
		"validation": map[string]any{
			"min_length": 5,
		},
	}
	padded := padToMinLength("name", "ab", fMapMin, logger)
	assert.Equal(t, "ab   ", padded)

	fMapMax := map[string]any{
		"validation": map[string]any{
			"max_length": 4,
		},
	}
	truncated := truncateToMaxLength("name", "abcdefgh", fMapMax, logger)
	assert.Equal(t, "abcd", truncated)

	// Array length constraints
	fMapArrMin := map[string]any{
		"type": "list",
		"validation": map[string]any{
			"min_length": 3,
		},
	}
	paddedArr := padArrayToMinLength("items", []any{"x"}, fMapArrMin, logger)
	arrSlice, ok := paddedArr.([]any)
	assert.True(t, ok)
	assert.Len(t, arrSlice, 3)

	fMapArrMax := map[string]any{
		"type": "list",
		"validation": map[string]any{
			"max_length": 2,
		},
	}
	truncatedArr := truncateArrayToMaxLength("items", []any{"a", "b", "c", "d"}, fMapArrMax, logger)
	arrSliceMax, ok := truncatedArr.([]any)
	assert.True(t, ok)
	assert.Len(t, arrSliceMax, 2)

	// Pattern fixing
	fixedID := fixIDPattern("req-999")
	require.NotNil(t, fixedID)
	assert.Equal(t, "REQ-999", *fixedID)

	fixedNS := fixNamespacePattern("", "^(zqk|domain|integration):[a-z0-9_]+$", logger)
	require.NotNil(t, fixedNS)
	assert.Equal(t, "zqk:default", *fixedNS)

	patternRes := fixPatternViolation("namespace_id", "", map[string]any{
		"validation": map[string]any{
			"pattern": "^(zqk|domain|integration):[a-z0-9_]+$",
		},
	}, logger)
	assert.NotNil(t, patternRes)
}

func TestExpansion_UpdateSpecsTraitHelpers(t *testing.T) {
	contains, extras := checkTraitsContainBase([]string{"base_a"}, []string{"base_a", "extra_1"})
	assert.True(t, contains)
	assert.Equal(t, []string{"extra_1"}, extras)

	contains, _ = checkTraitsContainBase([]string{"base_a", "base_b"}, []string{"base_a"})
	assert.False(t, contains)

	replaced, newContent := replaceObjectTraitsSection("traits:\n  - old_trait\n", "traits:\n  - new_trait\n", nil)
	assert.True(t, replaced)
	assert.Contains(t, newContent, "new_trait")

	patterns := getFieldTraitGroupPatterns()
	assert.NotEmpty(t, patterns)

	isStart, fName := detectFieldStart("title:", 4, map[string]any{"title": map[string]any{}})
	assert.True(t, isStart)
	assert.Equal(t, "title", fName)

	assert.True(t, detectFieldEnd("description:", 0, 2))
	assert.False(t, detectFieldEnd("    nested:", 4, 2))

	assert.True(t, detectFieldTraitsStart("traits:", 4, 2))
	assert.False(t, detectFieldTraitsStart("other:", 4, 2))
}

func TestExpansion_FixHashMismatches(t *testing.T) {
	cmd := NewFixHashMismatchesCmd()
	require.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "fix-hash-mismatches")

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	s1, err := createStrategy("remove-stale-index", logger)
	assert.NoError(t, err)
	assert.NotNil(t, s1)

	s2, err := createStrategy("recompute-hash", logger)
	assert.NoError(t, err)
	assert.NotNil(t, s2)

	s3, err := createStrategy("force-reindex", logger)
	assert.NoError(t, err)
	assert.NotNil(t, s3)

	_, err = createStrategy("nonexistent", logger)
	assert.Error(t, err)

	ids, err := readObjectIDs([]string{"ID-1", "ID-2", "ID-1"}, "")
	assert.NoError(t, err)
	assert.Equal(t, []string{"ID-1", "ID-2"}, ids)

	tmpDir := t.TempDir()
	idFile := filepath.Join(tmpDir, "ids.txt")
	err = fileutil.WriteFile(idFile, []byte("ID-A\nID-B\nID-A\n"), 0644)
	require.NoError(t, err)

	idsFromFile, err := readObjectIDs(nil, idFile)
	assert.NoError(t, err)
	assert.Equal(t, []string{"ID-A", "ID-B"}, idsFromFile)
}

func TestExpansion_UpdateHashRegistryHelpers(t *testing.T) {
	validCAS := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.yaml"
	assert.True(t, isCASFile(validCAS))
	assert.False(t, isCASFile("short.yaml"))
	assert.False(t, isCASFile("nothex___________________________________________________________.yaml"))

	tmpDir := t.TempDir()
	validYAML := filepath.Join(tmpDir, "test.yaml")
	_ = fileutil.WriteFile(validYAML, []byte("id: TEST-001\nkind: goal\n"), 0644)

	id, err := extractObjectIDFromYAMLFile(validYAML)
	assert.NoError(t, err)
	assert.Equal(t, "TEST-001", id)

	_, err = extractObjectIDFromYAMLFile(filepath.Join(tmpDir, "missing.yaml"))
	assert.Error(t, err)

	mismatch, _, err := checkCASHashMismatch(filepath.Join(tmpDir, "non_cas.yaml"))
	assert.NoError(t, err)
	assert.False(t, mismatch)
}

func TestExpansion_DiscoverGoalsAndQuestionsAndSign(t *testing.T) {
	discCmd := NewDiscoverGoalsCmd()
	require.NotNil(t, discCmd)
	assert.Equal(t, "discover-goals", discCmd.Use)

	summary := summarizeContext(map[string][]map[string]any{
		"goal": {{"id": "G1"}, {"id": "G2"}},
		"task": {{"id": "T1"}},
	})
	assert.Equal(t, 2, summary["goal"])
	assert.Equal(t, 1, summary["task"])

	seedCmd := NewSeedQuestionsCmd()
	require.NotNil(t, seedCmd)
	assert.Equal(t, "seed-questions", seedCmd.Use)

	questions := defaultEssentialSeedQuestions()
	assert.NotEmpty(t, questions)

	assert.Equal(t, []string{"a", "b", "c"}, splitCommaNonEmpty("a, b,, c "))

	emitCmd := NewEmitContextEventCmd()
	require.NotNil(t, emitCmd)
	assert.Equal(t, "emit-context-event", emitCmd.Use)

	signCmd := NewSignCmd()
	require.NotNil(t, signCmd)
	assert.Contains(t, signCmd.Use, "sign")
}

func TestExpansion_KernelIntegrityComposeAndCacheAudit(t *testing.T) {
	compCmd := newKernelIntegrityComposeCmd()
	require.NotNil(t, compCmd)
	assert.Equal(t, "compose", compCmd.Use)

	id := stablePipelineDefinitionID(compose.CompositionKey{ObjectKind: "goal", PipelineKind: "create", Intent: "mint"})
	assert.NotEmpty(t, id)

	stages := composedStagesAsMaps(&compose.Definition{})
	assert.NotNil(t, stages)

	cacheAuditCmd := NewCacheAuditCmd()
	require.NotNil(t, cacheAuditCmd)
	assert.Equal(t, "cache-audit", cacheAuditCmd.Use)

	delInfo := findGitDeletionInfo(t.TempDir(), "dummy.txt")
	assert.Nil(t, delInfo)

	syncCmd := NewCompactStreamStateCmd()
	require.NotNil(t, syncCmd)
	assert.Equal(t, "compact-stream-state", syncCmd.Use)

	migCmd := NewMigrateCmd()
	require.NotNil(t, migCmd)
	assert.Contains(t, migCmd.Use, "migrate")

	tmpDir := t.TempDir()
	checkCmd := NewCheckCmd()
	cliCtx := cli.ContextForProjectRoot(tmpDir)
	resSync := handleSynchronousAutoFix(cliCtx, checkCmd, tmpDir, nil)
	assert.Empty(t, resSync)

	errBatch := handleAutoFixBatching(cliCtx, checkCmd, tmpDir, nil)
	assert.NoError(t, errBatch)
}

func TestExpansion_ServiceAndRetentionJobs(t *testing.T) {
	srvCmd := NewServiceCmd()
	require.NotNil(t, srvCmd)
	assert.Equal(t, "service", srvCmd.Use)

	assert.NotNil(t, NewServiceStartCmd())
	assert.NotNil(t, NewServiceStopCmd())
	assert.NotNil(t, NewServiceStatusCmd())
	assert.NotNil(t, NewServiceListCmd())

	sm := NewServiceManager()
	require.NotNil(t, sm)
	services := sm.ListServices()
	assert.NotEmpty(t, services)

	_, err := sm.getServiceConfig("nonexistent_service_name_xyz")
	assert.Error(t, err)

	// Ensure retention jobs helpers
	res := &EnsureRetentionJobsResult{}
	setResultCreatedByID(res, "SCH-job-1", true)
	assert.Len(t, res.CreatedJobIDs, 1)

	setResultUpdatedByID(res, "SCH-retention-tolerance", "SCH-retention-tolerance-updated")
	assert.NotEmpty(t, res.RetentionToleranceJobUpdated)

	msg := buildEnsureResultMessage(res)
	assert.Contains(t, msg, "scheduler_maintenance_config.yaml")

	envMap := environmentVariablesMapFromObject(map[string]any{
		"environment_variables": map[string]any{"K1": "V1"},
	})
	assert.Equal(t, "V1", envMap["K1"])

	strMap := envToStringMap(map[string]any{"A": "1", "B": 2})
	assert.Equal(t, "1", strMap["A"])
	assert.Equal(t, "2", strMap["B"])

	assert.True(t, environmentStringMapsEqual(map[string]any{"A": "1"}, map[string]any{"A": "1"}))
	assert.False(t, environmentStringMapsEqual(map[string]any{"A": "1"}, map[string]any{"A": "2"}))

	sched, envParsed, hasEnv, err := parseTemplateScheduleAndEnv([]byte("schedule_expression: '0 0 * * *'\nenvironment_variables:\n  FOO: bar\n"))
	assert.NoError(t, err)
	assert.Equal(t, "0 0 * * *", sched)
	assert.True(t, hasEnv)
	assert.Equal(t, "bar", envParsed["FOO"])

	_, err = readTemplateData("/nonexistent/dir", "tmpl.yaml")
	assert.Error(t, err)

	// Dashboard helpers
	assert.Equal(t, "Excellent", getPCSStatus(90.0))
	assert.Equal(t, "Good", getPCSStatus(75.0))
	assert.Equal(t, "Fair", getPCSStatus(50.0))
	assert.Equal(t, "Poor", getPCSStatus(20.0))

	_, _ = getHistoricalMetric("pcs")
	_, _ = getHistoricalMetric("unknown_metric")

	// Codegen commands
	assert.NotNil(t, NewGeneratePipelineOutcomeKeysCmd())
	assert.NotNil(t, NewGenerateAPIBuildersCmd())
}
