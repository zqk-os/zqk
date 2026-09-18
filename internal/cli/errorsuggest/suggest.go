// Package errorsuggest provides a shared service for CLI error understanding and
// actionable suggestions (BLI-659, [REDACTED-ID]).
// The service can be used across all commands and adapts output to verbosity
// and user experience level.
package errorsuggest

import (
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

const emptyValue = ""

// ExperienceLevel indicates how much detail to show (beginner = more guidance, expert = terse).
type ExperienceLevel string

const (
	ExperienceBeginner ExperienceLevel = "beginner"
	ExperienceStandard ExperienceLevel = "standard"
	ExperienceExpert   ExperienceLevel = "expert"
)

// Options control how suggestions are generated.
type Options struct {
	Verbose         bool            // If true, include full error and extra hints
	ExperienceLevel ExperienceLevel // Affects terse vs verbose suggestion text
	CommandPath     string          // e.g. "object update" for context-aware hints
}

// Suggestion holds the suggested user-facing message and optional hint.
type Suggestion struct {
	Message string // Primary message (always present)
	Hint    string // Optional actionable hint (e.g. "Try: zqk object get BLI-001")
}

// Service produces actionable error suggestions from raw errors.
type Service interface {
	// Suggest returns a user-facing message and optional hint for the given error.
	// Callers can pass Options to control verbosity and experience level.
	Suggest(err error, opts Options) Suggestion
}

// defaultService is the built-in implementation used when no service is registered.
type defaultService struct{}

// Suggest implements Service using pattern matching and simple heuristics.
func (s *defaultService) Suggest(err error, opts Options) Suggestion {
	if err == nil {
		return Suggestion{}
	}
	msg := err.Error()

	// Common patterns: not found, invalid, permission, typo-like
	out := Suggestion{Message: msg}

	msgLower := strings.ToLower(msg)
	switch {
	case strings.Contains(msgLower, "project root not found") || strings.Contains(msgLower, "no project root found") || strings.Contains(msgLower, "not a zqk project"):
		out.Hint = suggestProjectRootNotFound(opts)
	case strings.Contains(msgLower, "not found") || strings.Contains(msgLower, "no such"):
		out.Hint = suggestNotFound(opts)
	case strings.Contains(msgLower, "invalid") || strings.Contains(msgLower, "validation"):
		out.Hint = suggestValidation(opts)
	case strings.Contains(msgLower, "permission") || strings.Contains(msgLower, "denied") || strings.Contains(msgLower, "not allowed"):
		out.Hint = suggestPermission(opts)
	case strings.Contains(msgLower, "unknown command") || strings.Contains(msgLower, "unknown flag"):
		out.Hint = suggestUnknownCommandOrFlag(msg, opts)
	case strings.Contains(msgLower, "required") || strings.Contains(msgLower, "missing"):
		out.Hint = suggestRequired(opts)
	default:
		if opts.Verbose {
			out.Hint = "Use --verbose for full error details."
		}
	}

	// Experience level: expert gets terse hints, beginner gets fuller guidance
	if opts.ExperienceLevel == ExperienceExpert && out.Hint != emptyValue && !opts.Verbose {
		out.Hint = truncateHint(out.Hint, 80)
	}

	return out
}

func suggestProjectRootNotFound(opts Options) string {
	if opts.ExperienceLevel == ExperienceBeginner || opts.Verbose {
		return "Project root not found. Run 'zqk system init --project-name <name>' to initialize, navigate to a directory containing .zqk, or specify --project-root."
	}
	return "Project root not found. Run 'zqk system init' to initialize, or pass --project-root."
}

func suggestNotFound(opts Options) string {
	if opts.ExperienceLevel == ExperienceBeginner || opts.Verbose {
		return "Check the object ID or path. Use list to see available items (e.g. zqk object list <kind>)."
	}
	return "Check ID or path; use list to see available items."
}

func suggestValidation(opts Options) string {
	if opts.ExperienceLevel == ExperienceBeginner || opts.Verbose {
		return "Check field names and allowed values. Use --dry-run to validate without applying. For batch or agentic workflows, consider using --relaxed to defer strict reference checks."
	}
	return "Check field names and values; try --dry-run or --relaxed."
}

func suggestPermission(opts Options) string {
	if opts.ExperienceLevel == ExperienceBeginner || opts.Verbose {
		return "This operation is not allowed for your role. Check context and permissions."
	}
	return "Operation not allowed for current context."
}

// commonFlagSuggestions maps mistaken flags to the intended flag (community discoverability).
var commonFlagSuggestions = map[string]string{
	"--field": "--fields",
	"-field":  "--fields",
}

func suggestUnknownCommandOrFlag(errMsg string, opts Options) string {
	if flagName := extractUnknownFlagName(errMsg); flagName != emptyValue {
		if better, ok := commonFlagSuggestions[flagName]; ok {
			return fmt.Sprintf("Did you mean %s instead of %s? Run --help for valid flags.", better, flagName)
		}
	}
	if opts.CommandPath != emptyValue && strings.Contains(opts.CommandPath, "object") {
		return "Check spelling. For projections use --fields; to read one object use object get|show. Run --help."
	}
	if opts.ExperienceLevel == ExperienceBeginner || opts.Verbose {
		return "Check spelling. Run with --help for valid commands and flags."
	}
	return "Check spelling; run --help for valid options."
}

// extractUnknownFlagName parses cobra-style "unknown flag: --field".
func extractUnknownFlagName(errMsg string) string {
	const marker = "unknown flag:"
	lower := strings.ToLower(errMsg)
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return emptyValue
	}
	rest := strings.TrimSpace(errMsg[idx+len(marker):])
	if rest == emptyValue {
		return emptyValue
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return emptyValue
	}
	return strings.TrimRight(fields[0], ".,;:")
}

func suggestRequired(opts Options) string {
	if opts.ExperienceLevel == ExperienceBeginner || opts.Verbose {
		return "Provide the required argument or flag. Use --help for usage."
	}
	return "Provide required argument or flag; see --help."
}

func truncateHint(hint string, maxLen int) string {
	if len(hint) <= maxLen {
		return hint
	}
	return hint[:maxLen-3] + "..."
}

var defaultSvc Service = &defaultService{}

// Default returns the default suggestion service (never nil).
func Default() Service {
	if defaultSvc == nil {
		defaultSvc = &defaultService{}
	}
	return defaultSvc
}

// SetDefault sets the service used by Default(). Pass nil to reset to built-in.
func SetDefault(svc Service) {
	if svc == nil {
		defaultSvc = &defaultService{}
		return
	}
	defaultSvc = svc
}

// Suggest is a convenience that calls Default().Suggest(err, opts).
func Suggest(err error, opts Options) Suggestion {
	return Default().Suggest(err, opts)
}

// Format formats an error with optional suggestion hint into a single string.
// If opts.Verbose is true, the full error is preserved; otherwise a concise message is used.
func Format(err error, opts Options) string {
	if err == nil {
		return ""
	}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		return s.Message
	}
	return fmt.Sprintf("%s\n  %s", s.Message, s.Hint)
}

// ExperienceFromProfile maps CLI context profile to experience level.
func ExperienceFromProfile(profile string) ExperienceLevel {
	switch strings.ToLower(profile) {
	case string(pkgctx.ProfileAIAgent), string(pkgctx.ProfileMCP), string(pkgctx.ProfileSystem), string(pkgctx.ProfileDebug):
		return ExperienceExpert
	case "quiet":
		return ExperienceExpert
	case string(pkgctx.ProfileHuman), "":
		return ExperienceStandard
	default:
		return ExperienceStandard
	}
}

// ErrSuggestion can be used to wrap an error with a suggestion in error chains (reserved for future use).
var ErrSuggestion = errfmt.Errorf("suggestion")
var _ error = ErrSuggestion
