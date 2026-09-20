package file_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/storage/file"
)

func TestFileConstants(t *testing.T) {
	require.NotEmpty(t, file.ErrMsgSwallowedError)
	require.NotEmpty(t, file.ConstMiscFailedToAcquireFileLock)
}
