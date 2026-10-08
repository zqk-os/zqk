package keystore

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestPluralize(t *testing.T) {
	assert.Equal(t, "y", pluralize(1))
	assert.Equal(t, "ies", pluralize(0))
	assert.Equal(t, "ies", pluralize(5))
}

func TestListCmd_Empty(t *testing.T) {
	tmpDir, _, _ := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	cmd := NewListCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestListCmd_WithEntry(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key for List",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:fakehash",
		objects.FieldKeyDescription:    "Test key description",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry))
	id, _ := entry[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id)

	cmd := NewListCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	// JSON format
	cmdJSON := NewListCmd()
	cmdJSON.SetArgs([]string{"--format", "json"})
	bufJSON := new(bytes.Buffer)
	cmdJSON.SetOut(bufJSON)
	cmdJSON.SetErr(bufJSON)

	err = cmdJSON.Execute()
	require.NoError(t, err)
}
