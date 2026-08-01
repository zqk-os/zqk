package storage

// Audit metadata keys and well-known values.
//
// These are intentionally centralized to:
// - Prevent string drift across code paths
// - Make auditing/analytics stable across versions
// - Avoid hardcoding ad-hoc keys in multiple places
const (
	// AuditMetadataKeySource identifies the originating subsystem.
	AuditMetadataKeySource = "source"

	// AuditMetadataSourceCLI indicates a CLI-originated operation.
	AuditMetadataSourceCLI = "cli"
	// AuditMetadataSourceGitHook indicates a git-hook-originated operation.
	AuditMetadataSourceGitHook = "git_hook"

	AuditMetadataKeyProjectRoot = "project_root"

	AuditMetadataKeyCommand = "command"
	AuditMetadataKeyArgs    = "args"

	AuditMetadataKeyRoles       = "roles"
	AuditMetadataKeyPermissions = "permissions"

	AuditMetadataKeyGitUser       = "git_user"
	AuditMetadataKeyGitEmail      = "git_email"
	AuditMetadataKeyCommitMessage = "commit_message"
	AuditMetadataKeyStagedFiles   = "staged_files"
	AuditMetadataKeyGoFileCount   = "go_file_count"
	AuditMetadataKeyTotalFiles    = "total_files"

	AuditMetadataKeyChangedFields = "changed_fields"
)
