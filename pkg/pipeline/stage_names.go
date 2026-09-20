package pipeline

// Canonical pipeline stage labels.
//
// These labels are used for observability/metrics grouping; stage execution order is determined
// by the sequence of AddStage(...) calls in the pipeline builder (not by the label string).
const (
	StageIngest    = "INGEST"
	StageNormalize = "NORMALIZE"
	StageDecide    = "DECIDE"
	StageCommit    = "COMMIT"
	StageFinalize  = "FINALIZE"
)
