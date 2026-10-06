package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// hasNestedSchedulerJobChurnID reports legacy recursive ids: SCH-<ts>-scheduler-job-<parent id>
// where parent id already contained another scheduler-job segment.
func hasNestedSchedulerJobChurnID(id string) bool {
	if id == emptyValue {
		return false
	}
	if strings.Count(id, "scheduler-job") > 1 {
		return true
	}
	return strings.Contains(id, ConstMiscSchedulerJobSch)
}

var schedJobLookupTable = func() [256]byte {
	var tbl [256]byte
	for i := 0; i < 256; i++ {
		tbl[i] = byte(i)
	}
	return tbl
}()

func sanitizeSchedKey(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	clean := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		clean[i] = schedJobLookupTable[s[i]]
	}
	return clean
}

// normalizeSchedulerJobIDIfRecursive replaces recursive / chained churn-style ids with a short
// SCH-<unix>-h-<12 hex> form derived from a hash of the original id (same kind + id always maps
// to the same suffix for a given second). Does not mutate when the id is already safe.
func normalizeSchedulerJobIDIfRecursive(kind, id string) string {
	if kind != objects.KindSchedulerJob || id == emptyValue || !hasNestedSchedulerJobChurnID(id) {
		return id
	}
	ts := time.Now().Unix()
	clean := sanitizeSchedKey(kind + "\x00" + id)
	h := sha256.Sum256(clean)
	suf := hex.EncodeToString(h[:6])
	return fmt.Sprintf("SCH-%d-h-%s", ts, suf)
}
