package core

import "os"

const (
	// OutputDirectoryPerm is the default directory permission for generated output paths.
	OutputDirectoryPerm os.FileMode = 0o755
	// OutputFilePerm is the default file permission for generated artifacts written directly.
	OutputFilePerm os.FileMode = 0o600
	// FileExtYAML is the canonical YAML extension for generated specbuilder artifacts.
	FileExtYAML = ".yaml"
)
