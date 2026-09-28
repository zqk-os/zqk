package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/service"
)

func TestNewStartCmd_Structure(t *testing.T) {
	cmd := NewStartCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "start", cmd.Use)
	assert.NotNil(t, cmd.Run)
}

func TestNewShutdownCmd_Structure(t *testing.T) {
	cmd := NewShutdownCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "shutdown", cmd.Use)
	assert.NotNil(t, cmd.Run)
}

func TestPluggableHostManager_Resolution(t *testing.T) {
	mgr := service.NewManager()
	require.NotNil(t, mgr)
	adapter := mgr.Adapter()
	require.NotNil(t, adapter)
	assert.NotEmpty(t, adapter.Name())
	assert.True(t, adapter.IsAvailable())
}
