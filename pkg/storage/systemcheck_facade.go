package storage

import "github.com/zqk-os/zqk/pkg/storage/systemcheck"

// SystemCheckFacade is the storage-root alias for the systemcheck Checker.
// The scan itself lives in pkg/storage/systemcheck, not on FileObjectStorage.
type SystemCheckFacade = systemcheck.Checker
