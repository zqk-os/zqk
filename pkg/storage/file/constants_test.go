package file_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/storage/file"
	"github.com/stretchr/testify/require"
)

func TestFileConstants(t *testing.T) {
	require.NotEmpty(t, file.ErrMsgSwallowedError)
	require.NotEmpty(t, file.ConstMiscFailedToAcquireFileLock)
}
