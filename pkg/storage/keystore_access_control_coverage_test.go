package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestKeystoreAccessControl_ApplyRules(t *testing.T) {
	storage := &FileObjectStorage{}

	baseEntry := map[string]any{
		objects.FieldKeyID:             "KEY-test-1",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyAccountID:      "ACC-alice",
		objects.FieldKeyCredentialHash: "secret-hash-12345",
		objects.FieldKeySalt:           "random-salt-67890",
		"public_meta":                  "allowed-info",
	}

	// Case 1: Nil security context returns only ID and kind
	resNil := storage.applyKeystoreAccessControl(baseEntry, nil)
	require.Equal(t, "KEY-test-1", resNil[objects.FieldKeyID])
	require.Equal(t, objects.KindKeystoreEntry, resNil[objects.FieldKeyKind])
	require.Nil(t, resNil[objects.FieldKeyAccountID])
	require.Nil(t, resNil[objects.FieldKeyCredentialHash])

	// Case 2: System account can see everything including hash and salt
	sysCtx := pkgctx.NewSystemSecurityContext()
	resSys := storage.applyKeystoreAccessControl(baseEntry, sysCtx)
	require.Equal(t, "secret-hash-12345", resSys[objects.FieldKeyCredentialHash])
	require.Equal(t, "random-salt-67890", resSys[objects.FieldKeySalt])
	require.Equal(t, "allowed-info", resSys["public_meta"])

	// Case 3: Admin role can see entry but sensitive fields are hidden
	adminCtx := pkgctx.NewSecurityContext("ACC-admin", []string{"admin"}, nil)
	resAdmin := storage.applyKeystoreAccessControl(baseEntry, adminCtx)
	require.Equal(t, "allowed-info", resAdmin["public_meta"])
	require.Nil(t, resAdmin[objects.FieldKeyCredentialHash])
	require.Nil(t, resAdmin[objects.FieldKeySalt])

	// Case 4: Owner account can see own entry but sensitive fields are hidden
	ownerCtx := pkgctx.NewSecurityContext("ACC-alice", []string{"developer"}, nil)
	resOwner := storage.applyKeystoreAccessControl(baseEntry, ownerCtx)
	require.Equal(t, "allowed-info", resOwner["public_meta"])
	require.Nil(t, resOwner[objects.FieldKeyCredentialHash])
	require.Nil(t, resOwner[objects.FieldKeySalt])

	// Case 5: Non-owner non-admin gets minimal object (access denied)
	strangerCtx := pkgctx.NewSecurityContext("ACC-bob", []string{"developer"}, nil)
	resStranger := storage.applyKeystoreAccessControl(baseEntry, strangerCtx)
	require.Equal(t, "KEY-test-1", resStranger[objects.FieldKeyID])
	require.Equal(t, objects.KindKeystoreEntry, resStranger[objects.FieldKeyKind])
	require.Nil(t, resStranger["public_meta"])
	require.Nil(t, resStranger[objects.FieldKeyCredentialHash])
}

func TestKeystoreAccessControl_PrepareEntryValidations(t *testing.T) {
	storage := &FileObjectStorage{}

	// Case 1: Injects AccountID from security context if omitted
	userCtx := pkgctx.NewSecurityContext("ACC-alice", []string{"developer"}, nil)
	entry1 := map[string]any{
		objects.FieldKeyID:   "KEY-1",
		objects.FieldKeyKind: objects.KindKeystoreEntry,
	}
	require.NoError(t, storage.prepareKeystoreEntry(entry1, userCtx))
	require.Equal(t, "ACC-alice", entry1[objects.FieldKeyAccountID])

	// Case 2: Error when account_id is missing and context is nil or system
	entry2 := map[string]any{
		objects.FieldKeyID:   "KEY-2",
		objects.FieldKeyKind: objects.KindKeystoreEntry,
	}
	err := storage.prepareKeystoreEntry(entry2, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrMsgAccountIDRequired)

	// Case 3: Non-system user trying to set credential_hash is rejected
	entry3 := map[string]any{
		objects.FieldKeyID:             "KEY-3",
		objects.FieldKeyAccountID:      "ACC-alice",
		objects.FieldKeyCredentialHash: "malicious-hash",
	}
	err = storage.prepareKeystoreEntry(entry3, userCtx)
	require.Error(t, err)

	// Case 4: Non-system user trying to set salt is rejected
	entry4 := map[string]any{
		objects.FieldKeyID:        "KEY-4",
		objects.FieldKeyAccountID: "ACC-alice",
		objects.FieldKeySalt:      "malicious-salt",
	}
	err = storage.prepareKeystoreEntry(entry4, userCtx)
	require.Error(t, err)

	// Case 5: System account setting hash and salt succeeds
	sysCtx := pkgctx.NewSystemSecurityContext()
	entry5 := map[string]any{
		objects.FieldKeyID:             "KEY-5",
		objects.FieldKeyAccountID:      "ACC-alice",
		objects.FieldKeyCredentialHash: "sys-hash",
		objects.FieldKeySalt:           "sys-salt",
	}
	require.NoError(t, storage.prepareKeystoreEntry(entry5, sysCtx))
}
