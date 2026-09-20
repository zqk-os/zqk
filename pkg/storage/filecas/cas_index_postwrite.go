package filecas

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ensureCASIndexMatchesContentHash enforces the Tier-1 post-write invariant:
// after Create/Update, ID→index hash must equal sha256(payload). On drift, repair
// with synchronous SetMapping (same effect as `zqk system sync-cas-index`).
//
func (cas *ContentAddressableStorage) EnsureCASIndexMatchesContentHash(objectID string, data []byte, bucketKey string) error {
	if cas == nil || cas.index == nil || objectID == emptyValue || len(data) == 0 {
		return nil
	}
	want := CalculateSHA256Hash(data)
	got, err := cas.index.GetHash(objectID)
	if err == nil && got == want {
		return nil
	}

	setArgs := []string{}
	if bucketKey != emptyValue {
		setArgs = append(setArgs, bucketKey)
	}
	if setErr := cas.index.SetMapping(objectID, want, setArgs...); setErr != nil {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageCASIndexPostWriteRepairFailedWarn).
			Kind(cas.kind).
			ObjectID(objectID).
			WithError(setErr).
			Log()
		return errfmt.Newf(ConstMiscFailedToUpdateCasIndex).Wrap(setErr)
	}
	if bucketKey != emptyValue {
		cas.SetIndexMappingInMemory(objectID, want, bucketKey)
	} else {
		cas.SetIndexMappingInMemory(objectID, want)
	}
	StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
		Warn(LogEventStorageCASIndexPostWriteRepairedWarn).
		Kind(cas.kind).
		ObjectID(objectID).
		String("want_hash_prefix", hashPrefixForLog(want)).
		String("got_hash_prefix", hashPrefixForLog(got)).
		Log()
	return nil
}

func hashPrefixForLog(h string) string {
	if h == emptyValue {
		return "(missing)"
	}
	if len(h) > 16 {
		return h[:16]
	}
	return h
}
