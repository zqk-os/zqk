package audit

import (
	"context"
	"time"
)

const FlushTSDBMeasurement = "audit_metric"

// FlushWriteResult is the outcome of FlushSink.WriteSummary.
type FlushWriteResult struct {
	UsedCAS bool
	Err     error
}

// FlushSink persists an aggregated_summary without EventStore.Create.
// Create re-enters the audit buffer; flush I/O must stay off that path.
type FlushSink interface {
	WriteSummary(ctx context.Context, dir, id string, event map[string]any, tags map[string]string) FlushWriteResult
}

// WriteFlush writes via sink and records flush metrics. Nil sink is a no-op success.
func WriteFlush(
	ctx context.Context,
	sink FlushSink,
	rec CreationRecorder,
	dir, id string,
	event map[string]any,
	tags map[string]string,
	validOK bool,
) FlushWriteResult {
	start := time.Now()
	var result FlushWriteResult
	if sink != nil {
		result = sink.WriteSummary(ctx, dir, id, event, tags)
	}
	RecordFlushWrite(ctx, rec, result.UsedCAS, time.Since(start), result.Err == nil, validOK)
	return result
}
