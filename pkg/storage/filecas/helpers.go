package filecas

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

func CalculateSHA256Hash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func StorageLog(logger logging.Logger) *logging.FluentRoot {
	return logging.Fluent(logger)
}

func projectRootFromCASKindDir(kindDir string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(kindDir)))
}

var GetCASPendingVisibilityCache func(dir string) PendingVisibilityCache

type PendingVisibilityCache interface {
	LookupPending(id string) (PendingVisibilityEntry, bool)
	PublishPending(objectID, kind, hash, bucketKey string) error
	EvictPending(objectID string) error
	SnapshotPending() []PendingVisibilityEntry
}

type PendingVisibilityEntry struct {
	ObjectID  string
	Hash      string
	BucketKey string
	Kind      string
}

func VerifyContentHash(content []byte, expectedHash string) error {
	actual := CalculateSHA256Hash(content)
	if actual != expectedHash {
		return errfmt.Errorf(ErrMsgHashMismatchVerify, expectedHash, actual)
	}
	return nil
}

var RestorePendingAfterFailedMutation func(projectRoot, objectID, kind, oldHash, oldBucketKey string)

func projectRootFromCASIndexPath(indexPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(indexPath))))
}

var GetCacheOperationHandler func() func(*pkgctx.CacheContext) error
var NoteObjectIDCachePending func(projectRoot, op, id, kind, filePath, reason string)

var ConfirmPendingAfterDurableMapping func(indexPath, objectID, hash string)

var WarnOnceNilCacheHandler func(objectID, kind string)
