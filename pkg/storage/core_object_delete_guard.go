package storage

import (
	"context"
	"fmt"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/paths"
)

// Core kernel kinds must not be hard-deleted without an explicit allow context.
// Archive → aggregate/compress with lineage is the durable path; raw Delete erases auditability.
// Incident: 2026-08-03 worktree lost 49 workstreams + criteria/questions with no archive lineage.
// TRACK: BLI-1785723654802038000-b14064bc — core hard-delete guard + archive-aggregate path.
// IsCoreKernelKind reports whether kind is a core process object that must not be
// silently erased (hard-delete, state-restore prune, etc.).
// Delegates to kernelcas.IsCriticalKind (Kernel Mutation Pipeline DECIDE policy).
func IsCoreKernelKind(kind string) bool {
	return kernelcas.IsCriticalKind(kind)
}

func isCoreKernelKind(kind string) bool {
	return IsCoreKernelKind(kind)
}

// WithTestHardDelete marks ctx as a CLI operation with explicit core hard-delete allow.
// Use in unit tests that assert Delete/cascade semantics for kernel-critical kinds.
// Production: CLI --reason-code → AllowCoreObjectDelete, or elevated delete:*/delete:core.
// TRACK: BLI-1785723654802038000-b14064bc — core hard-delete guard + archive-aggregate path.
func WithTestHardDelete(ctx context.Context) context.Context {
	return pkgctx.WithAllowCoreObjectDelete(WithCLIOperation(ctx))
}

// denyCoreKernelHardDelete fails closed on core kinds unless the caller has *declared intent* via
// AllowCoreObjectDelete (CLI --reason-code, or WithTestHardDelete in tests).
//
// Elevation deliberately does not satisfy this. It used to: any actor with delete:*, delete:core,
// the admin role, or the system account id could erase a kernel-critical object with no reason
// recorded. That made the guard structurally unable to stop the caller it most needed to stop,
// because every daemon is constructed elevated — NewSystemSecurityContext sets the system account
// id, the admin role, and delete:* all at once, so it satisfied three independent arms and removing
// any one of them changed nothing. The retention sweep of 2026-08-24 walked straight through and
// hard-deleted 270 archived kernel objects, leaving GhostRefs across the graph.
//
// Privilege answers "may you"; only a declared reason answers "did you mean to", and an automated
// sweep that is privileged by design can only be caught by the second question. Elevated actors are
// still allowed through — they just have to say so, which is what makes the erase auditable.
//
// Do not bypass on ZQK_TEST_ROOT alone: Local CI and scheduler bundlers set TestRoot on an
// already-initialized tree, which previously made these unit tests and CI falsely green.
// Isolated tests that need hard-delete must use WithTestHardDelete or pkgctx.WithAllowCoreObjectDelete.
// TRACK: BLI-1785723654802038000-b14064bc — core hard-delete guard + archive-aggregate path.
func denyCoreKernelHardDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id string) error {
	if !isCoreKernelKind(kind) {
		return nil
	}
	if pkgctx.GetAllowCoreObjectDelete(ctx) {
		return nil
	}
	// The message no longer offers an elevated account as a way through, because elevation is no
	// longer a way through. Naming one would send the reader to a door that does not open — the
	// mistake the agent guard made by advertising a bypass that relocated the project root.
	return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("hard delete refused for core kind %s (%s): promote/archive with aggregation, compression, and lineage instead of erasing CAS; break-glass: zqk object delete %s --reason-code \"…\" (min 30 chars). Elevation (%s / %s / admin / system account) does not substitute for a declared reason on core kinds: in-process callers must set pkgctx.WithAllowCoreObjectDelete, and tests storage.WithTestHardDelete", kind, id, id, pkgctx.PermissionDeleteAll, pkgctx.PermissionDeleteCore)))
}
