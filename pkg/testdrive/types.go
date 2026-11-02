package testdrive

import (
	"io"
)

// TestDriveOptions configures the guided developer test-drive experience.
type TestDriveOptions struct {
	Intent         string    `json:"intent"`
	Scenario       string    `json:"scenario"`
	Speed          string    `json:"speed"`
	NonInteractive bool      `json:"non_interactive"`
	DryRun         bool      `json:"dry_run"`
	Writer         io.Writer `json:"-"`
	Reader         io.Reader `json:"-"`
}

// MintedObjectSummary summarizes a Knowledge Kernel graph object minted during the test drive.
type MintedObjectSummary struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// SwarmWorkerSimulation summarizes a simulated swarm worker seat.
type SwarmWorkerSimulation struct {
	SeatID    string `json:"seat_id"`
	Persona   string `json:"persona"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Duration  string `json:"duration"`
	ASTChecks int    `json:"ast_checks"`
}

// TestDriveStageResult records the outcome of a single test drive stage.
type TestDriveStageResult struct {
	Stage     string   `json:"stage"`
	Title     string   `json:"title"`
	Success   bool     `json:"success"`
	Summary   string   `json:"summary"`
	Artifacts []string `json:"artifacts,omitempty"`
	Duration  string   `json:"duration"`
}

// TestDriveReport provides a complete report of the test drive execution.
type TestDriveReport struct {
	Intent         string                  `json:"intent"`
	Scenario       string                  `json:"scenario"`
	Success        bool                    `json:"success"`
	Duration       string                  `json:"duration"`
	Stages         []TestDriveStageResult  `json:"stages"`
	MintedObjects  []MintedObjectSummary   `json:"minted_objects"`
	SwarmWorkers   []SwarmWorkerSimulation `json:"swarm_workers"`
	VDSReceiptHash string                  `json:"vds_receipt_hash"`
	NextSteps      []string                `json:"next_steps"`
}
