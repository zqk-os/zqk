package wal_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/wal"
)

func TestWALLogging(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	entry := wal.StorageLog(logger)
	require.NotNil(t, entry)
}
