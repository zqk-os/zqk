package grep

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/search/interceptor"
	"gopkg.in/yaml.v3"
)

func TestGrepROI_EmptyProject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	cmd := NewGrepCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--roi", "--path", root})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --roi on empty project to succeed, got: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"zgrep Pilot ROI Scorecard",
		"Total Search Invocations:      0",
		"zgrep Compliance Ratio:        0.0%",
		"Estimated Tokens Saved:        0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestGrepROI_ScorecardFormats(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	events := []interceptor.ComplianceEvent{
		{
			Kind:                 interceptor.ComplianceKindZgrep,
			Compliant:            true,
			CommandLine:          "zqk grep --ast --kind struct Engine",
			EstimatedTokensSaved: 6000,
		},
		{
			Kind:                 interceptor.ComplianceKindZgrep,
			Compliant:            true,
			CommandLine:          "zgrep 'SearchResult' pkg/search",
			EstimatedTokensSaved: 6000,
		},
		{
			Kind:                 interceptor.ComplianceKindRawSearchIntercepted,
			Compliant:            false,
			CommandLine:          "grep -rn TODO .",
			RewrittenCommand:     "zqk grep --regex \"TODO\" --max-tokens 2000 -f json",
			EstimatedTokensSaved: 6000,
		},
		{
			Kind:                 interceptor.ComplianceKindRawFindNudge,
			Compliant:            false,
			CommandLine:          "find . -name '*.ts'",
			EstimatedTokensSaved: 0,
		},
	}
	for _, ev := range events {
		if err := interceptor.RecordComplianceEvent(root, ev); err != nil {
			t.Fatalf("RecordComplianceEvent failed: %v", err)
		}
	}

	t.Run("json", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--roi", "-f", "json", root})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("json --roi failed: %v", err)
		}
		var got interceptor.ComplianceSummary
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("invalid json output: %v\n%s", err, buf.String())
		}
		if got.TotalSearchInvocations != 4 || got.ZgrepCompliantCount != 2 || got.RawSearchNonCompliantCount != 2 {
			t.Fatalf("unexpected counts: %+v", got)
		}
		if got.ComplianceRatioPct != 50.0 {
			t.Errorf("expected 50.0%% compliance ratio, got %.2f", got.ComplianceRatioPct)
		}
		if got.EstimatedTokensSaved != 18000 {
			t.Errorf("expected 18000 tokens saved, got %d", got.EstimatedTokensSaved)
		}
	})

	t.Run("yaml", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--roi", "-f", "yaml", "--path", root})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("yaml --roi failed: %v", err)
		}
		var got interceptor.ComplianceSummary
		if err := yaml.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("invalid yaml output: %v\n%s", err, buf.String())
		}
		if got.TotalSearchInvocations != 4 || got.EstimatedTokensSaved != 18000 {
			t.Fatalf("unexpected yaml summary: %+v", got)
		}
	})

	t.Run("text", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--roi", root})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("text --roi failed: %v", err)
		}
		out := buf.String()
		for _, want := range []string{
			"Total Search Invocations:      4",
			"zgrep Compliant Invocations:   2",
			"Raw Shell Search Intercepted:  2",
			"zgrep Compliance Ratio:        50.0%",
			"Estimated Tokens Saved:        18000",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected text output to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("markdown", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--roi", "-f", "markdown", root})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("markdown --roi failed: %v", err)
		}
		out := buf.String()
		for _, want := range []string{
			"## zgrep Pilot ROI Scorecard",
			"| Total Search Invocations | 4 |",
			"| **zgrep Compliance Ratio** | **50.0%** |",
			"| **Estimated Context Tokens Saved** | **18000** |",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected markdown output to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("min_compliance_pass", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--roi", "--min-compliance", "50", root})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected --min-compliance 50 to pass at 50.0%%, got: %v", err)
		}
	})

	t.Run("min_compliance_fail_closed", func(t *testing.T) {
		cmd := NewGrepCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--roi", "--min-compliance", "80", root})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "below required --min-compliance threshold 80.0%") {
			t.Fatalf("expected fail-closed threshold error, got: %v", err)
		}
	})

	t.Run("min_compliance_invalid_range", func(t *testing.T) {
		for _, bad := range []string{"-5", "105"} {
			cmd := NewGrepCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{"--roi", "--min-compliance", bad, root})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "must be between 0 and 100") {
				t.Fatalf("expected range validation error for %s, got: %v", bad, err)
			}
		}
	})
}
