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

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/drifthotspots"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// ensureDriftHotspotsFlags binds flags expected by runAnalyzeDriftHotspots if not present.
func ensureDriftHotspotsFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("root") == nil {
		cmd.Flags().String("root", "", "root directory to scan")
	}
	if cmd.Flags().Lookup("specs-dir") == nil {
		cmd.Flags().String("specs-dir", "", "specs directory")
	}
	if cmd.Flags().Lookup("include-tests") == nil {
		cmd.Flags().Bool("include-tests", false, "include tests")
	}
	if cmd.Flags().Lookup("min-risk") == nil {
		cmd.Flags().String("min-risk", "", "minimum risk")
	}
	if cmd.Flags().Lookup("extra-kinds") == nil {
		cmd.Flags().String("extra-kinds", "", "extra kinds")
	}
	if cmd.Flags().Lookup("include-constant-barrels") == nil {
		cmd.Flags().Bool("include-constant-barrels", false, "include constant barrels")
	}
	if cmd.Flags().Lookup("summary-only") == nil {
		cmd.Flags().Bool("summary-only", false, "summary only")
	}
	if cmd.Flags().Lookup("details") == nil {
		cmd.Flags().Bool("details", false, "details")
	}
	if cmd.Flags().Lookup("top") == nil {
		cmd.Flags().Int("top", 25, "top count")
	}
}

// setupCrit3CommandContext attaches output buffers and CLI context to a command.
func setupCrit3CommandContext(t *testing.T, cmd *cobra.Command, projectRoot string) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	t.Cleanup(cancel)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, buf))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, projectRoot)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		manager := cli.NewContextManager()
		minimalCtx, err := manager.LoadContext(initCtx)
		if err != nil {
			t.Fatalf("Failed to create minimal context: %v", err)
		}
		cliCtx = cli.ContextFromInner(minimalCtx)
	}
	cli.SetContext(cmd, cliCtx)
	return buf
}

// TestNegativeAdversarialBoundary_AggregateAudit_CRIT_1793632807358587000_53046cae tests
// fault isolation, adversarial flag combinations, context cancellations, payload validation,
// and error boundaries in aggregate_audit_pipeline.go.
func TestNegativeAdversarialBoundary_AggregateAudit_CRIT_1793632807358587000_53046cae(t *testing.T) {
	t.Run("NilCommand_FaultIsolation", func(t *testing.T) {
		err := RunAggregateAuditViaPipeline(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cmd required")
	})

	t.Run("MissingContext_FaultIsolation", func(t *testing.T) {
		cmd := NewAggregateAuditCmd()
		cli.SetContext(cmd, nil)
		err := RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context")
	})

	t.Run("UnresolvableProjectRoot_Boundary", func(t *testing.T) {
		emptyDir := t.TempDir()
		t.Setenv(zqkenv.ProjectRoot().Name(), "")
		t.Setenv(zqkenv.TestRoot().Name(), "")
		t.Chdir(emptyDir)

		cmd := NewAggregateAuditCmd()
		cli.SetContext(cmd, cli.ContextForProjectAndProfile("", "test"))
		err := RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "project root not found")
	})

	t.Run("InvalidCPUProfilePath_Boundary", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		invalidProfilePath := filepath.Join(proj.Root, "nonexistent_dir_987654", "sub", "test.pprof")
		err := cmd.Flags().Set("cpu-profile", invalidProfilePath)
		require.NoError(t, err)

		err = RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create CPU profile")
	})

	t.Run("MalformedTimeWindow_InvalidDuration", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		err := cmd.Flags().Set("window", "invalid_window_duration")
		require.NoError(t, err)

		err = RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse time window")
		assert.Contains(t, err.Error(), "invalid window format")
	})

	t.Run("MalformedTimeWindow_InvalidStartFormat", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		err := cmd.Flags().Set("start", "corrupted-date-not-rfc3339")
		require.NoError(t, err)

		err = RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse time window")
		assert.Contains(t, err.Error(), "invalid start time format")
	})

	t.Run("MalformedTimeWindow_InvalidEndFormat", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		err := cmd.Flags().Set("start", "2026-01-01T00:00:00Z")
		require.NoError(t, err)
		err = cmd.Flags().Set("end", "corrupted-end-not-rfc3339")
		require.NoError(t, err)

		err = RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse time window")
		assert.Contains(t, err.Error(), "invalid end time format")
	})

	t.Run("AdversarialCleanupDecision_MissingArchiveOrDelete", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		// cleanup-metric requires either --archive or --delete
		err := cmd.Flags().Set("cleanup-metric", "AAM-MALFORMED-999")
		require.NoError(t, err)

		err = RunAggregateAuditViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cleanup-metric requires --archive or --delete")
	})

	t.Run("PayloadDecoding_FaultIsolation", func(t *testing.T) {
		// Test nil payload
		p1, ok1 := aggregateAuditPayloadFrom(nil, false)
		assert.False(t, ok1)
		assert.Nil(t, p1)

		p2, ok2 := aggregateAuditPayloadFrom(nil, true)
		assert.False(t, ok2)
		assert.Nil(t, p2)

		// Test wrong concrete type
		p3, ok3 := aggregateAuditPayloadFrom("malformed_payload_string", false)
		assert.False(t, ok3)
		assert.Nil(t, p3)

		p4, ok4 := aggregateAuditPayloadFrom(12345, true)
		assert.False(t, ok4)
		assert.Nil(t, p4)

		// Test aggregateAuditPipelinePayload with nil aggCtx
		payloadWithNilCtx := &aggregateAuditPipelinePayload{aggCtx: nil}
		p5, ok5 := aggregateAuditPayloadFrom(payloadWithNilCtx, true)
		assert.False(t, ok5)
		assert.Nil(t, p5)

		p6, ok6 := aggregateAuditPayloadFrom(payloadWithNilCtx, false)
		assert.True(t, ok6)
		assert.Equal(t, payloadWithNilCtx, p6)

		// Test valid payload with aggCtx
		validPayload := &aggregateAuditPipelinePayload{aggCtx: &AggregateAuditContext{}}
		p7, ok7 := aggregateAuditPayloadFrom(validPayload, true)
		assert.True(t, ok7)
		assert.Equal(t, validPayload, p7)
	})

	t.Run("CancelledContext_DeadlockPrevention", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		cmd.SetContext(cancelledCtx)

		done := make(chan error, 1)
		goroutinelabels.NewGoroutine("test_audit_pipeline", "run audit pipeline").StartSimple(func() {
			done <- RunAggregateAuditViaPipeline(cmd, nil)
		})

		select {
		case err := <-done:
			require.Error(t, err)
			assert.Equal(t, context.Canceled, err)
		case <-time.After(3 * time.Second):
			t.Fatal("Deadlock detected: RunAggregateAuditViaPipeline stalled on cancelled context")
		}
	})

	t.Run("FullPipeline_IsolatedProject_ZeroEventsAndCleanupModes", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)

		// 1. Run without cleanup flags (mode: none)
		cmd1 := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd1, proj.Root)
		err := RunAggregateAuditViaPipeline(cmd1, nil)
		require.NoError(t, err)

		// 2. Run with --archive flag
		cmd2 := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd2, proj.Root)
		err = cmd2.Flags().Set("archive", "true")
		require.NoError(t, err)
		err = RunAggregateAuditViaPipeline(cmd2, nil)
		require.NoError(t, err)

		// 3. Run with --delete flag
		cmd3 := NewAggregateAuditCmd()
		setupCrit3CommandContext(t, cmd3, proj.Root)
		err = cmd3.Flags().Set("delete", "true")
		require.NoError(t, err)
		err = RunAggregateAuditViaPipeline(cmd3, nil)
		require.NoError(t, err)
	})
}

// TestNegativeAdversarialBoundary_AggregateChangeJournal_CRIT_1793632807358587000_53046cae tests
// fault isolation, adversarial flags, unresolvable storage root, cancelled contexts, and payload
// integrity in aggregate_change_journal_pipeline.go.
func TestNegativeAdversarialBoundary_AggregateChangeJournal_CRIT_1793632807358587000_53046cae(t *testing.T) {
	t.Run("NilCommand_FaultIsolation", func(t *testing.T) {
		err := RunAggregateChangeJournalViaPipeline(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aggregate-change-journal: cmd required")
	})

	t.Run("MissingContext_FaultIsolation", func(t *testing.T) {
		cmd := NewAggregateChangeJournalCmd()
		cli.SetContext(cmd, nil)
		err := RunAggregateChangeJournalViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aggregate-change-journal: failed to get context")
	})

	t.Run("UnresolvableProjectRoot_Boundary", func(t *testing.T) {
		emptyDir := t.TempDir()
		t.Setenv(zqkenv.ProjectRoot().Name(), "")
		t.Setenv(zqkenv.TestRoot().Name(), "")
		t.Chdir(emptyDir)

		cmd := NewAggregateChangeJournalCmd()
		cli.SetContext(cmd, cli.ContextForProjectAndProfile("", "test"))
		err := RunAggregateChangeJournalViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aggregate-change-journal: project root not found")
	})

	t.Run("MalformedTimeWindow_InvalidStart", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateChangeJournalCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		err := cmd.Flags().Set("start", "corrupted-date-value")
		require.NoError(t, err)

		err = RunAggregateChangeJournalViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aggregate-change-journal: failed to parse time window")
	})

	t.Run("MalformedTimeWindow_InvalidWindow", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateChangeJournalCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		err := cmd.Flags().Set("window", "invalid_duration")
		require.NoError(t, err)

		err = RunAggregateChangeJournalViaPipeline(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aggregate-change-journal: failed to parse time window")
	})

	t.Run("CancelledContext_DeadlockPrevention", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAggregateChangeJournalCmd()
		setupCrit3CommandContext(t, cmd, proj.Root)

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		cmd.SetContext(cancelledCtx)

		done := make(chan error, 1)
		goroutinelabels.NewGoroutine("test_change_journal_pipeline", "run change journal pipeline").StartSimple(func() {
			done <- RunAggregateChangeJournalViaPipeline(cmd, nil)
		})

		select {
		case err := <-done:
			require.Error(t, err)
			assert.Equal(t, context.Canceled, err)
		case <-time.After(3 * time.Second):
			t.Fatal("Deadlock detected: RunAggregateChangeJournalViaPipeline stalled on cancelled context")
		}
	})

	t.Run("PayloadDecoding_FaultIsolation", func(t *testing.T) {
		// nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload]
		p1, ok1 := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload](nil)
		assert.False(t, ok1)
		assert.Nil(t, p1)

		p2, ok2 := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload]("invalid_string")
		assert.False(t, ok2)
		assert.Nil(t, p2)

		p3, ok3 := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload](struct{}{})
		assert.False(t, ok3)
		assert.Nil(t, p3)

		valid := &aggregateChangeJournalPipelinePayload{projectRoot: "/tmp/root"}
		p4, ok4 := nildecode.DecodeNonNilPayload[*aggregateChangeJournalPipelinePayload](valid)
		assert.True(t, ok4)
		assert.Equal(t, valid, p4)
	})

	t.Run("FullPipeline_IsolatedProject_ZeroEntriesAndCleanups", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)

		// 1. Standard execution without delete
		cmd1 := NewAggregateChangeJournalCmd()
		setupCrit3CommandContext(t, cmd1, proj.Root)
		err := RunAggregateChangeJournalViaPipeline(cmd1, nil)
		require.NoError(t, err)

		// 2. Execution with --delete flag (safe on 0 entries)
		cmd2 := NewAggregateChangeJournalCmd()
		setupCrit3CommandContext(t, cmd2, proj.Root)
		err = cmd2.Flags().Set("delete", "true")
		require.NoError(t, err)
		err = RunAggregateChangeJournalViaPipeline(cmd2, nil)
		require.NoError(t, err)
	})
}

// TestNegativeAdversarialBoundary_AnalyzeDriftHotspots_CRIT_1793632807358587000_53046cae tests
// project root discovery fallbacks, missing/corrupted specs directories, AST unparseable Go sources,
// format matrix resilience, and output formatting boundaries in analyze_drift_hotspots.go.
func TestNegativeAdversarialBoundary_AnalyzeDriftHotspots_CRIT_1793632807358587000_53046cae(t *testing.T) {
	t.Run("MissingContext_FaultIsolation", func(t *testing.T) {
		cmd := NewAnalyzeDriftHotspotsCmd()
		cli.SetContext(cmd, nil)
		err := runAnalyzeDriftHotspots(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context")
	})

	t.Run("UnresolvableProjectRoot_Boundary", func(t *testing.T) {
		emptyDir := t.TempDir()
		t.Setenv(zqkenv.ProjectRoot().Name(), "")
		t.Setenv(zqkenv.TestRoot().Name(), "")
		t.Chdir(emptyDir)

		cmd := NewAnalyzeDriftHotspotsCmd()
		ensureDriftHotspotsFlags(cmd)
		cli.SetContext(cmd, cli.ContextForProjectAndProfile("", "test"))

		err := runAnalyzeDriftHotspots(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not resolve project root")
	})

	t.Run("NonExistentSpecsDirectory_Boundary", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		cmd := NewAnalyzeDriftHotspotsCmd()
		ensureDriftHotspotsFlags(cmd)
		setupCrit3CommandContext(t, cmd, proj.Root)

		nonExistentSpecs := filepath.Join(proj.Root, "nonexistent_specs_dir_54321")
		err := cmd.Flags().Set("specs-dir", nonExistentSpecs)
		require.NoError(t, err)
		err = cmd.Flags().Set("root", proj.Root)
		require.NoError(t, err)

		err = runAnalyzeDriftHotspots(cmd, nil)
		require.Error(t, err)
	})

	t.Run("CorruptedSpecsFile_MalformedYAML_Boundary", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		badSpecsDir := filepath.Join(proj.Root, "bad_specs")
		err := fileutil.MkdirAll(badSpecsDir, paths.DirPerm755)
		require.NoError(t, err)

		// Write malformed YAML file into specs directory
		badSpecPath := filepath.Join(badSpecsDir, "broken_spec.yaml")
		err = fileutil.WriteFile(badSpecPath, []byte("schema_version: 2.0.0\nontology: [unclosed_yaml_syntax"), paths.FilePerm644)
		require.NoError(t, err)

		cmd := NewAnalyzeDriftHotspotsCmd()
		ensureDriftHotspotsFlags(cmd)
		setupCrit3CommandContext(t, cmd, proj.Root)

		err = cmd.Flags().Set("specs-dir", badSpecsDir)
		require.NoError(t, err)
		err = cmd.Flags().Set("root", proj.Root)
		require.NoError(t, err)

		err = runAnalyzeDriftHotspots(cmd, nil)
		require.Error(t, err)
	})

	t.Run("TargetSourceResilience_MalformedGoSyntax", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		srcDir := filepath.Join(proj.Root, "src")
		err := fileutil.MkdirAll(srcDir, paths.DirPerm755)
		require.NoError(t, err)

		// Create a Go file with completely broken syntax that fails go/parser
		malformedGoFile := filepath.Join(srcDir, "unparseable.go")
		err = fileutil.WriteFile(malformedGoFile, []byte("package broken\nfunc unclosed( { syntax error !!!"), paths.FilePerm644)
		require.NoError(t, err)

		// Create a valid Go file alongside it
		validGoFile := filepath.Join(srcDir, "valid.go")
		err = fileutil.WriteFile(validGoFile, []byte("package valid\nfunc Hello() string { return \"world\" }"), paths.FilePerm644)
		require.NoError(t, err)

		// Create a valid dummy specs directory with ontology field so loadKindSetForAnalyze succeeds
		specsDir := filepath.Join(proj.Root, paths.ProcessInternalObjectSpecsDir)
		err = fileutil.MkdirAll(specsDir, paths.DirPerm755)
		require.NoError(t, err)
		specFile := filepath.Join(specsDir, "sample_spec.yaml")
		err = fileutil.WriteFile(specFile, []byte("ontology: sample_kind\nschema_version: \"2.0.0\"\n"), paths.FilePerm644)
		require.NoError(t, err)

		cmd := NewAnalyzeDriftHotspotsCmd()
		ensureDriftHotspotsFlags(cmd)
		buf := setupCrit3CommandContext(t, cmd, proj.Root)

		err = cmd.Flags().Set("root", srcDir)
		require.NoError(t, err)
		err = cmd.Flags().Set("specs-dir", specsDir)
		require.NoError(t, err)

		// The analyzer must gracefully skip the unparseable file without crashing or panicking
		err = runAnalyzeDriftHotspots(cmd, nil)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Drift hotspot scan")
	})

	t.Run("OutputFormatting_MatrixCoverage", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, nil)
		specsDir := filepath.Join(proj.Root, paths.ProcessInternalObjectSpecsDir)
		err := fileutil.MkdirAll(specsDir, paths.DirPerm755)
		require.NoError(t, err)
		err = fileutil.WriteFile(filepath.Join(specsDir, "sample.yaml"), []byte("ontology: sample\nschema_version: \"2.0.0\"\n"), paths.FilePerm644)
		require.NoError(t, err)

		srcDir := filepath.Join(proj.Root, "pkg")
		err = fileutil.MkdirAll(srcDir, paths.DirPerm755)
		require.NoError(t, err)
		err = fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte("package sample\nconst kind = \"sample\"\n"), paths.FilePerm644)
		require.NoError(t, err)

		formats := []string{"json", "yaml", "jsonl", "text"}
		for _, fmtType := range formats {
			cmd := NewAnalyzeDriftHotspotsCmd()
			ensureDriftHotspotsFlags(cmd)
			buf := setupCrit3CommandContext(t, cmd, proj.Root)

			_ = cmd.Flags().Set("root", srcDir)
			_ = cmd.Flags().Set("specs-dir", specsDir)
			_ = cmd.Flags().Set("format", fmtType)
			_ = cmd.Flags().Set("extra-kinds", "goal,requirement,,,custom_kind")
			_ = cmd.Flags().Set("include-tests", "true")
			_ = cmd.Flags().Set("include-constant-barrels", "true")
			_ = cmd.Flags().Set("min-risk", "critical")

			err := runAnalyzeDriftHotspots(cmd, nil)
			require.NoError(t, err, "Format %s should succeed", fmtType)
			assert.NotEmpty(t, buf.String())
		}
	})

	t.Run("WriteDriftHotspotsText_ComprehensiveProof", func(t *testing.T) {
		cmd := NewAnalyzeDriftHotspotsCmd()
		ensureDriftHotspotsFlags(cmd)
		buf := setupCrit3CommandContext(t, cmd, "/mock/root")

		report := drifthotspots.Report{
			RootDir:                "/mock/root",
			SpecsDir:               "/mock/specs",
			IncludeTests:           true,
			IncludeConstantBarrels: true,
			MinRiskFloor:           "low",
			CountByRisk: map[string]int{
				drifthotspots.RiskCritical: 5,
				drifthotspots.RiskHigh:     3,
				drifthotspots.RiskMedium:   2,
				drifthotspots.RiskLow:      1,
			},
			CountByCategory: map[string]int{
				drifthotspots.CategoryKindLiteralCompare:   4,
				drifthotspots.CategoryKindLiteralAssign:    3,
				drifthotspots.CategorySystemFieldKey:       2,
				drifthotspots.CategorySchemaVersionLiteral: 2,
			},
			FileSummaries: []drifthotspots.FileSummary{
				{File: "a.go", Count: 6},
				{File: "b.go", Count: 3},
				{File: "c.go", Count: 2},
			},
			Findings: []drifthotspots.Finding{
				{
					File:     "a.go",
					Line:     10,
					Column:   5,
					Risk:     drifthotspots.RiskCritical,
					Category: drifthotspots.CategoryKindLiteralCompare,
					Literal:  "\"goal\"",
					Hint:     "use objects.KindGoal constant",
				},
				{
					File:     "b.go",
					Line:     22,
					Column:   8,
					Risk:     drifthotspots.RiskHigh,
					Category: drifthotspots.CategorySystemFieldKey,
					Literal:  "\"id\"",
					Hint:     "use objects.FieldKeyID constant",
				},
			},
		}

		// 1. Text with summary-only
		_ = cmd.Flags().Set("summary-only", "true")
		_ = cmd.Flags().Set("top", "2")
		err := writeDriftHotspotsText(cmd, report)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Drift hotspot scan")
		assert.Contains(t, buf.String(), "By risk: critical=5, high=3, medium=2, low=1")
		assert.Contains(t, buf.String(), "Top files by finding count (showing 2 of 3):")

		// 2. Text without details (omitted message)
		buf.Reset()
		_ = cmd.Flags().Set("summary-only", "false")
		_ = cmd.Flags().Set("details", "false")
		_ = cmd.Flags().Set("top", "-1") // Resets to topN 25
		err = writeDriftHotspotsText(cmd, report)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Omitted 2 line-level findings. Re-run with --details")

		// 3. Text with details (line findings listed)
		buf.Reset()
		_ = cmd.Flags().Set("details", "true")
		err = writeDriftHotspotsText(cmd, report)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "a.go:10:5 [critical] kind_literal_compare \"\\\"goal\\\"\"")
		assert.Contains(t, buf.String(), "use objects.KindGoal constant")
	})
}
