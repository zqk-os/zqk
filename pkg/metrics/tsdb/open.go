package tsdb

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Open returns a MemoryTSDB when root is empty or ":memory:", otherwise a FileTSDB under root.
// TRACK: follow-up in kernel backlog
func Open(root string) (TSDB, error) {
	r := strings.TrimSpace(root)
	if r == "" || strings.EqualFold(r, ":memory:") {
		return NewMemoryTSDB(), nil
	}
	db, err := OpenFileTSDB(r)
	if err != nil {
		return nil, errfmt.Newf("tsdb: open file").Wrap(err)
	}
	return db, nil
}
