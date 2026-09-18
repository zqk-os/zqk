package filecas

// TRACK: BLI-CEF-R2-REL-CAS-FSYNC — Darwin queues file fsync to avoid blocking
// publication and synchronously fsyncs the parent directory; other platforms
// perform both operations synchronously. Tests may replace these hooks.
var CasPublishSyncFile = CasPublishSyncFileOS
var CasPublishSyncDir = CasPublishSyncDirOS
