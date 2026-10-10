package matrix

import (
	"context"
	"time"
)

// FileClass categorizes files into homogeneous evaluation groups.
type FileClass string

const (
	ClassGoProd     FileClass = "go-production"
	ClassGoTest     FileClass = "go-test"
	ClassScript     FileClass = "scripts"
	ClassConfigFile FileClass = "config"
	ClassDocs       FileClass = "docs"
	ClassOther      FileClass = "other"
)

// CheckKind represents whether a check is programmatic or conducted by an adversarial agent.
type CheckKind string

const (
	CheckKindProgrammatic CheckKind = "programmatic"
	CheckKindAdversarial  CheckKind = "adversarial_agent"
)

// CheckStatus represents the state of a verification check on a file.
type CheckStatus string

const (
	CheckStatusPending CheckStatus = "pending"
	CheckStatusPassed  CheckStatus = "passed"
	CheckStatusFailed  CheckStatus = "failed"
	CheckStatusSkipped CheckStatus = "skipped"
)

// DimensionCode represents authoritative evaluation dimensions in the knowledge kernel.
type DimensionCode string

const (
	DimensionHCODE   DimensionCode = "HCODE"
	DimensionEFFPERF DimensionCode = "EFFPERF"
	DimensionERRHYG  DimensionCode = "ERRHYG"
	DimensionCONCURR DimensionCode = "CONCURR"
	DimensionSECOBS  DimensionCode = "SECOBS"
	DimensionDOCSIG  DimensionCode = "DOCSIG"
)

// DimensionSpec defines canonical metadata and policy bindings for an evaluation dimension.
type DimensionSpec struct {
	Code        DimensionCode `json:"code"`
	Name        string        `json:"name"`
	PolicyID    string        `json:"policy_id"`
	Description string        `json:"description"`
	Severity    string        `json:"severity"`
}

// DefaultDimensions returns the canonical evaluation dimension specifications.
func DefaultDimensions() []DimensionSpec {
	return []DimensionSpec{
		{
			Code:        DimensionHCODE,
			Name:        "Hardcoded Logic and Literal Eradication",
			PolicyID:    "POL-CODE-1791593579304410000-e749bb5f",
			Description: "Zero hardcoded paths, versions, raw permissions, ports, and duplicate string literals.",
			Severity:    "critical",
		},
		{
			Code:        DimensionEFFPERF,
			Name:        "Efficiency, Complexity, and Resource Hygiene",
			PolicyID:    "POL-CODE-1791593697272400000-9b0f833b",
			Description: "Zero unbounded O(N^2) loops, mandatory context timeouts, and bounded buffers.",
			Severity:    "high",
		},
		{
			Code:        DimensionERRHYG,
			Name:        "Error Hygiene, Wrapping, and Fail-Closed Resilience",
			PolicyID:    "POL-CODE-1791593699894406000-cd6f25fd",
			Description: "Zero swallowed errors, structured error wrapping, and fail-closed safety.",
			Severity:    "high",
		},
		{
			Code:        DimensionCONCURR,
			Name:        "Concurrency Lifecycle and Named Goroutines",
			PolicyID:    "POL-CODE-1791593703788100000-9f2dff69",
			Description: "Zero naked goroutines, mandatory goroutinelabels, and explicit termination.",
			Severity:    "critical",
		},
		{
			Code:        DimensionSECOBS,
			Name:        "Zero-PII Security and Structured Observability",
			PolicyID:    "POL-CODE-1791593707127249000-965eda9e",
			Description: "Zero hardcoded credentials, zero PII in logs, and structured observability.",
			Severity:    "critical",
		},
		{
			Code:        DimensionDOCSIG,
			Name:        "API Signatures, Documentation, and Contracts",
			PolicyID:    "POL-CODE-1791593709986529000-029390e6",
			Description: "100% exported symbol GoDoc coverage and contract synchronization.",
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
	Name            FileClass `json:"name"`
	Description     string    `json:"description"`
	Extensions      []string  `json:"extensions"`
	MatchPatterns   []string  `json:"match_patterns,omitempty"`
	ExcludePatterns []string  `json:"exclude_patterns,omitempty"`
	RequiredChecks  []string  `json:"required_checks"`
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
