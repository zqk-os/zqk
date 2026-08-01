package zqkdev

import (
	"github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/system"
)

func ProjectRootOrResolve(path string) string {
	return system.ProjectRootOrResolve(path)
}

func InvalidateDescriptorReadModelCache(projectRoot string) error {
	system.InvalidateDescriptorReadModelCache(projectRoot)
	return nil
}
