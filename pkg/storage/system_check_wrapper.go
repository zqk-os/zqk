package storage

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/systemcheck"
)

func (f *FileObjectStorage) RunSystemCheckForHandCASAndDupIDs(ctx context.Context) ([]*systemcheck.HandCASSystemCheckResult, error) {
	if f == nil {
		return nil, errfmt.Errorf("FileObjectStorage must not be nil")
	}

	var allResults []*systemcheck.HandCASSystemCheckResult

	for _, kind := range objects.GetGlobalKindMapper().GetAllKinds() {
		// Resolve path for this kind.
		kindDir := f.GetKindDir(kind)
		if kindDir == "" {
			continue
		}

		result, err := systemcheck.RunSystemCheckForHandCASAndDupIDs(ctx, kindDir, kind)
		if err != nil {
			// Missing kind dirs / transient FS errors must not abort the whole scan.
			continue
		}

		if result.Violations > 0 {
			allResults = append(allResults, result)
		}
	}

	return allResults, nil
}
