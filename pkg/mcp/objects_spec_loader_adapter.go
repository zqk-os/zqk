// Package mcp: adapter so callers with *objects.SpecLoader can use SpecAccessControl (ITEM-642 field-level permissions).

package mcp

import (
	"github.com/lanceman/zqk/pkg/objects"
)

// ObjectsSpecLoaderAdapter adapts *objects.SpecLoader to the SpecLoader interface.
// Use this when creating SpecAccessControl from a project's objects.SpecLoader
// (e.g. in object get/update commands for field-level permission checks).
type ObjectsSpecLoaderAdapter struct {
	Loader *objects.SpecLoader
}

// LoadSpecWithInheritance implements SpecLoader interface.
func (a *ObjectsSpecLoaderAdapter) LoadSpecWithInheritance(filename string) (Spec, error) {
	if a.Loader == nil {
		return nil, nil
	}
	spec, err := a.Loader.LoadSpecWithInheritance(filename)
	if err != nil {
		return nil, err
	}
	return &objectsSpecAdapter{spec: spec}, nil
}

// objectsSpecAdapter adapts *objects.Spec to Spec interface.
type objectsSpecAdapter struct {
	spec *objects.Spec
}

// GetResolvedFields implements Spec interface.
func (a *objectsSpecAdapter) GetResolvedFields() map[string]any {
	if a.spec == nil {
		return nil
	}
	return a.spec.ResolvedFields
}

// NewSpecAccessControlFromObjectsLoader creates SpecAccessControl from a project's *objects.SpecLoader.
// Returns nil if loader is nil. Callers can use it for field-level read/write checks (ITEM-642).
func NewSpecAccessControlFromObjectsLoader(loader *objects.SpecLoader) *SpecAccessControl {
	if loader == nil {
		return nil
	}
	return NewSpecAccessControl(&ObjectsSpecLoaderAdapter{Loader: loader})
}
