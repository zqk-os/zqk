package grep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGrepBench_TableAndJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	var content strings.Builder
	content.WriteString("package benchdemo\n")
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&content, "// SelfBenchSymbol line %03d with realistic codebase context for token reduction measurement\n", i)
	}
	if err := fileutil.WriteFile(filepath.Join(root, "demo.go"), []byte(content.String()), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing fixture: %v", err)
	}

	t.Run("json", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--bench", "SelfBenchSymbol", root, "--max-tokens", "300", "-f", "json"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("zqk grep --bench -f json failed: %v", err)
		}
		var report search.BenchReport
		if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
			t.Fatalf("invalid json output: %v\n%s", err, buf.String())
		}
		if report.BaselineMatches < 100 || report.TokenReductionPct <= 0 {
			t.Fatalf("unexpected bench report: %+v", report)
		}
	})

	t.Run("table", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--bench", "SelfBenchSymbol", root, "--max-tokens", "300"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("zqk grep --bench table failed: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"zgrep Self-Benchmark", "Wall-Clock Latency", "Token Reduction:", "init --lite --with-mcp --with-hooks"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected table output to contain %q, got:\n%s", want, out)
			}
		}
	})

	for _, format := range []string{"markdown", "md"} {
		t.Run("format_"+format, func(t *testing.T) {
			cmd := NewGrepCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{"--bench", "SelfBenchSymbol", root, "--max-tokens", "300", "-f", format})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("zqk grep --bench -f %s failed: %v", format, err)
			}
			out := buf.String()
			for _, want := range []string{"## zgrep Self-Benchmark", "| Metric | Raw Baseline | zgrep (Budgeted) |", "**Token Reduction:**", "init --lite --with-mcp --with-hooks"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected markdown output to contain %q, got:\n%s", want, out)
				}
			}
		})
	}
}

func TestGrepReindex_JSONStats(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(root, "app.ts"), []byte("export class ReindexTarget {}\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing fixture: %v", err)
	}

	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--reindex", "--path", root, "-f", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("zqk grep --reindex -f json failed: %v", err)
	}
	var stats search.IndexBuildStats
	if err := json.Unmarshal(buf.Bytes(), &stats); err != nil {
		t.Fatalf("invalid json stats: %v\n%s", err, buf.String())
	}
	if stats.IndexedFiles != 1 || stats.IndexBytes <= 0 {
		t.Fatalf("unexpected reindex stats: %+v", stats)
	}
}
