package matrix

import (
	"context"
	"time"
)

// FileClass categorizes files into homogeneous evaluation groups.
type FileClass string

const (
	ClassSource     FileClass = "source"
	ClassTest       FileClass = "test"
	ClassScript     FileClass = "scripts"
	ClassConfigFile FileClass = "config"
	ClassDocs       FileClass = "docs"
	ClassOther      FileClass = "other"

	// Backwards-compatible aliases for Go projects
	ClassGoProd FileClass = ClassSource
	ClassGoTest FileClass = ClassTest
)

// CheckKind represents whether a check is pattern-based, external command, or conducted by an adversarial agent.
type CheckKind string

const (
	CheckKindPattern CheckKind = "pattern"
	CheckKindCommand CheckKind = "command"
	CheckKindAgent   CheckKind = "agent"

	// Backwards-compatible aliases
	CheckKindProgrammatic CheckKind = CheckKindPattern
	CheckKindAdversarial  CheckKind = CheckKindAgent
)

// CheckStatus represents the state of a verification check on a file.
type CheckStatus string

const (
	CheckStatusPending CheckStatus = "pending"
	CheckStatusPassed  CheckStatus = "passed"
	CheckStatusFailed  CheckStatus = "failed"
	CheckStatusSkipped CheckStatus = "skipped"
)

// DimensionCode represents standard evaluation dimensions across codebases.
type DimensionCode string

const (
	DimensionHCODE   DimensionCode = "HCODE"
	DimensionEFFPERF DimensionCode = "EFFPERF"
	DimensionERRHYG  DimensionCode = "ERRHYG"
	DimensionCONCURR DimensionCode = "CONCURR"
	DimensionSECOBS  DimensionCode = "SECOBS"
	DimensionDOCSIG  DimensionCode = "DOCSIG"
	DimensionLINT    DimensionCode = "LINT"
)

// DimensionSpec defines metadata and policy bindings for an evaluation dimension.
type DimensionSpec struct {
	Code        DimensionCode `json:"code" yaml:"code"`
	Name        string        `json:"name" yaml:"name"`
	Category    string        `json:"category,omitempty" yaml:"category,omitempty"`
	PolicyID    string        `json:"policy_id,omitempty" yaml:"policy_id,omitempty"`
	Description string        `json:"description" yaml:"description"`
	Severity    string        `json:"severity" yaml:"severity"`
}

// DefaultDimensions returns universal evaluation dimension specifications.
func DefaultDimensions() []DimensionSpec {
	return []DimensionSpec{
		{
			Code:        DimensionHCODE,
			Name:        "Hardcoded Logic and Literal Eradication",
			Category:    "maintainability",
			PolicyID:    "POL-CODE-HARDCODING-001",
			Description: "Zero hardcoded paths, versions, raw permissions, ports, and duplicate string literals.",
			Severity:    "critical",
		},
		{
			Code:        DimensionEFFPERF,
			Name:        "Efficiency, Complexity, and Resource Hygiene",
			Category:    "performance",
			PolicyID:    "POL-CODE-EFFPERF-001",
			Description: "Zero unbounded O(N^2) loops, bounded buffer allocations, and resource limits.",
			Severity:    "high",
		},
		{
			Code:        DimensionERRHYG,
			Name:        "Error Hygiene, Wrapping, and Fail-Closed Resilience",
			Category:    "reliability",
			PolicyID:    "POL-CODE-ERRHYG-001",
			Description: "Zero swallowed errors, structured error wrapping, and fail-closed safety.",
			Severity:    "high",
		},
		{
			Code:        DimensionCONCURR,
			Name:        "Concurrency Lifecycle and Resource Safety",
			Category:    "concurrency",
			PolicyID:    "POL-CODE-CONCURRENCY-001",
			Description: "Zero leaked background workers, explicit lifecycle management, and clean shutdown.",
			Severity:    "critical",
		},
		{
			Code:        DimensionSECOBS,
			Name:        "Zero-PII Security and Structured Observability",
			Category:    "security",
			PolicyID:    "POL-CODE-SECOBS-001",
			Description: "Zero hardcoded credentials, zero PII in logs, and structured audit logs.",
			Severity:    "critical",
		},
		{
			Code:        DimensionDOCSIG,
			Name:        "API Signatures, Documentation, and Contracts",
			Category:    "documentation",
			PolicyID:    "POL-CODE-DOCSIG-001",
			Description: "100% exported API documentation coverage and contract synchronization.",
			Severity:    "medium",
		},
	}
}

// DiamondScore represents the 1-5 diamond quality score assigned by the policy engine.
type DiamondScore int

const (
	ScoreUnrated  DiamondScore = 0
	ScoreCritical DiamondScore = 1 // 1 diamond: critical violation present
	ScoreFailing  DiamondScore = 2 // 2 diamonds: high violation present
	ScoreBacklog  DiamondScore = 3 // 3 diamonds: non-blocking warnings, blocking for release
	ScoreMinor    DiamondScore = 4 // 4 diamonds: minor notices, passing
	ScoreFlawless DiamondScore = 5 // 5 diamonds: zero violations, exemplary
)

// String returns human-readable diamond notation.
func (s DiamondScore) String() string {
	switch s {
	case ScoreFlawless:
		return "◆◆◆◆◆ (5/5 Flawless)"
	case ScoreMinor:
		return "◆◆◆◆◇ (4/5 Minor Polish)"
	case ScoreBacklog:
		return "◆◆◆◇◇ (3/5 Remediation Needed)"
	case ScoreFailing:
		return "◆◆◇◇◇ (2/5 Failing)"
	case ScoreCritical:
		return "◆◇◇◇◇ (1/5 Critical Failure)"
	default:
		return "◇◇◇◇◇ (Unrated)"
	}
}

// Finding represents a discrete violation or observation within a file.
type Finding struct {
	Line         int    `json:"line,omitempty"`
	Column       int    `json:"column,omitempty"`
	RuleID       string `json:"rule_id"`
	Message      string `json:"message"`
	Severity     string `json:"severity"` // "critical", "error", "warning", "info"
	Category     string `json:"category,omitempty"`
	LiteralValue string `json:"literal_value,omitempty"`
	Count        int    `json:"count,omitempty"`
}

// CheckResult represents the outcome of a check evaluation on a specific file content hash.
type CheckResult struct {
	CheckID      string        `json:"check_id"`
	Dimension    DimensionCode `json:"dimension,omitempty"`
	Status       CheckStatus   `json:"status"`
	DiamondScore DiamondScore  `json:"diamond_score,omitempty"`
	Evaluator    string        `json:"evaluator"`
	EvaluatedAt  time.Time     `json:"evaluated_at"`
	Feedback     string        `json:"feedback,omitempty"`
	Findings     []Finding     `json:"findings,omitempty"`
}

// StampRequest specifies parameters for programmatically stamping a file check.
type StampRequest struct {
	Path         string        `json:"path"`
	CheckID      string        `json:"check_id"`
	Dimension    DimensionCode `json:"dimension,omitempty"`
	Status       CheckStatus   `json:"status"`
	ContentHash  string        `json:"content_hash,omitempty"`
	DiamondScore DiamondScore  `json:"diamond_score,omitempty"`
	Evaluator    string        `json:"evaluator"`
	Feedback     string        `json:"feedback,omitempty"`
	Findings     []Finding     `json:"findings,omitempty"`
}

// FileEntry represents a repository file tracked by its content-addressed SHA-256 hash.
type FileEntry struct {
	Path        string                 `json:"path"`
	ContentHash string                 `json:"content_hash"`
	Class       FileClass              `json:"class"`
	Size        int64                  `json:"size"`
	ModTime     time.Time              `json:"mod_time"`
	Checks      map[string]CheckResult `json:"checks"`
}

// ClassConfig defines rules and required verification checks for a class of files.
type ClassConfig struct {
	Name            FileClass `json:"name" yaml:"name"`
	Description     string    `json:"description" yaml:"description"`
	Extensions      []string  `json:"extensions" yaml:"extensions"`
	MatchPatterns   []string  `json:"match_patterns,omitempty" yaml:"match_patterns,omitempty"`
	ExcludePatterns []string  `json:"exclude_patterns,omitempty" yaml:"exclude_patterns,omitempty"`
	RequiredChecks  []string  `json:"required_checks" yaml:"required_checks"`
}

// Profile defines declarative file classification rules, glob matchers, and required checks.
type Profile struct {
	Name        string                    `json:"name" yaml:"name"`
	Description string                    `json:"description,omitempty" yaml:"description,omitempty"`
	Classes     map[FileClass]ClassConfig `json:"classes" yaml:"classes"`
}

// LedgerRecord represents an individual content-addressed verification record tracking file status.
type LedgerRecord struct {
	FilePath  string        `json:"file_path"`
	SHA256    string        `json:"sha256"`
	Dimension DimensionCode `json:"dimension"`
	Status    CheckStatus   `json:"status"`
	StampedBy string        `json:"stamped_by"`
}

// MatrixLedger is the root persistent schema for the verification matrix.
type MatrixLedger struct {
	SchemaVersion string                    `json:"schema_version"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Files         map[string]FileEntry      `json:"files"`
	Classes       map[FileClass]ClassConfig `json:"classes"`
}

// CheckRunner executes a verification check against a file entry.
type CheckRunner interface {
	Run(ctx context.Context, repoRoot string, entry *FileEntry) (CheckResult, error)
}

// CheckDefinition defines metadata and execution logic for a verification check.
type CheckDefinition struct {
	ID            string
	Name          string
	Description   string
	Kind          CheckKind
	TargetClasses []FileClass
	Runner        CheckRunner
}

// RemediationHook provides an interface for external systems (e.g., ticket trackers, issue managers, or orchestrators)
// to automatically act upon policy failures and create remediation items.
type RemediationHook interface {
	OnPolicyFailure(ctx context.Context, sc *FileScorecard, failures []string) (*RemediationBlock, error)
}
