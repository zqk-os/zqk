package wal

const (
	emptyValue = ""

	ConstStreamCreateWalDir                                     = `create WAL dir`
	ConstStreamFailedToCloseTempWalFile                         = `failed to close temp WAL file`
	ConstStreamFailedToCreateTempWalFile                        = `failed to create temp WAL file`
	ConstStreamFailedToFlushWalFile                             = `failed to flush WAL file`
	ConstStreamFailedToMarshalWalRecord                         = `failed to marshal WAL record`
	ConstStreamFailedToOpenWalFile                              = `failed to open WAL file`
	ConstStreamFailedToReadCheckpoint                           = `failed to read checkpoint`
	ConstStreamFailedToReadWalFile                              = `failed to read WAL file`
	ConstStreamFailedToReplaceWalFile                           = `failed to replace WAL file`
	ConstStreamFailedToSyncWalFile                              = `failed to sync WAL file`
	ConstStreamFailedToWriteWalNewline                          = `failed to write WAL newline`
	ConstStreamFailedToWriteWalRecord                           = `failed to write WAL record`
	ConstStreamMarshalWalBatch                                  = `marshal WAL batch`
	ConstStreamMarshalWalRecord                                 = `marshal WAL record`
	ConstStreamMigrateObjectWalRenameStrToStrErr                = `migrate object WAL: rename %s -> %s: %w`
	ConstStreamObjectWalRequiresNonEmptyProjectRoot             = `object WAL requires non-empty projectRoot`
	ConstStreamProjectRootRequiredForWalCompaction              = `project root required for WAL compaction`
	ConstStreamRecordsApplied                                   = `records_applied`
	ConstStreamRemovedEntries                                   = `removed_entries`
	ConstStreamReplayRecordSeqIntErr                            = `replay record seq=%d: %w`
	ConstStreamSwallowedErrorValN                               = `swallowed error: %v\n`
	ConstStreamToReduceWalSizeRunTheSchedulerSoTheWriteBehind   = `To reduce WAL size, run the scheduler so the write-behind worker drains it, or use 'zqk storage compact-wal'.`
	ConstStreamWalBatchMarshalledSizeIntExceedsMaxLineSizeInt   = `WAL batch marshalled size (%d) exceeds max line size (%d) count=%d`
	ConstStreamWalBatchRecordObjectIdLengthIntExceedsMaxIntKind = `WAL batch record object ID length (%d) exceeds max (%d) for kind '%s' (id: %s)`
	ConstStreamWalRecordMarshalledSizeIntExceedsMaxLineSizeInt  = `WAL record marshalled size (%d) exceeds max line size (%d) for %s/%s`
	ConstStreamWalRecordObjectIdLengthIntExceedsMaxIntKindStr   = `WAL record object ID length (%d) exceeds max (%d) for kind '%s'`
	ConstStreamWriteWalBatch                                    = `write WAL batch`
	ConstStreamWriteWalNewline                                  = `write WAL newline`
)

const (
	ConstMiscAuditEventValuesMarshal                  = `audit event values marshal`
	ConstMiscAuditEventValuesUnmarshal                = `audit event values unmarshal`
	ConstMiscAuditEventYamlMarshal                    = `audit event YAML marshal`
	ConstMiscAuditEventYamlUnmarshal                  = `audit event YAML unmarshal`
	ConstMiscDecodeDatab64ForCompact                  = `decode data_base64 for compact`
	ConstMiscInvalidWalLineMissingOp                  = `invalid WAL line (missing op)`
	ConstMiscMarshalcompactbatchEmptySlice            = `marshalCompactBatch: empty slice`
	ConstMiscFailedToReadWalCheckpoint                = `failed to read WAL checkpoint`
	ConstMiscTimeoutWaitingForWalProcessingAppliedSeq = `timeout waiting for WAL processing: appliedSeq=%v progressed=%v`
)
