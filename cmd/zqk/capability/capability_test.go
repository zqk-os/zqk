package capability

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	hivecap "github.com/zqk-os/zqk/pkg/hive/capability"
)

func TestCapabilityAuthorize_ForbiddenReturnsPolicyRejected(t *testing.T) {
	cmd := NewCapabilityCmd()
	cmd.SetArgs([]string{"authorize", "--name", "demo:forbidden", "--agent-id", "stranger"})
	err := cmd.Execute()
	require.Error(t, err)
	require.True(t, errors.Is(err, hivecap.ErrPolicyRejected))
}

func TestCapabilityAuthorize_SafeAllows(t *testing.T) {
	cmd := NewCapabilityCmd()
	cmd.SetArgs([]string{"authorize", "--name", "demo:safe", "--agent-id", "stranger"})
	require.NoError(t, cmd.Execute())
}

func TestCapabilityList_Runs(t *testing.T) {
	cmd := NewCapabilityCmd()
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())
}
