package pipeline

import "time"

// Envelope is the immutable wrapper for pipeline payloads per the standardized data pipeline lifecycle.
// See docs/architecture/data-pipeline-lifecycle.md. Carries payload plus metadata required for
// idempotency, ordering, and observability.
type Envelope struct {
	TraceID        string    // Single id for the run (e.g. job_id + batch basename).
	Source         string    // Origin: "cli", "scheduler", or job ID.
	ReceivedAt     time.Time // When the payload was accepted (INGEST).
	IdempotencyKey string    // Stable key so retries do not double-apply effects.
	PartitionKey   string    // Key for ordered processing per key (e.g. project_root, object_id).
	Payload        any       // The actual data (raw bytes, normalized struct, etc.).
}
