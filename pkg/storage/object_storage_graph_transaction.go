package storage

import (
	"context"

	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// BeginTransaction starts a transaction for atomic multi-object operations
func (g *GraphObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	graphTx, err := g.conn.BeginTransaction(ctx)
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.BeginTransaction", err).Log()
		}
		return nil, errfmt.Newf(ConstStreamFailedToBeginTransaction).Wrap(err)
	}

	return &GraphObjectTransaction{
		storage: g,
		graphTx: graphTx,
		ctx:     ctx,
	}, nil
}

// GraphObjectTransaction implements ObjectTransaction for graph storage
type GraphObjectTransaction struct {
	storage ObjectStorageProvider
	graphTx provider.GraphTransaction
	ctx     context.Context
}

// Create creates a new object within the transaction
func (tx *GraphObjectTransaction) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	// Use the storage's Create method, but it will use the transaction automatically
	// since GraphConnection methods check for open transactions
	return tx.storage.Create(tx.ctx, secCtx, obj)
}

// Read reads an object within the transaction
func (tx *GraphObjectTransaction) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return tx.storage.Read(tx.ctx, secCtx, id)
}

// Update updates an object within the transaction
func (tx *GraphObjectTransaction) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return tx.storage.Update(tx.ctx, secCtx, id, updates)
}

// Delete deletes an object within the transaction
func (tx *GraphObjectTransaction) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	// Read existing object to get kind
	existing, err := tx.storage.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}

	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}

	label := toLabel(kind)
	return tx.graphTx.DeleteNode(ctx, id, []string{label, "Entity"})
}

// Commit commits the transaction
func (tx *GraphObjectTransaction) Commit(ctx context.Context) error {
	return tx.graphTx.Commit(ctx)
}

// Rollback rolls back the transaction
func (tx *GraphObjectTransaction) Rollback(ctx context.Context) error {
	return tx.graphTx.Rollback(ctx)
}
