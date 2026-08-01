package contextevents

// Wire keys for context_events.jsonl when no objects.FieldKey exists in this package
// or we avoid importing scheduler (WireJobID matches scheduler.KeyJobID).
const (
	// WireTsRFC3339 is the metrics JSONL timestamp field (POLICY-OBS-001).
	WireTsRFC3339     = "ts_rfc3339"
	WireJobID         = "job_id"
	WireCorrelationID = "correlation_id"
)

// Legacy JSON keys from earlier context_events drafts (unmarshal only).
const (
	wireLegacyBacklogItemIDs       = "backlog_item_ids"
	wireLegacyConvergenceSessionID = "convergence_session_id"
)

// Semantic event_type / source values produced by repo pipelines (callers append via Append).
const (
	// EventTypeTestBundleMatrixVerifyOK is emitted when verify-test-bundle-matrix passes in quality.RunTestBundleMatrixPipeline.
	EventTypeTestBundleMatrixVerifyOK = "test_bundle_matrix_verify_ok"
	// SourceTestBundleMatrixPipeline identifies pkg/quality.RunTestBundleMatrixPipeline.
	SourceTestBundleMatrixPipeline = "test_bundle_matrix_pipeline"
	// WirePayloadBundlePrefix is payload map key for TestBundleMatrixOptions.BundlePrefix when non-empty.
	WirePayloadBundlePrefix = "bundle_prefix"
	// WirePayloadStrictVerify is payload map key for TestBundleMatrixOptions.StrictVerify.
	WirePayloadStrictVerify = "strict_verify"
)
