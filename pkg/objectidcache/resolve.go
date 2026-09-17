package objectidcache

import (
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// ProjectRootResolver maps empty/"." sentinels to a concrete project root.
type ProjectRootResolver func(projectRoot string) string

var resolveProjectRoot ProjectRootResolver = defaultResolveProjectRoot

func defaultResolveProjectRoot(projectRoot string) string {
	if projectRoot == emptyValue || projectRoot == "." {
		if wd, err := fileutil.Getwd(); err == nil {
			return wd
		}
	}
	return projectRoot
}

// SetProjectRootResolver installs the CLI-aware resolver (walks to brand settings).
// Call from cmd/zqk/system init so cache ops share ProjectRootOrResolveDot.
func SetProjectRootResolver(fn ProjectRootResolver) {
	if fn != nil {
		resolveProjectRoot = fn
	}
}
