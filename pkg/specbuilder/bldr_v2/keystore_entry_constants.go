package bldr_v2

// Field name constants for keystore_entry objects
// These constants are generated from fields defined in THIS spec (not inherited)
// Inherited fields from base_object are defined in base_object_constants.go (same package)
// All constants are in package bldr_v2, so you can access inherited fields via bldr_v2.FieldX
const (
	// FieldAccountId is the field name for account_id
	FieldAccountId = "account_id"
	// FieldCredentialHash is the field name for credential_hash
	FieldCredentialHash = "credential_hash" //nolint:gosec
	// KeystoreEntryFieldDescription is the field name for description
	KeystoreEntryFieldDescription = "description"
	// KeystoreEntryFieldExpiresAt is the field name for expires_at
	KeystoreEntryFieldExpiresAt = "expires_at"
	// FieldKeyType is the field name for key_type
	FieldKeyType = "key_type"
	// FieldLastUsedAt is the field name for last_used_at
	FieldLastUsedAt = "last_used_at"
	// FieldRevoked is the field name for revoked
	FieldRevoked = "revoked"
	// FieldRevokedAt is the field name for revoked_at
	FieldRevokedAt = "revoked_at"
	// FieldSalt is the field name for salt
	FieldSalt = "salt"
)
