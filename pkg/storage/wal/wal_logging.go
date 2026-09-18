package wal

import (
	"github.com/zqk-os/zqk/pkg/logging"
)

func StorageLog(logger logging.Logger) *logging.FluentRoot {
	return logging.Fluent(logger)
}

const (
	ErrMsgSwallowedError = "swallowed error"

	LogEventStorageObjectWALReplaySkipLineWarn                  = "storage.object_wal.replay.skip_line"
	LogEventStorageObjectWALReplayCompletedInfo                 = "storage.object_wal.replay.completed"
	LogEventStorageObjectWALCompactionSkipLineWarn              = "storage.object_wal.compaction.skip_line"
	LogEventStorageObjectWALCompactionAppliedSeqZeroWarn        = "storage.object_wal.compaction.applied_seq_zero"
	LogEventStorageObjectWALCompactionAppliedInfo               = "storage.object_wal.compaction.applied"
	LogEventStorageObjectWALCompactionWriteCheckpointFailedErr  = "storage.object_wal.compaction.write_checkpoint_failed"
	LogEventStorageObjectWALCompactionFailedErr                 = "storage.object_wal.compaction.failed"
	LogEventStorageObjectWALCompactionCheckpointWriteFailedErr  = "storage.object_wal.compaction.checkpoint_write_failed"
	LogEventStorageObjectWALCompactionReplaySkipCorruptLineWarn = "storage.object_wal.compaction.replay_skip_corrupt_line"
	LogEventStorageObjectWALCompactionWriteRecordFailedErr      = "storage.object_wal.compaction.write_record_failed"
)

const (
	LogEventStorageObjectWALCompactionNoRemovableInfo = "storage.object_wal.compaction.no_removable"
	LogEventStorageObjectWALCompactionCompletedInfo   = "storage.object_wal.compaction.completed"
	LogEventStorageWriteBehindOwnerSkipped            = "storage.write_behind.owner_skipped"
)
