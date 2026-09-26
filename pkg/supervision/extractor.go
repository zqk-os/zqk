package supervision

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// Matches standard file:line:col or file:line error anchors across languages
	fileAnchorPattern = regexp.MustCompile(`([a-zA-Z0-9_\-\./\\]+\.[a-zA-Z0-9]+):(\d+)(?::(\d+))?`)
	// Matches unit test failures across common runners (Go, pytest, jest, etc.)
	testFailPattern = regexp.MustCompile(`(?:--- FAIL:|\bFAIL:|\bFAILED\b|AssertionError:)\s*([A-Za-z0-9_\-]+)`)
	// Matches panics or unhandled exceptions
	panicPattern = regexp.MustCompile(`(?:panic:|Exception:|Error:)\s*(.+)`)
)

// ExtractFailureAnchor locates the primary source file, test function, or symbol associated with the failure.
func ExtractFailureAnchor(stderr, stdout string) string {
	combined := stderr + "\n" + stdout

	// Check for test failure symbol first
	if m := testFailPattern.FindStringSubmatch(combined); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}

	// Check for panic or exception message
	if m := panicPattern.FindStringSubmatch(combined); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}

	// Check for file:line anchor
	if m := fileAnchorPattern.FindStringSubmatch(combined); len(m) > 1 {
		return strings.TrimSpace(m[0])
	}

	return ""
}

// ExtractFocusedDiagnostic extracts the high-signal error lines from raw stderr and stdout,
// filtering out noisy build banners, repeated warnings, or voluminous traces.
func ExtractFocusedDiagnostic(stderr, stdout string, exitCode int, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 30
	}

	raw := strings.TrimSpace(stderr)
	if raw == "" {
		raw = strings.TrimSpace(stdout)
	}
	if raw == "" {
		return fmt.Sprintf("Process exited with status code %d without diagnostic stderr/stdout output.", exitCode)
	}

	lines := strings.Split(raw, "\n")

	// Filter lines for signal
	var signalLines []string
	captureContext := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect high-value error markers
		isSignal := strings.Contains(line, "FAIL") ||
			strings.Contains(line, "Error") ||
			strings.Contains(line, "error:") ||
			strings.Contains(line, "panic:") ||
			strings.Contains(line, "undefined:") ||
			strings.Contains(line, "cannot use") ||
			strings.Contains(line, "not found") ||
			strings.Contains(line, "expected") ||
			strings.Contains(line, "validation errors") ||
			fileAnchorPattern.MatchString(line)

		if isSignal {
			signalLines = append(signalLines, line)
			captureContext = 3 // capture immediate follow-up lines for trace context
		} else if captureContext > 0 {
			signalLines = append(signalLines, line)
			captureContext--
		}
	}

	// If signal filtering yielded too little, fall back to the tail of the output
	if len(signalLines) < 3 {
		start := 0
		if len(lines) > maxLines {
			start = len(lines) - maxLines
		}
		return strings.Join(lines[start:], "\n")
	}

	// Cap signal lines to maxLines
	if len(signalLines) > maxLines {
		signalLines = signalLines[len(signalLines)-maxLines:]
	}

	return strings.Join(signalLines, "\n")
}

// BuildFailureDiagnostic packages execution telemetry into a structured FailureDiagnostic.
func BuildFailureDiagnostic(attempt, exitCode int, phase FailingPhase, stderr, stdout string, anomalyDesc string) FailureDiagnostic {
	focused := ExtractFocusedDiagnostic(stderr, stdout, exitCode, 35)
	anchor := ExtractFailureAnchor(stderr, stdout)

	if phase == "" {
		if exitCode == 0 {
			phase = PhaseValidation
		} else {
			phase = PhaseExecution
		}
	}

	return FailureDiagnostic{
		Attempt:           attempt,
		ExitCode:          exitCode,
		Phase:             phase,
		FailureAnchor:     anchor,
		RawStderr:         stderr,
		RawStdout:         stdout,
		FocusedDiagnostic: focused,
		ObservedAnomaly:   anomalyDesc,
		Timestamp:         time.Now().UTC(),
	}
}

// SynthesizeNegativeHypothesis derives anti-thrashing negative boundaries from a previous failure.
func SynthesizeNegativeHypothesis(attempt int, diag *FailureDiagnostic, triedApproach string) NegativeHypothesis {
	hyp := NegativeHypothesis{
		Attempt: attempt,
	}

	if triedApproach != "" {
		hyp.HypothesisSummary = fmt.Sprintf("Approach in attempt %d was invalidated.", attempt)
		hyp.InvalidatedApproach = triedApproach
	} else if diag != nil && diag.FailureAnchor != "" {
		hyp.HypothesisSummary = fmt.Sprintf("Hypothesis around %s failed verification.", diag.FailureAnchor)
		hyp.InvalidatedApproach = fmt.Sprintf("Prior implementation at %s resulted in error during %s phase.", diag.FailureAnchor, diag.Phase)
	} else {
		hyp.HypothesisSummary = fmt.Sprintf("Prior attempt %d failed verification.", attempt)
		hyp.InvalidatedApproach = "The preceding implementation strategy did not satisfy the verification contract."
	}

	// Add practical anti-thrashing constraints
	if diag != nil {
		if diag.ExitCode != 0 {
			hyp.ForbiddenActions = append(hyp.ForbiddenActions,
				fmt.Sprintf("Do not repeat the exact same implementation that triggered exit code %d.", diag.ExitCode))
		}
		if diag.FailureAnchor != "" {
			hyp.ForbiddenActions = append(hyp.ForbiddenActions,
				fmt.Sprintf("Verify modifications to %s with targeted tests before final acceptance.", diag.FailureAnchor))
		}
	}

	return hyp
}
