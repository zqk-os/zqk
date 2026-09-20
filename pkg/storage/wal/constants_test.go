package wal_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/storage/wal"
)

func TestWALConstants(t *testing.T) {
	require.NotEmpty(t, wal.ConstStreamCreateWalDir)
	require.NotEmpty(t, wal.ConstMiscAuditEventValuesMarshal)
}
