package authcred

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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

type keystoreSnap struct {
	mu       sync.Mutex
	dirMtime int64
	loaded   bool
	err      error
	records  []KeystoreRecord
}

var keystoreSnaps sync.Map

func keystoreCache(projectRoot string) *keystoreSnap {
	if existing, ok := keystoreSnaps.Load(projectRoot); ok {
		return existing.(*keystoreSnap)
	}
	fresh := &keystoreSnap{}
	actual, _ := keystoreSnaps.LoadOrStore(projectRoot, fresh)
	return actual.(*keystoreSnap)
}

// ListKeystoreRecords returns parsed keystore YAML for projectRoot.
func ListKeystoreRecords(projectRoot string) ([]KeystoreRecord, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return nil, errfmt.Errorf("project root not available")
	}
	dir := paths.KeystoreDirPath(projectRoot)
	var dirMtime int64
	if info, err := fileutil.Stat(dir); err == nil {
		dirMtime = info.ModTime().UnixNano()
	}
	snap := keystoreCache(projectRoot)
	snap.mu.Lock()
	defer snap.mu.Unlock()
	if snap.loaded && snap.dirMtime == dirMtime {
		return snap.records, snap.err
	}
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		snap.loaded = true
		snap.dirMtime = dirMtime
		snap.records = nil
		snap.err = err
		return nil, err
	}
	out := make([]KeystoreRecord, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), paths.YAMLExtension) {
			continue
		}
		data, readErr := fileutil.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue
		}
		var entry map[string]any
		if yaml.Unmarshal(data, &entry) != nil {
			continue
		}
		fileID := strings.TrimSuffix(e.Name(), paths.YAMLExtension)
		keyID, _ := entry[objects.FieldKeyID].(string)
		if keyID == "" {
			keyID = fileID
		}
		out = append(out, KeystoreRecord{FileID: fileID, KeyID: keyID, Entry: entry})
	}
	snap.loaded = true
	snap.dirMtime = dirMtime
	snap.err = nil
	snap.records = out
	return out, nil
}

// InvalidateKeystore drops the parsed keystore snapshot after a write.
func InvalidateKeystore(projectRoot string) {
	keystoreSnaps.Delete(strings.TrimSpace(projectRoot))
}
