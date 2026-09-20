package storage

import "testing"

func TestIPCWriterTypeExists(t *testing.T) {
	t.Parallel()
	_ = IPCWriter{}
}
