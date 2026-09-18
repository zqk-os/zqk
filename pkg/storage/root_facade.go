package storage

import (
	"github.com/zqk-os/zqk/pkg/storage/audit"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/storage/systemcheck"
)

// RootFacade is the file-store facade for CRIT-CEF-STORAGE-SUBPACKAGES-001:
// CRUD + CAS + audit writes. Aggregation, migration, and systemcheck stay
// services behind their subpackage interfaces (not extra methods here).
// Migration StorageFacade is asserted at the CLI migrate-cas/dsia call sites
// (pkg/storage cannot import pkg/storage/migration).
type RootFacade interface {
	crud.CRUDFacade
	caspkg.CASFacade
	audit.Writer
}

var (
	_ crud.CRUDFacade     = (*FileObjectStorage)(nil)
	_ caspkg.CASFacade    = (*FileObjectStorage)(nil)
	_ audit.Writer        = (*FileObjectStorage)(nil)
	_ RootFacade          = (*FileObjectStorage)(nil)
	_ systemcheck.Checker = systemcheck.DefaultChecker{}
	_ audit.Aggregator    = (*AuditAggregationService)(nil)
)
