package testdrive

import "time"

const (
	// DefaultIntentPrompt is the default developer intent used when none is provided.
	DefaultIntentPrompt = "Build an authenticated multi-tenant SQLite task API with telemetry"

	// ScenarioAPIService is the REST / RPC API service scenario preset.
	ScenarioAPIService = "api-service"

	// ScenarioCLITool is the CLI tool scenario preset.
	ScenarioCLITool = "cli-tool"

	// ScenarioStreamPipeline is the event streaming pipeline scenario preset.
	ScenarioStreamPipeline = "stream-pipeline"

	// ScenarioCustom is the user-provided intent scenario.
	ScenarioCustom = "custom"

	// SpeedNormal is the human interactive pacing delay.
	SpeedNormal = "normal"

	// SpeedFast is the brisk demonstration pacing delay.
	SpeedFast = "fast"

	// SpeedInstant has zero artificial delay (for CI and automated tests).
	SpeedInstant = "instant"

	// NormalStepDelay is the pacing delay for normal speed.
	NormalStepDelay = 300 * time.Millisecond

	// FastStepDelay is the pacing delay for fast speed.
	FastStepDelay = 50 * time.Millisecond

	// StageSeedIntent represents Stage 1: Seed Planting (Vision & Mission).
	StageSeedIntent = "seed_intent"

	// StageOntologyCascade represents Stage 2: Intent Crystallization (5-Layer Cascade).
	StageOntologyCascade = "ontology_cascade"

	// StageSwarmAST represents Stage 3: Concurrent Swarm Execution & AST Proofs.
	StageSwarmAST = "swarm_ast"

	// StageVDSReceipt represents Stage 4: Cryptographic Verification Receipt & VDS Done-Gate.
	StageVDSReceipt = "vds_receipt"

	// MaxIntentLength limits developer intent string to prevent memory bloat.
	MaxIntentLength = 512

	// MinIntentLength requires sufficient semantic content.
	MinIntentLength = 3

	// DefaultSwarmSeatCount is the number of concurrent seats showcased during the test drive.
	DefaultSwarmSeatCount = 3
)
