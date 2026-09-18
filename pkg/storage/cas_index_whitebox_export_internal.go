//go:build !production

package storage

import "github.com/zqk-os/zqk/pkg/storage/filecas"

func (f *FileObjectStorage) GetContentAddressableStorageForTest(kind string) (*filecas.ContentAddressableStorage, error) {
	return f.getContentAddressableStorage(kind)
}
