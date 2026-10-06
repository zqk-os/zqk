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

// normalizeSchedulerJobIDIfRecursive replaces recursive / chained churn-style ids with a short
// SCH-<unix>-h-<12 hex> form derived from a hash of the original id (same kind + id always maps
// to the same suffix for a given second). Does not mutate when the id is already safe.
func normalizeSchedulerJobIDIfRecursive(kind, id string) string {
	if kind != objects.KindSchedulerJob || id == emptyValue || !hasNestedSchedulerJobChurnID(id) {
		return id
	}
	ts := time.Now().Unix()
	h := sha256.Sum256([]byte(kind + "\x00" + id))
	suf := hex.EncodeToString(h[:6])
	return fmt.Sprintf("SCH-%d-h-%s", ts, suf)
}
