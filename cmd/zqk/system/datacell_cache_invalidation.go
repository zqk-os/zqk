package system

import "github.com/zqk-os/zqk/pkg/datacellregistry"

// InvalidateDescriptorReadModelCache clears the in-process data-cell descriptor snapshot for this
// project after spec_index materialization changes (see datacellregistry.InvalidateDescriptorReadModelCache).
func InvalidateDescriptorReadModelCache(projectRoot string) {
	datacellregistry.InvalidateDescriptorReadModelCache(projectRoot)
}
