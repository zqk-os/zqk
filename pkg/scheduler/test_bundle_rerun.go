package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/zqktime"
)

const goModulePathPrefix = "github.com/zqk-os/zqk/"

// testBundleRunDelimiter is written before each scheduler retry of the same job stdout/stderr (see handlers_run_wrapper_execution).
// When the same file accumulates multiple runs, ParseGoTestOutput must see only the last run — otherwise --- FAIL lines from
// earlier runs make health.jsonl report test_fail even when the latest go test passed.
const testBundleRunDelimiter = "\n--- run "

var (
	goTestTimeoutRe = regexp.MustCompile(`-timeout\s+(\d+)s`)
	// Strip absolute log paths so fingerprint groups runs of the "same" bundle across machines/sessions.
	logPathStripRe = regexp.MustCompile(`[^\s]*/\.zqk/logs/[^\s]+`)
)

// ExtractBundleLogPath returns the log path from the command string, or empty string.
func ExtractBundleLogPath(cmdStr string) string {
	if m := logPathStripRe.FindString(cmdStr); m != "" {
		return m
	}
	return ""
}

// ResolveBundleLogPath returns the log path from the command string, or falls back to standard scheduler test-bundles log layout for the job.
func ResolveBundleLogPath(projectRoot, jobID, cmdStr string) string {
	if m := ExtractBundleLogPath(cmdStr); m != "" {
		return m
	}
	if projectRoot != "" && strings.HasPrefix(jobID, "SCH-run-") {
		suffix := strings.TrimPrefix(jobID, "SCH-run-")
		candidates := []string{
			filepath.Join(projectRoot, ".zqk", "logs", "scheduler", "cvs", "test-bundles", "bundle-"+suffix+".log"),
			filepath.Join(projectRoot, ".zqk", "logs", "scheduler", "cvs", "test-bundles", suffix+".log"),
			filepath.Join(projectRoot, ".zqk", "logs", "scheduler", "cvs", "test-bundles", jobID+".log"),
		}
		for _, c := range candidates {
			if _, err := fileutil.Stat(c); err == nil {
				return c
			}
		}
	}
	return ""
}

// FingerprintBundleCommand returns a short hex id for the bundle command line (paths to log files normalized).
func FingerprintBundleCommand(cmdStr string) string {
	norm := strings.TrimSpace(cmdStr)
	norm = logPathStripRe.ReplaceAllString(norm, "<log>")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:12])
}

// FormatRunWrapperCommandString builds the same command string run_wrapper uses for logging and fingerprints
// when the job is not a multi-line shell script (command + args joined with spaces).
func FormatRunWrapperCommandString(command string, args []string) string {
	if len(args) > 0 {
		return command + " " + strings.Join(args, " ")
	}
	return command
}

// extractGoTestPackageArg returns the first ./... package argument from a go test invocation, or "".
func extractGoTestPackageArg(command string, commandArgs []string) string {
	rel := circuitbreaker.ExtractPackagePathFromRunWrapperCommand(command, commandArgs)
	if rel == emptyValue {
		return ""
	}
	return "./" + rel
}

func parseGoTestTimeoutSeconds(cmdStr string, fallback int) int {
	if m := goTestTimeoutRe.FindStringSubmatch(cmdStr); len(m) == 2 {
		var sec int
		_, _ = fmt.Sscanf(m[1], "%d", &sec)
		if sec > 0 {
			// Slightly under job max so go test exits before SIGKILL (same spirit as test_failures_rerun).
			if sec > 120 {
				return sec - 60
			}
			return sec
		}
	}
	if fallback <= 0 {
		return 300
	}
	return fallback
}

// splitFailedTestName returns a repo-relative package path (e.g. cmd/zqk) and test function name.
// defaultPkgRel is from the go test ./... argument (without leading ./).
func splitFailedTestName(full string, defaultPkgRel string) (pkgRel string, testFunc string) {
	s := strings.TrimSpace(full)
	if s == emptyValue {
		return defaultPkgRel, ""
	}
	s = strings.TrimPrefix(s, goModulePathPrefix)
	idx := strings.LastIndex(s, ".")
	if idx <= 0 {
		return defaultPkgRel, s
	}
	left, right := s[:idx], s[idx+1:]
	// left is either a package path (contains /) or a single segment (ambiguous).
	if strings.Contains(left, "/") {
		return left, right
	}
	// Bare "pkg.TestName" with no slash — prefer default package from command line.
	if defaultPkgRel != emptyValue {
		return defaultPkgRel, right
	}
	return left, right
}

// BuildSuggestedGoTestRerunCommands returns one go test command per package group, re-running only failed tests.
// failedTestNames should match go test failure lines / parsers (package.TestName or TestName).
func BuildSuggestedGoTestRerunCommands(cmdStr string, command string, commandArgs []string, failedTestNames []string, jobMaxRuntimeSeconds int) []string {
	if len(failedTestNames) == 0 {
		return nil
	}
	defaultPkgArg := extractGoTestPackageArg(command, commandArgs)
	defaultPkgRel := strings.TrimPrefix(defaultPkgArg, "./")

	timeout := parseGoTestTimeoutSeconds(cmdStr, jobMaxRuntimeSeconds)

	byPkg := make(map[string][]string)
	for _, name := range failedTestNames {
		pkgRel, fn := splitFailedTestName(name, defaultPkgRel)
		if fn == emptyValue {
			continue
		}
		if pkgRel == emptyValue {
			pkgRel = defaultPkgRel
		}
		if pkgRel == emptyValue {
			continue
		}
		byPkg[pkgRel] = append(byPkg[pkgRel], fn)
	}
	if len(byPkg) == 0 {
		return nil
	}

	pkgs := make([]string, 0, len(byPkg))
	for p := range byPkg {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	out := make([]string, 0, len(pkgs))
	for _, pkgRel := range pkgs {
		names := byPkg[pkgRel]
		sort.Strings(names)
		var pat string
		if len(names) == 1 {
			pat = "^" + regexp.QuoteMeta(names[0]) + "$"
		} else {
			quoted := make([]string, 0, len(names))
			for _, n := range names {
				quoted = append(quoted, regexp.QuoteMeta(n))
			}
			pat = "^(" + strings.Join(quoted, "|") + ")$"
		}
		line := fmt.Sprintf("go test ./%s -run '%s' -v -count=1 -timeout %ds", pkgRel, pat, timeout)
		out = append(out, line)
	}
	return out
}

// BuildTestBundleHealthEntry returns one JSON object for health.jsonl (best-effort; never nil keys for required fields).
func BuildTestBundleHealthEntry(jobID, lineEventType, outcome string, testsFailed int, fingerprint string, suggested []string, packageDir string, logPath string, failedTestNames []string) map[string]any {
	e := map[string]any{
		KeyTimestamp:                zqktime.NowRFC3339UTC(),
		KeyJobID:                    jobID,
		KeyEventType:                lineEventType,
		KeyTestOutcome:              outcome,
		"tests_failed":              testsFailed,
		KeyBundleCommandFingerprint: fingerprint,
	}
	if len(suggested) > 0 {
		e[KeySuggestedRerunCommands] = suggested
	}
	if len(failedTestNames) > 0 {
		e[KeyTestFailures] = failedTestNames
	}
	if packageDir != "" {
		e["package"] = packageDir
	}
	if logPath != "" {
		e["log_path"] = logPath
	}
	return e
}

// substantiveGoTestRunBlockAfterDelimiter reports whether a slice that begins with "--- run ... ---"
// (same shape as s[idx+1:] after splitting on testBundleRunDelimiter) contains real go test output.
// Appended bundle logs sometimes end with duplicate empty "--- run RFC3339 ---" trailers; the last
// such marker must not win over the previous block that actually contains FAIL/PASS lines, or
// health.jsonl will show tests_failed=0 while go test exited 1.
func substantiveGoTestRunBlockAfterDelimiter(block string) bool {
	b := strings.TrimSpace(block)
	if b == emptyValue {
		return false
	}
	// Drop the first line ("--- run ... ---" header).
	firstNL := strings.IndexByte(b, '\n')
	if firstNL < 0 {
		return false
	}
	body := strings.TrimSpace(b[firstNL+1:])
	if body == emptyValue {
		return false
	}
	return strings.Contains(body, "=== RUN") || strings.Contains(body, "=== PAUSE") ||
		strings.Contains(body, "=== CONT") || strings.Contains(body, "--- FAIL:") ||
		strings.Contains(body, "--- PASS:") || strings.Contains(body, "FAIL\t") ||
		strings.Contains(body, "ok  \t") || strings.Contains(body, "ok\t")
}

// trimToLastSchedulerTestRunForParsing keeps only the last substantive "--- run ... ---" block so test output parsers are not confused
// by earlier runs in the same appended log file. Empty trailing run markers are skipped.
func trimToLastSchedulerTestRunForParsing(s string) string {
	if s == emptyValue {
		return s
	}
	parts := strings.Split(s, testBundleRunDelimiter)
	if len(parts) <= 1 {
		return s
	}
	for i := len(parts) - 1; i >= 1; i-- {
		block := "--- run " + parts[i]
		if substantiveGoTestRunBlockAfterDelimiter(block) {
			return block
		}
	}
	if strings.TrimSpace(parts[0]) != emptyValue {
		return strings.TrimSpace(parts[0])
	}
	return s
}

// gatherTestOutputForParsing returns stdout+stderr text suitable for ParseGoTestOutput (file tail or ring preview).
func gatherTestOutputForParsing(streamedToFiles bool, projectRoot, jobID string, stdoutWriter *streamingOutputWriter, stderrWriter *streamingOutputWriter) string {
	if streamedToFiles && projectRoot != emptyValue && jobID != emptyValue {
		stdoutPath := JobStdoutFilePath(projectRoot, jobID)
		stderrPath := JobStderrFilePath(projectRoot, jobID)
		const perStream = maxBytesToReadForTestParse / 2
		var testOutput string
		if b, _ := readLastBytesFromFile(stdoutPath, perStream); len(b) > 0 {
			testOutput = string(b)
		}
		if b, _ := readLastBytesFromFile(stderrPath, perStream); len(b) > 0 {
			if testOutput != emptyValue {
				testOutput += "\n" + string(b)
			} else {
				testOutput = string(b)
			}
		}
		if testOutput != emptyValue {
			return trimToLastSchedulerTestRunForParsing(testOutput)
		}
	}
	if stdoutWriter != nil {
		if s := stdoutWriter.Preview(); s != emptyValue {
			return trimToLastSchedulerTestRunForParsing(s)
		}
	}
	if stderrWriter != nil {
		return trimToLastSchedulerTestRunForParsing(stderrWriter.Preview())
	}
	return ""
}
