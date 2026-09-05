package core

import (
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	// OutputDirectoryPerm is the default directory permission for generated output paths.
	OutputDirectoryPerm fileutil.FileMode = 0o755
	// OutputFilePerm is the default file permission for generated artifacts written directly.
	OutputFilePerm fileutil.FileMode = 0o600
	// FileExtYAML is the canonical YAML extension for generated specbuilder artifacts.
	FileExtYAML = ".yaml"
)
