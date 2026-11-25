package interceptor

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// ComplianceKindZgrep marks a compliant zgrep / zqk grep invocation.
	ComplianceKindZgrep = "zgrep_compliant"
	// ComplianceKindRawSearchIntercepted marks a raw grep/rg/git-grep invocation redirected to zgrep.
	ComplianceKindRawSearchIntercepted = "raw_search_intercepted"
	// ComplianceKindRawFindNudge marks a repo-wide raw find invocation that triggered a steering nudge.
	ComplianceKindRawFindNudge = "raw_find_nudge"

	// DefaultEstimatedRawSearchTokens approximates the unbudgeted context bloat of raw shell grep/find.
	DefaultEstimatedRawSearchTokens = 8000
	// DefaultBudgetedSearchTokens is the typical token-budgeted zgrep response size.
	DefaultBudgetedSearchTokens = 2000

	complianceLogRelFile = "zgrep_compliance.jsonl"
)

var complianceWriteMu sync.Mutex

// ComplianceEvent records a single code-search compliance observation for ROI telemetry (GH-1220).
type ComplianceEvent struct {
	Timestamp            string `json:"timestamp"`
	Kind                 string `json:"kind"`
	Tool                 string `json:"tool"`
	Compliant            bool   `json:"compliant"`
	CommandLine          string `json:"command_line"`
	RewrittenCommand     string `json:"rewritten_command,omitempty"`
	SteeringNudge        string `json:"steering_nudge,omitempty"`
	EstimatedTokensSaved int    `json:"estimated_tokens_saved"`
}

// ComplianceSummary aggregates zgrep vs. raw-search compliance and token savings.
type ComplianceSummary struct {
	TotalSearchInvocations     int     `json:"total_search_invocations"`
	ZgrepCompliantCount        int     `json:"zgrep_compliant_count"`
	RawSearchNonCompliantCount int     `json:"raw_search_non_compliant_count"`
	ComplianceRatioPct         float64 `json:"compliance_ratio_pct"`
	EstimatedTokensSaved       int     `json:"estimated_tokens_saved"`
}

// DetectZgrepCompliant returns true when commandLine invokes zgrep or `<brand> grep`.
func DetectZgrepCompliant(commandLine string) bool {
	trimmed := strings.TrimSpace(commandLine)
	if trimmed == "" {
		return false
	}
	segments, err := splitPipelineSegments(trimmed)
	if err != nil {
		return false
	}
	for _, seg := range segments {
		exe, idx := extractExecutable(seg.Tokens)
		base := filepath.Base(exe)
		if base == "zgrep" {
			return true
		}
		if (base == DefaultCLICommand || strings.HasPrefix(base, "zqk")) && idx+1 < len(seg.Tokens) && seg.Tokens[idx+1] == GrepSubcommand {
			return true
		}
	}
	return false
}

// DetectRawFindNonCompliance detects repo-scoped `find` commands used for code discovery
// and returns a high-signal steering nudge with an equivalent token-budgeted zgrep command.
func DetectRawFindNonCompliance(commandLine, projectRoot string) (bool, string) {
	trimmed := strings.TrimSpace(commandLine)
	if trimmed == "" {
		return false, ""
	}
	segments, err := splitPipelineSegments(trimmed)
	if err != nil {
		return false, ""
	}
	for _, seg := range segments {
		exe, idx := extractExecutable(seg.Tokens)
		if filepath.Base(exe) != "find" {
			continue
		}
		args := seg.Tokens[idx+1:]
		if len(args) == 0 {
			return true, buildFindSteeringNudge(".")
		}
		target := "."
		for _, tok := range args {
			if strings.HasPrefix(tok, "-") || tok == "!" || tok == "(" {
				break
			}
			target = tok
			break
		}
		if isOutsideRepoTarget(target, projectRoot) {
			return false, ""
		}
		return true, buildFindSteeringNudge(target)
	}
	return false, ""
}

func isOutsideRepoTarget(target, projectRoot string) bool {
	if target == "" || target == "." || !filepath.IsAbs(target) {
		return strings.HasPrefix(target, "/tmp") || strings.HasPrefix(target, "/var") || strings.HasPrefix(target, "/private/tmp")
	}
	if strings.TrimSpace(projectRoot) == "" {
		return strings.HasPrefix(target, "/tmp") || strings.HasPrefix(target, "/var") || strings.HasPrefix(target, "/private/tmp")
	}
	absRoot, errRoot := filepath.Abs(projectRoot)
	absTarget, errTarget := filepath.Abs(target)
	if errRoot != nil || errTarget != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func buildFindSteeringNudge(target string) string {
	return paths.RewriteCanonicalCLIInvocations(
		"Non-compliance steering signal (GH-1220): raw `find` bypasses ZQK's trigram index and token budget. " +
			"Use `zqk grep \"<pattern>\" " + target + " --max-tokens 2000 -f json` (or `zgrep --ast --kind func " + target + "`) for sub-15ms bounded code discovery.",
	)
}

// FormatSteeringNudge returns a concise corrective nudge for intercepted raw search commands.
func FormatSteeringNudge(rewrittenCmd string) string {
	if strings.TrimSpace(rewrittenCmd) == "" {
		return paths.RewriteCanonicalCLIInvocations("Use `zqk grep \"<pattern>\" --max-tokens 2000 -f json` instead of raw shell grep/rg.")
	}
	return paths.RewriteCanonicalCLIInvocations(
		"Intercepted search (GH-1220): redirected to token-budgeted zqk grep trigram engine -> " + rewrittenCmd,
	)
}

// ComplianceLogPath returns the project-scoped JSONL telemetry path for zgrep compliance.
func ComplianceLogPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, complianceLogRelFile)
}

// RecordComplianceEvent appends a compliance event to `.zqk/logs/zgrep_compliance.jsonl`.
func RecordComplianceEvent(projectRoot string, ev ComplianceEvent) error {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	logPath := ComplianceLogPath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(logPath)); err != nil {
		return err
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	complianceWriteMu.Lock()
	defer complianceWriteMu.Unlock()

	return appendLineToFile(logPath, data)
}

func appendLineToFile(path string, line []byte) error {
	f, err := fileutil.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(line)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// SummarizeCompliance computes aggregate zgrep compliance ratio and estimated token savings.
func SummarizeCompliance(projectRoot string) (ComplianceSummary, error) {
	var summary ComplianceSummary
	if strings.TrimSpace(projectRoot) == "" {
		return summary, nil
	}
	logPath := ComplianceLogPath(projectRoot)
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return summary, nil
		}
		return summary, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev ComplianceEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		summary.TotalSearchInvocations++
		if ev.Compliant {
			summary.ZgrepCompliantCount++
		} else {
			summary.RawSearchNonCompliantCount++
		}
		summary.EstimatedTokensSaved += ev.EstimatedTokensSaved
	}
	if err := scanner.Err(); err != nil {
		return summary, err
	}
	if summary.TotalSearchInvocations > 0 {
		summary.ComplianceRatioPct = float64(summary.ZgrepCompliantCount) * 100.0 / float64(summary.TotalSearchInvocations)
	}
	return summary, nil
}
