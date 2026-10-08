package keystore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKeystoreCmd_Structure(t *testing.T) {
	cmd := NewKeystoreCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "keystore", cmd.Use)
	assert.NotEmpty(t, cmd.Commands())

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	assert.True(t, subcommands["create"])
	assert.True(t, subcommands["issue"])
	assert.True(t, subcommands["list"])
	assert.True(t, subcommands["rotate"])
}

func TestNewCreateCmd_Flags(t *testing.T) {
	cmd := NewCreateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "create [flags]", cmd.Use)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("account-id"))
	assert.NotNil(t, flags.Lookup("key-type"))
	assert.NotNil(t, flags.Lookup("credential"))
	assert.NotNil(t, flags.Lookup("title"))
	assert.NotNil(t, flags.Lookup("description"))
	assert.NotNil(t, flags.Lookup("expires-at"))
}

func TestNewIssueCmd_Flags(t *testing.T) {
	cmd := NewIssueCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "issue [flags]", cmd.Use)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("account-id"))
	assert.NotNil(t, flags.Lookup("title"))
	assert.NotNil(t, flags.Lookup("description"))
	assert.NotNil(t, flags.Lookup("expires-at"))
	assert.NotNil(t, flags.Lookup("no-seating-file"))
}

func TestNewListCmd_Flags(t *testing.T) {
	cmd := NewListCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "list [flags]", cmd.Use)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("filter"))
	assert.NotNil(t, flags.Lookup("sort-by"))
	assert.NotNil(t, flags.Lookup("sort-asc"))
	assert.NotNil(t, flags.Lookup("offset"))
	assert.NotNil(t, flags.Lookup("limit"))
}

func TestNewRotateCmd_Flags(t *testing.T) {
	cmd := NewRotateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "rotate <key-id> [flags]", cmd.Use)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("new-credential"))
	assert.NotNil(t, flags.Lookup("revoke"))
	assert.NotNil(t, flags.Lookup("revoke-old"))
}
