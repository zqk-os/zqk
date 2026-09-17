package audit

// Buffer is the aggregation buffer slice Create uses. AuditEventBuffer in
// package storage implements this; lock and flush I/O stay there.
type Buffer interface {
	ShouldAggregate(event map[string]any) bool
	AddEvent(event map[string]any) error
}

// IDAllocator allocates the next AUD- id. The CAS-aware generator stays in
// package storage.
type IDAllocator interface {
	GenerateNextID() (string, error)
}
