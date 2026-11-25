package search

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEngine_RunSelfBench(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create enough matching lines that raw baseline exceeds a tight token budget.
	var content strings.Builder
	content.WriteString("package demo\n")
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&content, "// BenchmarkTarget line %03d with padding payload text to simulate verbose repository search output\n", i)
	}
	if err := fileutil.WriteFile(filepath.Join(root, "bench_target.go"), []byte(content.String()), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing fixture: %v", err)
	}

	engine := NewEngine(root)
	report, err := engine.RunSelfBench(context.Background(), SearchOptions{
		Query:     "BenchmarkTarget",
		MaxTokens: 300,
	})
	if err != nil {
		t.Fatalf("RunSelfBench failed: %v", err)
	}
	if report.BaselineMatches < 100 {
		t.Fatalf("expected >=100 baseline matches, got %d", report.BaselineMatches)
	}
	if report.BaselineEstimatedTokens <= report.ZgrepEstimatedTokens {
		t.Fatalf("expected budgeted zgrep tokens (%d) < baseline tokens (%d)", report.ZgrepEstimatedTokens, report.BaselineEstimatedTokens)
	}
	if report.TokenReductionPct <= 0 {
		t.Errorf("expected positive TokenReductionPct, got %.2f", report.TokenReductionPct)
	}

	table := FormatBenchTable(report, "zqk init --lite --with-mcp")
	for _, want := range []string{"zgrep Self-Benchmark", "Wall-Clock Latency", "Token Reduction:", "Next step: zqk init --lite --with-mcp"} {
		if !strings.Contains(table, want) {
			t.Errorf("expected bench table to contain %q, got:\n%s", want, table)
		}
	}
}
