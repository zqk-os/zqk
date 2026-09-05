package wal_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/storage/wal"
	"github.com/stretchr/testify/require"
)

func TestWALConstants(t *testing.T) {
	require.NotEmpty(t, wal.ConstStreamCreateWalDir)
	require.NotEmpty(t, wal.ConstMiscAuditEventValuesMarshal)
}
