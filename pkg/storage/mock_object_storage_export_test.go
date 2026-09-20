package storage

import "context"

// NewMockObjectStorageForMetricsTests returns an ObjectStorageProvider backed by [mockObjectStorage]
// with a working Create for tests that persist objects (e.g. file-lock async metrics tests).
// For cross-package tests that only need a non-nil provider, prefer [NewNoopObjectStorage].
func NewMockObjectStorageForMetricsTests() ObjectStorageProvider {
	return &mockObjectStorage{}
}

func (m *mockObjectStorage) Shutdown(ctx context.Context) error {
	return nil
}
