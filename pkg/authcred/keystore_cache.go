package authcred

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"gopkg.in/yaml.v3"
)

// KeystoreRecord is a parsed keystore YAML retained until the keystore dir mtime changes.
type KeystoreRecord struct {
	FileID string
	KeyID  string
	Entry  map[string]any
}

func (r KeystoreRecord) KeyType() string {
	s, _ := r.Entry[objects.FieldKeyKeyType].(string)
	return s
}

func (r KeystoreRecord) AccountID() string {
	s, _ := r.Entry[objects.FieldKeyAccountID].(string)
	return s
}

func (r KeystoreRecord) CredentialHash() string {
	s, _ := r.Entry[objects.FieldKeyCredentialHash].(string)
	return s
}

func (r KeystoreRecord) Revoked() bool {
	b, _ := r.Entry[objects.FieldKeyRevoked].(bool)
	return b
}

func (r KeystoreRecord) ExpiresAt() string {
	s, _ := r.Entry[objects.FieldKeyExpiresAt].(string)
	return s
}

// keystoreRecords is keyed by project root. Stamp is the keystore YAML dir.
var keystoreRecords stampmemo.Table[[]KeystoreRecord]

func parseKeystoreYAML(fileID string, data []byte) (KeystoreRecord, bool) {
	var entry map[string]any
	if yaml.Unmarshal(data, &entry) != nil {
		return KeystoreRecord{}, false
	}
	keyID, _ := entry[objects.FieldKeyID].(string)
	if keyID == "" {
		keyID = fileID
	}
	return KeystoreRecord{FileID: fileID, KeyID: keyID, Entry: entry}, true
}

// ListKeystoreRecords returns parsed keystore YAML for projectRoot.
func ListKeystoreRecords(projectRoot string) ([]KeystoreRecord, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return nil, errfmt.Errorf("project root not available")
	}
	dir := paths.KeystoreDirPath(projectRoot)
	return keystoreRecords.Load(projectRoot, stampmemo.Of(dir), func() ([]KeystoreRecord, error) {
		var out []KeystoreRecord
		err := forEachYAMLFile(dir, func(fileID string, data []byte) {
			if rec, ok := parseKeystoreYAML(fileID, data); ok {
				out = append(out, rec)
			}
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	})
}

// InvalidateKeystore drops the parsed keystore snapshot after a write.
func InvalidateKeystore(projectRoot string) {
	keystoreRecords.Delete(strings.TrimSpace(projectRoot))
}
