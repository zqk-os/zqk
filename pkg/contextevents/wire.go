package contextevents

// Wire keys for context_events.jsonl when no objects.FieldKey exists in this package
// or we avoid importing scheduler (WireJobID matches scheduler.KeyJobID).
const (
	// WireTsRFC3339 is the metrics JSONL timestamp field (POL-OBS-001).
	WireTsRFC3339     = "ts_rfc3339"
	WireJobID         = "job_id"
	WireCorrelationID = "correlation_id"
)

// Legacy JSON keys from earlier context_events drafts (unmarshal only).
const (
	wireLegacyBacklogItemIDs       = "backlog_item_ids"
	wireLegacyConvergenceSessionID = "convergence_session_id"
)
