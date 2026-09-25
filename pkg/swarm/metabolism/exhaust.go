package metabolism

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrInvalidOutputDir is returned when the target exhaust directory is invalid or inaccessible.
	ErrInvalidOutputDir = errors.New("invalid or insecure exhaust output directory")

	// ErrFindingInvalid is returned when a finding fails structural validation.
	ErrFindingInvalid = errors.New("finding is missing mandatory fields (id, lens, severity, title)")
)

// Finding represents an actionable defect or observation emitted during pack evaluation.
type Finding struct {
	ID          string   `json:"id"`
	Lens        string   `json:"lens"`
	Severity    string   `json:"severity"` // "E0", "E1", "E2", "E3"
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Evidence    string   `json:"evidence,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	Files       []string `json:"files,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
}

// Scorecard represents the multi-dimensional evaluation results and envelope metric.
type Scorecard struct {
	PackURN     string             `json:"pack_urn"`
	SessionID   string             `json:"session_id"`
	Timestamp   string             `json:"timestamp"`
	EnvelopeMin float64            `json:"envelope_min"`
	Scores      map[string]float64 `json:"scores"`
	Status      string             `json:"status"` // "converged", "non_converged", "passed", "failed"
}

// StreamARecipient receives anabolic internal kernel object mutations.
type StreamARecipient interface {
	RecordKernelMutation(kind string, id string, data map[string]any) error
}

// DualStreamRouter coordinates the split between internal anabolic mutations (Stream A)
// and external catabolic exhaust files (Stream B).
type DualStreamRouter struct {
	mu           sync.Mutex
	outputDir    string
	streamA      StreamARecipient
	findingsFile string
}

// NewDualStreamRouter creates a router ensuring outputDir is safely created and isolated.
func NewDualStreamRouter(outputDir string, streamA StreamARecipient) (*DualStreamRouter, error) {
	if outputDir == "" {
		outputDir = filepath.Join(paths.ProjectDataDir, "exhaust", fmt.Sprintf("run-%d", time.Now().UnixNano()))
	}

	cleanDir := filepath.Clean(outputDir)
	if err := fileutil.MkdirAll(cleanDir, paths.DirPerm755); err != nil {
		return nil, fmt.Errorf("%w: failed to create output directory %s: %v", ErrInvalidOutputDir, cleanDir, err)
	}

	return &DualStreamRouter{
		outputDir:    cleanDir,
		streamA:      streamA,
		findingsFile: filepath.Join(cleanDir, "findings.jsonl"),
	}, nil
}

// OutputDir returns the absolute or configured path to the exhaust directory.
func (r *DualStreamRouter) OutputDir() string {
	return r.outputDir
}

// RouteFinding emits a finding to Stream B (findings.jsonl) and optionally alerts Stream A.
func (r *DualStreamRouter) RouteFinding(f Finding) error {
	if f.ID == "" || f.Lens == "" || f.Severity == "" || f.Title == "" {
		return ErrFindingInvalid
	}
	if f.CreatedAt == "" {
		f.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Stream B: Append to findings.jsonl
	line, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("failed to serialize finding: %w", err)
	}

	fHandle, err := fileutil.OpenFile(r.findingsFile, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm644)
	if err != nil {
		return fmt.Errorf("failed to open findings.jsonl: %w", err)
	}
	defer fHandle.Close()

	if _, err := fHandle.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("failed to write finding to exhaust: %w", err)
	}

	// Stream A: If Stream A is wired, notify for potential defect BLI synthesis
	if r.streamA != nil {
		objData := map[string]any{
			"id":          f.ID,
			"lens":        f.Lens,
			"severity":    f.Severity,
			"title":       f.Title,
			"description": f.Description,
		}
		if err := r.streamA.RecordKernelMutation("finding", f.ID, objData); err != nil {
			return fmt.Errorf("failed to record finding mutation to stream A: %w", err)
		}
	}

	return nil
}

// RouteScorecard emits the evaluation scorecard to Stream B (scorecard.json).
func (r *DualStreamRouter) RouteScorecard(sc Scorecard) error {
	if sc.Timestamp == "" {
		sc.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize scorecard: %w", err)
	}

	scorecardPath := filepath.Join(r.outputDir, "scorecard.json")
	if err := fileutil.WriteFile(scorecardPath, data, paths.FilePerm644); err != nil {
		return fmt.Errorf("failed to write scorecard.json: %w", err)
	}

	// Stream A: Record scorecard state if wired
	if r.streamA != nil {
		objData := map[string]any{
			"pack_urn":     sc.PackURN,
			"session_id":   sc.SessionID,
			"envelope_min": sc.EnvelopeMin,
			"status":       sc.Status,
		}
		if err := r.streamA.RecordKernelMutation("scorecard", sc.SessionID, objData); err != nil {
			return fmt.Errorf("failed to record scorecard mutation to stream A: %w", err)
		}
	}

	return nil
}

// RouteTrace writes raw diagnostic or seismograph trace data into outputDir/traces/<filename>.
func (r *DualStreamRouter) RouteTrace(filename string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	traceDir := filepath.Join(r.outputDir, "traces")
	if err := fileutil.MkdirAll(traceDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("failed to create traces directory: %w", err)
	}

	cleanFile := filepath.Base(filename)
	dest := filepath.Join(traceDir, cleanFile)
	return fileutil.WriteFile(dest, data, paths.FilePerm644)
}

// EmitSummary writes an executive summary markdown document to outputDir/summary.md.
func (r *DualStreamRouter) EmitSummary(markdownContent string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dest := filepath.Join(r.outputDir, "summary.md")
	return fileutil.WriteFile(dest, []byte(markdownContent), paths.FilePerm644)
}
