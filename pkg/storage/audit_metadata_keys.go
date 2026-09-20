package storage

import "github.com/zqk-os/zqk/pkg/storage/audit"

// Audit metadata keys and well-known values.
// Canonical definitions live in pkg/storage/audit; these aliases keep root callers stable.
const (
	AuditMetadataKeySource = audit.MetadataKeySource

	AuditMetadataSourceCLI     = audit.MetadataSourceCLI
	AuditMetadataSourceGitHook = audit.MetadataSourceGitHook

	AuditMetadataKeyProjectRoot = audit.MetadataKeyProjectRoot

	AuditMetadataKeyCommand = audit.MetadataKeyCommand
	AuditMetadataKeyArgs    = audit.MetadataKeyArgs

	AuditMetadataKeyRoles       = audit.MetadataKeyRoles
	AuditMetadataKeyPermissions = audit.MetadataKeyPermissions

	AuditMetadataKeyGitUser       = audit.MetadataKeyGitUser
	AuditMetadataKeyGitEmail      = audit.MetadataKeyGitEmail
	AuditMetadataKeyCommitMessage = audit.MetadataKeyCommitMessage
	AuditMetadataKeyStagedFiles   = audit.MetadataKeyStagedFiles
	AuditMetadataKeyGoFileCount   = audit.MetadataKeyGoFileCount
	AuditMetadataKeyTotalFiles    = audit.MetadataKeyTotalFiles

	AuditMetadataKeyChangedFields = audit.MetadataKeyChangedFields

	OriginalEventTypeMetadataKey = audit.MetadataKeyOriginalEventType
)
