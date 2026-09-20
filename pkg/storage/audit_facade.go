package storage

import "github.com/zqk-os/zqk/pkg/storage/audit"

// AuditFacade is the storage-root alias for the audit subpackage aggregator.
// FileObjectStorage is not an Aggregator; AuditAggregationService is.
type AuditFacade = audit.Aggregator

var (
	_ audit.Aggregator       = (*AuditAggregationService)(nil)
	_ audit.Buffer           = (*AuditEventBuffer)(nil)
	_ audit.IDAllocator      = (*AuditIDGenerator)(nil)
	_ audit.ObjectQuery      = auditStore{}
	_ audit.MetricStore      = auditStore{}
	_ audit.BulkStatusWriter = auditStore{}
	_ audit.BulkDeleter      = auditStore{}
)
