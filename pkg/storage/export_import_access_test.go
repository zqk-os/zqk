package storage

// UnmarshalImportForTest exposes unmarshalImport for external storage_test package tests.
// Prefer keeping export/import assertions in package storage when possible.
func UnmarshalImportForTest(data []byte, format ExportFormat) ([]map[string]any, error) {
	return unmarshalImport(data, format)
}
