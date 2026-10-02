package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	defaultTargetedCheckTimeout = 3 * time.Minute
	defaultKindCheckTimeout     = 10 * time.Minute
	defaultFastCheckTimeout     = 15 * time.Minute
	defaultFullCheckTimeout     = 30 * time.Minute
	defaultAutofixCheckTimeout  = 45 * time.Minute
	defaultMaxProgressiveBudget = 60 * time.Minute
)

// CalculateCheckProgressiveTimeout determines the progressive timeout for a check execution.
// If an explicit timeout is specified by the user or context, it is honored.
// Otherwise, it computes a progressive timeout based on command scope and flags.
func CalculateCheckProgressiveTimeout(cmd *cobra.Command, args []string) time.Duration {
	if cmd != nil {
		if explicit := cli.GetTimeout(cmd); explicit > 0 {
			return explicit
		}
		if cmd.Context() != nil {
			if deadline, ok := cmd.Context().Deadline(); ok {
				remaining := time.Until(deadline)
				if remaining > 0 {
					return remaining
				}
			}
		}
	}

	// Determine based on flags
	var autoFix, force, fast bool
	var idsFile string
	if cmd != nil {
		autoFix, _ = cmd.Flags().GetBool("auto-fix")
		force, _ = cmd.Flags().GetBool("force")
		fast, _ = cmd.Flags().GetBool("fast")
		idsFile, _ = cmd.Flags().GetString("ids-from-file")
	}

	if idsFile != "" {
		return defaultKindCheckTimeout
	}

	// Determine scope from positional args
	if len(args) > 0 && args[0] != "all" {
		firstArg := args[0]
		// If argument looks like an object ID (has hyphen e.g. BLI-123)
		if strings.Contains(firstArg, "-") || len(args) > 1 {
			return defaultTargetedCheckTimeout
		}
		// Single kind target (e.g. "backlog_item")
		return defaultKindCheckTimeout
	}

	// Full check (all objects)
	if autoFix || force {
		return defaultAutofixCheckTimeout
	}
	if fast {
		return defaultFastCheckTimeout
	}
	return defaultFullCheckTimeout
}

// FormatCheckTimeoutError constructs actionable diagnostic guidance when a system check times out.
func FormatCheckTimeoutError(cmd *cobra.Command, args []string, timeout time.Duration) error {
	cmdPattern := "all"
	if len(args) > 0 {
		cmdPattern = strings.Join(args, " ")
	}

	var fast, autoFix bool
	if cmd != nil {
		fast, _ = cmd.Flags().GetBool("fast")
		autoFix, _ = cmd.Flags().GetBool("auto-fix")
	}

	progressiveNext := timeout * 2
	if progressiveNext > defaultMaxProgressiveBudget {
		progressiveNext = defaultMaxProgressiveBudget
	}

	var suggestions []string

	// Suggest progressive timeout increment
	suggestions = append(suggestions, fmt.Sprintf(
		"• Progressive Timeout: Extend the execution budget using '--timeout %s' (or '--timeout 0' for auto-calculation).",
		progressiveNext.Round(time.Minute),
	))

	// Suggest fast mode if not already active
	if !fast {
		suggestions = append(suggestions,
			"• Fast Mode: Use '--fast' to skip reference graph verification for quick partial health checks.",
		)
	}

	// Suggest background execution for long or auto-fix operations
	suggestions = append(suggestions,
		"• Background Execution: Use '--background' to run detached via the coordinator without blocking the terminal.",
	)

	// Suggest scoping if checking all
	if len(args) == 0 || args[0] == "all" {
		suggestions = append(suggestions, paths.RewriteCanonicalCLIInvocations("• Scoped Validation: Narrow scope by checking a specific kind ('zqk system check <kind>') or object ID ('zqk system check <id>')."), "• Fast Iteration: Save failing IDs using '--write-failing-ids fail.txt --fast' and re-check with '--ids-from-file fail.txt --fast'.")
	}

	if autoFix {
		suggestions = append(suggestions,
			"• Autofix Partitioning: Full-project autofix performs intensive CAS reconciliation; run with '--timeout 60m --background'.",
		)
	}

	msg := paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("system check timed out after %v\n\nCommand: zqk system check %s\n\nActionable Guidance & Suggested Remedies:\n  %s", timeout,
		cmdPattern,
		strings.Join(suggestions, "\n  ")))

	return errfmt.Errorf("%s", msg)
}

// FormatCheckExecutionError formats runtime check errors with actionable remediation guidance.
func FormatCheckExecutionError(cmd *cobra.Command, args []string, err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	if strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "timeout waiting") {
		timeout := CalculateCheckProgressiveTimeout(cmd, args)
		return FormatCheckTimeoutError(cmd, args, timeout)
	}

	if strings.Contains(errStr, "CAP journal is stale") {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("%v\n\nActionable Guidance:\n  • The scheduler CAP daemon is inactive or stalled.\n  • Restart background daemons: zqk scheduler restart\n  • Verify scheduler status: zqk scheduler status", err)))
	}

	return err
}

// ValidateTargetKindOrSuggest checks if a non-ID argument is a valid kind or returns an error with suggestions.
func ValidateTargetKindOrSuggest(target string, allKinds []string) error {
	if target == "" || target == "all" {
		return nil
	}
	// If it contains a hyphen, it is treated as an object ID
	if strings.Contains(target, "-") {
		return nil
	}

	for _, k := range allKinds {
		if target == k {
			return nil
		}
	}

	suggestions := SuggestSimilarKinds(target, allKinds)
	var guidance string
	if len(suggestions) > 0 {
		guidance = fmt.Sprintf("\n\nDid you mean:\n  • %s", strings.Join(suggestions, "\n  • "))
	}

	return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("unknown object kind %q.%s\n\nRun 'zqk object list kinds' or 'zqk system check --help' for valid options.", target, guidance)))
}

// SuggestSimilarKinds finds closest kinds using edit distance and prefix matching.
func SuggestSimilarKinds(input string, allKinds []string) []string {
	inputLower := strings.ToLower(input)
	var matches []string

	// Check prefix / substring matches first
	for _, k := range allKinds {
		kLower := strings.ToLower(k)
		if strings.HasPrefix(kLower, inputLower) || strings.Contains(kLower, inputLower) {
			matches = append(matches, k)
		}
	}

	// Check Levenshtein distance
	for _, k := range allKinds {
		kLower := strings.ToLower(k)
		dist := computeLevenshtein(inputLower, kLower)
		if dist <= 3 && !sliceContainsString(matches, k) {
			matches = append(matches, k)
		}
	}

	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches
}

func computeLevenshtein(a, b string) int {
	return strutil.LevenshteinDistance(a, b)
}

func sliceContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
