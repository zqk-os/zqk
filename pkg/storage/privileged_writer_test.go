package storage

import "testing"

func TestPrivilegedWriterInterfaceExists(t *testing.T) {
	t.Parallel()
	var _ PrivilegedWriter = (*IPCWriter)(nil)
}
