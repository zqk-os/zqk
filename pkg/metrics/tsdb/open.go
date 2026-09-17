package tsdb

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// Open returns a MemoryTSDB when root is empty or ":memory:", otherwise a FileTSDB under root.
// TRACK: BLI-1783822950016030000-81da5812
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
