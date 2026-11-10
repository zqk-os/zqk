package coverage

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Invariant 1: Static Floor Invariant (CRIT-1794286015698376000-7951dee4)
// Verify shard coverprofile metadata schemas, package mapping dictionaries, and aggregation thresholds satisfy immutable structural invariants.
func TestMatrixCoverage_StaticFloorInvariants(t *testing.T) {
	t.Run("ProfileBlockSchemaAndFormatting", func(t *testing.T) {
		block := &ProfileBlock{
			Path:      "pkg/consensus/ledger.go",
			StartLine: 10,
			StartCol:  1,
			EndLine:   15,
			EndCol:    20,
			NumStmt:   3,
			Count:     5,
		}

		wantKey := "pkg/consensus/ledger.go:10.1,15.20"
		if block.Key() != wantKey {
			t.Fatalf("expected key %q, got %q", wantKey, block.Key())
		}

		wantFormat := "pkg/consensus/ledger.go:10.1,15.20 3 5"
		if block.Format() != wantFormat {
			t.Fatalf("expected format %q, got %q", wantFormat, block.Format())
		}

		if err := block.Validate(false); err != nil {
			t.Fatalf("expected valid block, got error: %v", err)
		}
	})

	t.Run("LineParserValidatesStructure", func(t *testing.T) {
		m := NewMerger()
		line := "github.com/zqk-os/zqk/pkg/storage/store.go:42.10,48.2 4 12"
		b, err := m.ParseLine(line)
		if err != nil {
			t.Fatalf("unexpected error parsing line: %v", err)
		}
		if b == nil {
			t.Fatal("expected non-nil parsed block")
		}
		if b.Path != "github.com/zqk-os/zqk/pkg/storage/store.go" {
			t.Fatalf("unexpected path: %s", b.Path)
		}
		if b.StartLine != 42 || b.StartCol != 10 || b.EndLine != 48 || b.EndCol != 2 {
			t.Fatalf("unexpected coordinates: %d.%d,%d.%d", b.StartLine, b.StartCol, b.EndLine, b.EndCol)
		}
		if b.NumStmt != 4 {
			t.Fatalf("unexpected numStmt: %d", b.NumStmt)
		}
		if b.Count != 12 {
			t.Fatalf("unexpected count: %d", b.Count)
		}
	})

	t.Run("ModeHeaderValidation", func(t *testing.T) {
		m := NewMerger()
		validModes := []string{ModeSet, ModeCount, ModeAtomic}
		for _, mode := range validModes {
			content := "mode: " + mode + "\npkg/a/a.go:1.1,2.1 1 1\n"
			parsedMode, blocks, err := m.ParseProfile(strings.NewReader(content))
			if err != nil {
				t.Fatalf("mode %s failed: %v", mode, err)
			}
			if parsedMode != mode {
				t.Fatalf("expected parsed mode %q, got %q", mode, parsedMode)
			}
			if len(blocks) != 1 {
				t.Fatalf("expected 1 block for mode %s, got %d", mode, len(blocks))
			}
		}
	})
}

// Invariant 2: Operational Dynamic Proof (CRIT-1794286018797248000-f65600e3)
// Demonstrate atomic merging of multi-shard coverprofile blocks, accurate statement count deduplication, and validated JSON report generation.
func TestMatrixCoverage_OperationalDynamicProof(t *testing.T) {
	t.Run("MultiShardDeduplicationAndAtomicCountSum", testOperationalProof_Deduplication)
	t.Run("SevenShardMatrixSimulation", testOperationalProof_SevenShards)
	t.Run("FileMergingAndArtifactWriting", testOperationalProof_FileMerging)
}

func testOperationalProof_Deduplication(t *testing.T) {
	shard1 := []byte("mode: atomic\n" +
		"github.com/zqk-os/zqk/pkg/shared/util.go:10.1,12.5 2 3\n" +
		"github.com/zqk-os/zqk/pkg/shard1/a.go:5.1,8.10 4 1\n")
	shard2 := []byte("mode: atomic\n" +
		"github.com/zqk-os/zqk/pkg/shared/util.go:10.1,12.5 2 7\n" +
		"github.com/zqk-os/zqk/pkg/shard2/b.go:20.1,25.4 6 2\n")
	shard3 := []byte("mode: atomic\n" +
		"github.com/zqk-os/zqk/pkg/shared/util.go:10.1,12.5 2 2\n" +
		"github.com/zqk-os/zqk/pkg/shard3/c.go:30.1,35.4 3 0\n")

	m := NewMerger()
	merged, err := m.MergeShards(shard1, shard2, shard3)
	if err != nil {
		t.Fatalf("unexpected error merging shards: %v", err)
	}

	if merged.BlockCount() != 4 {
		t.Fatalf("expected 4 deduplicated blocks, got %d", merged.BlockCount())
	}

	totalStmts, coveredStmts := merged.TotalStatements()
	if totalStmts != 15 {
		t.Fatalf("expected 15 total statements, got %d (check for duplicate counting)", totalStmts)
	}
	if coveredStmts != 12 {
		t.Fatalf("expected 12 covered statements, got %d", coveredStmts)
	}

	var sharedBlock *ProfileBlock
	for _, b := range merged.SortedBlocks() {
		if strings.Contains(b.Path, "shared/util.go") {
			sharedBlock = b
			break
		}
	}
	if sharedBlock == nil {
		t.Fatal("shared block not found in merged profile")
	}
	if sharedBlock.Count != 12 {
		t.Fatalf("expected accumulated count 12, got %d", sharedBlock.Count)
	}

	formatted := merged.Format()
	if !strings.HasPrefix(string(formatted), "mode: atomic\n") {
		t.Fatalf("expected formatted output to begin with 'mode: atomic\\n', got: %s", string(formatted[:20]))
	}
}

func testOperationalProof_SevenShards(t *testing.T) {
	shards := make([][]byte, 7)
	shardNames := []string{"core-kernel", "storage", "cli", "objects", "agent", "validation", "ai-tools"}
	for i, name := range shardNames {
		shards[i] = []byte(strings.Join([]string{
			"mode: atomic",
			"github.com/zqk-os/zqk/pkg/common/base.go:1.1,5.1 3 1",
			"github.com/zqk-os/zqk/pkg/" + name + "/file.go:10.1,20.1 5 10",
		}, "\n") + "\n")
	}

	m := NewMerger()
	merged, err := m.MergeShards(shards...)
	if err != nil {
		t.Fatalf("unexpected error merging 7 shards: %v", err)
	}

	if merged.BlockCount() != 8 {
		t.Fatalf("expected 8 blocks, got %d", merged.BlockCount())
	}
	total, covered := merged.TotalStatements()
	if total != 38 {
		t.Fatalf("expected 38 statements, got %d", total)
	}
	if covered != 38 {
		t.Fatalf("expected 38 covered statements, got %d", covered)
	}

	report, err := merged.Rollup(80.0)
	if err != nil {
		t.Fatalf("failed to synthesize rollup: %v", err)
	}
	if report.TotalStatements != 38 || report.CoveredStatements != 38 || report.CoveragePct != 100.0 {
		t.Fatalf("unexpected rollup metrics: %+v", report)
	}
	if !report.MeetsThreshold {
		t.Fatal("expected report to meet threshold 80.0%")
	}
}

func testOperationalProof_FileMerging(t *testing.T) {
	tempDir := t.TempDir()
	shard1Path := filepath.Join(tempDir, "coverage-1.out")
	shard2Path := filepath.Join(tempDir, "coverage-2.out")

	if err := fileutil.WriteFile(shard1Path, []byte("mode: atomic\ngithub.com/zqk-os/zqk/pkg/foo/foo.go:1.1,5.1 2 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(shard2Path, []byte("mode: atomic\ngithub.com/zqk-os/zqk/pkg/bar/bar.go:1.1,5.1 4 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := NewMerger()
	merged, err := m.MergeGlob(filepath.Join(tempDir, "coverage-*.out"))
	if err != nil {
		t.Fatalf("MergeGlob failed: %v", err)
	}
	if merged.BlockCount() != 2 {
		t.Fatalf("expected 2 blocks, got %d", merged.BlockCount())
	}

	report, err := merged.Rollup(85.0)
	if err != nil {
		t.Fatal(err)
	}

	artifactPath, checksumPath, err := community.WriteVerifiedCoverageArtifact(tempDir, report)
	if err != nil {
		t.Fatalf("failed to write coverage artifact: %v", err)
	}

	verified, err := community.VerifyCoverageArtifact(artifactPath)
	if err != nil {
		t.Fatalf("artifact verification failed: %v", err)
	}
	if !verified.Verified {
		t.Fatal("expected verified coverage artifact")
	}
	if !filepath.IsAbs(checksumPath) {
		t.Fatalf("expected absolute checksum path: %s", checksumPath)
	}
}

// Invariant 3: Negative Adversarial Boundary (CRIT-1794286021883487000-9b1dd315)
// Enforce fail-closed rejection when coverprofile blocks have invalid syntax, missing shard payloads, conflicting statement counts, or paths outside project root.
func TestMatrixCoverage_NegativeAdversarialBoundaries(t *testing.T) {
	t.Run("RejectsEmptyShardSet", testNegativeBoundaries_EmptyShards)
	t.Run("RejectsMissingShardFile", testNegativeBoundaries_MissingFile)
	t.Run("RejectsMalformedBlockSyntax", testNegativeBoundaries_MalformedSyntax)
	t.Run("RejectsInconsistentCoverageModes", testNegativeBoundaries_InconsistentModes)
	t.Run("RejectsConflictingStatementCounts", testNegativeBoundaries_ConflictingCounts)
	t.Run("RejectsEscapingPathsAndTraversal", testNegativeBoundaries_EscapingPaths)
	t.Run("RejectsNilBlockOrProfile", testNegativeBoundaries_NilBlocks)
}

func testNegativeBoundaries_EmptyShards(t *testing.T) {
	m := NewMerger()
	if _, err := m.MergeShards(); !errors.Is(err, ErrEmptyShardSet) {
		t.Fatalf("expected ErrEmptyShardSet, got %v", err)
	}
	if _, err := m.MergeFiles(); !errors.Is(err, ErrEmptyShardSet) {
		t.Fatalf("expected ErrEmptyShardSet, got %v", err)
	}
}

func testNegativeBoundaries_MissingFile(t *testing.T) {
	m := NewMerger()
	if _, err := m.MergeFiles("non-existent-shard-file.out"); err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func testNegativeBoundaries_MalformedSyntax(t *testing.T) {
	m := NewMerger()
	malformedCases := []string{
		"pkg/a/a.go 1 1",
		"pkg/a/a.go:1.1 1 1",
		"pkg/a/a.go:x.y,z.w 1 1",
		"pkg/a/a.go:1.1,2.1 notanint 1",
		"pkg/a/a.go:1.1,2.1 1 notanint",
		"pkg/a/a.go:5.1,2.1 1 1",
		"pkg/a/a.go:1.1,2.1 -5 1",
		"pkg/a/a.go:1.1,2.1 1 -2",
	}

	for _, badLine := range malformedCases {
		if _, err := m.ParseLine(badLine); err == nil {
			t.Fatalf("expected rejection for malformed line %q, got nil", badLine)
		}
	}

	zeroStmtBlock, err := m.ParseLine("pkg/a/a.go:1.1,2.1 0 1")
	if err != nil {
		t.Fatalf("expected acceptance of zero-statement block, got %v", err)
	}
	if zeroStmtBlock.NumStmt != 0 {
		t.Fatalf("expected NumStmt=0, got %d", zeroStmtBlock.NumStmt)
	}
}

func testNegativeBoundaries_InconsistentModes(t *testing.T) {
	m := NewMerger()
	shardAtomic := []byte("mode: atomic\npkg/a/a.go:1.1,2.1 1 1\n")
	shardSet := []byte("mode: set\npkg/b/b.go:1.1,2.1 1 1\n")
	if _, err := m.MergeShards(shardAtomic, shardSet); !errors.Is(err, ErrInconsistentCoverageMode) {
		t.Fatalf("expected ErrInconsistentCoverageMode, got %v", err)
	}
}

func testNegativeBoundaries_ConflictingCounts(t *testing.T) {
	m := NewMerger()
	shardA := []byte("mode: atomic\npkg/a/a.go:1.1,5.1 4 1\n")
	shardB := []byte("mode: atomic\npkg/a/a.go:1.1,5.1 6 2\n")
	if _, err := m.MergeShards(shardA, shardB); !errors.Is(err, ErrConflictingStatementCount) {
		t.Fatalf("expected ErrConflictingStatementCount, got %v", err)
	}
}

func testNegativeBoundaries_EscapingPaths(t *testing.T) {
	m := NewMerger()
	traversalCases := []string{
		"../escape/evil.go:1.1,2.1 1 1",
		"pkg/../../evil.go:1.1,2.1 1 1",
		"/etc/passwd:1.1,2.1 1 1",
		`pkg\windows\backslash.go:1.1,2.1 1 1`,
	}
	for _, bad := range traversalCases {
		if _, err := m.ParseLine(bad); !errors.Is(err, ErrInvalidProfilePath) {
			t.Fatalf("expected ErrInvalidProfilePath for line %q, got %v", bad, err)
		}
	}
}

func testNegativeBoundaries_NilBlocks(t *testing.T) {
	var nilBlock *ProfileBlock
	if err := nilBlock.Validate(false); !errors.Is(err, ErrNilProfile) {
		t.Fatalf("expected ErrNilProfile, got %v", err)
	}

	p := NewMergedProfile(ModeAtomic)
	if err := p.AddBlock(nil); !errors.Is(err, ErrNilProfile) {
		t.Fatalf("expected ErrNilProfile on AddBlock(nil), got %v", err)
	}
}
