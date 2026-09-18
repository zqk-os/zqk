package zqkdev

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/datacellregistry"
	"github.com/zqk-os/zqk/pkg/strutil"
)

func ProjectRootOrResolve(path string) string {
	return strutil.OrDefault(path, cli.ResolveProjectRoot("."))
}

func InvalidateDescriptorReadModelCache(projectRoot string) error {
	datacellregistry.InvalidateDescriptorReadModelCache(projectRoot)
	return nil
}
