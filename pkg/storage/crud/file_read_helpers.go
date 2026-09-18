package crud

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

func ApplyRuntimeDeltaOverlay(f FileStorageReadFacade, projectRoot, kind, id string, base map[string]any) map[string]any {
	if base == nil || f.StreamStorageEnabledForKind(kind) || !f.RuntimeDeltaEnabledForKind(projectRoot, kind) {
		return base
	}
	overlay := f.ReadRuntimeDeltaCurrentState(projectRoot, kind, id)
	if overlay == nil {
		return base
	}
	for k, v := range overlay {
		if k == objects.FieldKeyID || k == objects.FieldKeyKind {
			continue
		}
		if k == "annotations" {
			if bAnn, ok := base[k].(map[string]any); ok {
				if oAnn, ok := v.(map[string]any); ok {
					for ak, av := range oAnn {
						bAnn[ak] = av
					}
					continue
				}
			}
		}
		base[k] = v
	}
	return base
}

func MaterializeCasYAMLMapAfterLoad(f FileStorageReadFacade, projectRoot, kind, logicalIDHint string, base map[string]any) map[string]any {
	if base == nil {
		return nil
	}
	logicalID, _ := base[objects.FieldKeyID].(string)
	if logicalID == "" {
		logicalID = logicalIDHint
	}
	if logicalID != "" {
		base = ApplyRuntimeDeltaOverlay(f, projectRoot, kind, logicalID, base)
	}
	base[objects.FieldKeyKind] = kind
	return base
}
