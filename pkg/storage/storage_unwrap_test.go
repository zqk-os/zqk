package storage

import (
	"testing"
)

func TestUnwrapToFileObjectStorage(t *testing.T) {
	// 1. Nil provider
	if UnwrapToFileObjectStorage(nil) != nil {
		t.Errorf("expected nil for nil provider")
	}

	// 2. Direct FileObjectStorage
	fs := &FileObjectStorage{projectRoot: "test-root"}
	if UnwrapToFileObjectStorage(fs) != fs {
		t.Errorf("expected direct FileObjectStorage to be returned")
	}

	// 3. HybridObjectStorage wrapper
	hybrid := NewHybridObjectStorage(fs, nil)
	if UnwrapToFileObjectStorage(hybrid) != fs {
		t.Errorf("expected unwrapped FileObjectStorage from HybridObjectStorage primary")
	}

	hybridSecondary := NewHybridObjectStorage(nil, fs)
	if UnwrapToFileObjectStorage(hybridSecondary) != fs {
		t.Errorf("expected unwrapped FileObjectStorage from HybridObjectStorage secondary")
	}

	// 4. MeshObjectStorage wrapper
	mesh := NewMeshObjectStorage(fs, nil)
	if UnwrapToFileObjectStorage(mesh) != fs {
		t.Errorf("expected unwrapped FileObjectStorage from MeshObjectStorage")
	}

	// 5. Nested wrapping: Mesh(Hybrid(File))
	nested := NewMeshObjectStorage(hybrid, nil)
	if UnwrapToFileObjectStorage(nested) != fs {
		t.Errorf("expected unwrapped FileObjectStorage from nested wrappers")
	}
}
