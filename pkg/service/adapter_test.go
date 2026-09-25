package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/service"
)

func TestServiceSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    service.ServiceSpec
		wantErr bool
	}{
		{
			name: "valid minimal spec",
			spec: service.ServiceSpec{
				ID:         "com.example.service",
				Executable: "/usr/local/bin/example",
			},
			wantErr: false,
		},
		{
			name: "missing ID fails",
			spec: service.ServiceSpec{
				Executable: "/usr/local/bin/example",
			},
			wantErr: true,
		},
		{
			name: "missing Executable fails",
			spec: service.ServiceSpec{
				ID: "com.example.service",
			},
			wantErr: true,
		},
		{
			name: "whitespace ID fails",
			spec: service.ServiceSpec{
				ID:         "   ",
				Executable: "/usr/local/bin/example",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, service.ErrInvalidSpec))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMockAdapter_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := service.NewMockAdapter("mock", true)
	assert.Equal(t, "mock", mock.Name())
	assert.True(t, mock.IsAvailable())

	spec := service.ServiceSpec{
		ID:          "com.example.daemon",
		DisplayName: "Example Daemon",
		Executable:  "/bin/echo",
		Arguments:   []string{"hello"},
		RunAtLoad:   false,
	}

	// 1. Initial status -> stopped
	st, err := mock.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateStopped, st.State)

	// 2. Install
	err = mock.Install(ctx, spec)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.GetInstalledCount())

	// Duplicate install error
	err = mock.Install(ctx, spec)
	assert.ErrorIs(t, err, service.ErrServiceAlreadyExists)

	// 3. Start
	err = mock.Start(ctx, spec.ID)
	require.NoError(t, err)
	assert.True(t, mock.IsRunning(spec.ID))

	st, err = mock.Status(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, service.StateRunning, st.State)
	assert.Greater(t, st.PID, 0)

	// Duplicate start error
	err = mock.Start(ctx, spec.ID)
	assert.ErrorIs(t, err, service.ErrServiceAlreadyActive)

	// 4. Restart
	err = mock.Restart(ctx, spec.ID)
	require.NoError(t, err)
	assert.True(t, mock.IsRunning(spec.ID))

	// 5. Stop
	err = mock.Stop(ctx, spec.ID)
	require.NoError(t, err)
	assert.False(t, mock.IsRunning(spec.ID))

	// Stop when already stopped
	err = mock.Stop(ctx, spec.ID)
	assert.ErrorIs(t, err, service.ErrServiceNotRunning)

	// 6. Uninstall
	err = mock.Uninstall(ctx, spec.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, mock.GetInstalledCount())

	// Uninstall non-existent
	err = mock.Uninstall(ctx, spec.ID)
	assert.ErrorIs(t, err, service.ErrServiceNotFound)
}

func TestMockAdapter_CleanupLegacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := service.NewMockAdapter("mock", true)

	s1 := service.ServiceSpec{ID: "legacy.daemon.1", Executable: "/bin/sleep"}
	s2 := service.ServiceSpec{ID: "legacy.daemon.2", Executable: "/bin/sleep"}
	s3 := service.ServiceSpec{ID: "current.daemon", Executable: "/bin/sleep"}

	require.NoError(t, mock.Install(ctx, s1))
	require.NoError(t, mock.Install(ctx, s2))
	require.NoError(t, mock.Install(ctx, s3))
	assert.Equal(t, 3, mock.GetInstalledCount())

	cleaned, err := mock.CleanupLegacy(ctx, []string{"legacy.daemon.1", "legacy.daemon.2", "nonexistent"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"legacy.daemon.1", "legacy.daemon.2"}, cleaned)
	assert.Equal(t, 1, mock.GetInstalledCount())
}
