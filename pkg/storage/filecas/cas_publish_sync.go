package filecas

// Darwin queues file fsync to avoid blocking
// publication and synchronously fsyncs the parent directory; other platforms
// perform both operations synchronously. Tests may replace these hooks.
var CasPublishSyncFile = CasPublishSyncFileOS
var CasPublishSyncDir = CasPublishSyncDirOS
