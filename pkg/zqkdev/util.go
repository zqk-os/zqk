package zqkdev

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/datacellregistry"
	"github.com/lanceman/zqk/pkg/strutil"
)

func ProjectRootOrResolve(path string) string {
	return strutil.OrDefault(path, cli.ResolveProjectRoot("."))
}

func InvalidateDescriptorReadModelCache(projectRoot string) error {
	datacellregistry.InvalidateDescriptorReadModelCache(projectRoot)
	return nil
}
