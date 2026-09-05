package filecas

// TRACK: BLI-CEF-R2-REL-CAS-FSYNC — darwin skips fsync (F_FULLFSYNC stalls);
// other GOOS fsync the temp file and parent dir. Tests may replace these hooks.
var CasPublishSyncFile = CasPublishSyncFileOS
var CasPublishSyncDir = CasPublishSyncDirOS
