package storage

// GetContentAddressableStorageForTest exposes getContentAddressableStorage for storage_test package tests.
func (f *FileObjectStorage) GetContentAddressableStorageForTest(kind string) (*ContentAddressableStorage, error) {
	return f.getContentAddressableStorage(kind)
}

// CasIndexBatchSizeForTest is the max CAS index batch size (see casIndexBatchSize).
const CasIndexBatchSizeForTest = casIndexBatchSize
