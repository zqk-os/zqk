package storage

import "testing"

func TestPrivilegedWriterDaemonTypeExists(t *testing.T) {
	t.Parallel()
	_ = PrivilegedWriterDaemon{}
}
