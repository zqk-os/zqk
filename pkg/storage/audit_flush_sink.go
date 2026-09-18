package storage

import (
	"context"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// auditFlushSink writes aggregated_summary payloads off the Create() path so
// flush cannot re-enter the audit buffer.
type auditFlushSink struct {
	tsdb        TSDBProvider
	fileStorage *FileObjectStorage
}

func (s auditFlushSink) WriteSummary(ctx context.Context, dir, id string, event map[string]any, tags map[string]string) audit.FlushWriteResult {
	if s.tsdb != nil {
		err := s.tsdb.WritePoint(ctx, TSDBPoint{
			Measurement: audit.FlushTSDBMeasurement,
			Tags:        tags,
			Fields:      event,
			Timestamp:   time.Now().UTC(),
		})
		return audit.FlushWriteResult{Err: err}
	}

	path := audit.SummaryFilePath(dir, id)
	data, err := FormatMultiLineYAML(event)
	if err != nil {
		return audit.FlushWriteResult{Err: errfmt.Newf(ErrMsgFormatAggEvent).Wrap(err)}
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	if StreamStorageEnabledForKind(objects.KindAuditEvent) {
		if s.fileStorage == nil {
			return audit.FlushWriteResult{Err: errfmt.Errorf(ErrMsgNoFileStorage)}
		}
		return audit.FlushWriteResult{Err: s.fileStorage.writeObjectToStorage(ctx, id, objects.KindAuditEvent, "", data, secCtx, false)}
	}
	usedCAS := s.fileStorage != nil && s.fileStorage.usesContentAddressableStorage(objects.KindAuditEvent)
	baseAuditDir := filepath.Dir(dir)
	err = WriteSystemObjectAndRegisterHash(path, data, objects.KindAuditEvent, baseAuditDir, id, s.fileStorage)
	return audit.FlushWriteResult{UsedCAS: usedCAS, Err: err}
}

var _ audit.FlushSink = auditFlushSink{}
