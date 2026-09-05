package audit

// Metadata keys and well-known values for audit events.
const (
	MetadataKeySource = "source"

	MetadataSourceCLI     = "cli"
	MetadataSourceGitHook = "git_hook"

	MetadataKeyProjectRoot = "project_root"

	MetadataKeyCommand = "command"
	MetadataKeyArgs    = "args"

	MetadataKeyRoles       = "roles"
	MetadataKeyPermissions = "permissions"

	MetadataKeyGitUser       = "git_user"
	MetadataKeyGitEmail      = "git_email"
	MetadataKeyCommitMessage = "commit_message"
	MetadataKeyStagedFiles   = "staged_files"
	MetadataKeyGoFileCount   = "go_file_count"
	MetadataKeyTotalFiles    = "total_files"

	MetadataKeyChangedFields = "changed_fields"

	MetadataKeyOriginalEventType = "original_event_type"
)
