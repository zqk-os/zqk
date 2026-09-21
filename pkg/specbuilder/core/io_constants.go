package core

import (
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// OutputDirectoryPerm is the default directory permission for generated output paths.
	OutputDirectoryPerm fileutil.FileMode = paths.DirPerm755
	// OutputFilePerm is the default file permission for generated artifacts written directly.
	OutputFilePerm fileutil.FileMode = paths.FilePerm600
	// FileExtYAML is the canonical YAML extension for generated specbuilder artifacts.
	FileExtYAML = ".yaml"
)
