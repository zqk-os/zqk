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

// ExportSnapshot dumps the entire graph state to a compressed snapshot
func (g *GraphObjectStorage) ExportSnapshot(ctx context.Context, secCtx *pkgctx.SecurityContext, snapshotPath string) error {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	// 1. List all objects from graph
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Query all nodes using a direct Cypher query
	queryResult, err := g.Query(ctx, secCtx, &pkgctx.StorageContext{}, Query{
		Type:       QueryTypeCypher,
		Expression: "MATCH (n) RETURN n",
		Parameters: map[string]any{},
	})
	if err != nil {
		return errfmt.Newf("failed to query all nodes for snapshot").Wrap(err)
	}

	if len(queryResult.Objects) == 0 {
		return errfmt.Errorf("no objects found to snapshot")
	}

	// 2. Create compressed snapshot
	cs, err := CreateCompressedSnapshot(queryResult.Objects, time.Now(), logger)
	if err != nil {
		return errfmt.Newf("failed to create compressed snapshot").Wrap(err)
	}

	// 3. Write to file
	if err := WriteCompressedSnapshot(cs, snapshotPath); err != nil {
		return errfmt.Newf("failed to write compressed snapshot").Wrap(err)
	}

	return nil
}

// ImportSnapshot loads a compressed snapshot directly into MemGraph
func (g *GraphObjectStorage) ImportSnapshot(ctx context.Context, secCtx *pkgctx.SecurityContext, snapshotPath string) error {
	ctx, cancel := pkgctx.EnforceTimeout(ctx, 30*time.Second)
	defer cancel()
	cs, err := ReadCompressedSnapshot(snapshotPath)
	if err != nil {
		return errfmt.Newf("failed to read compressed snapshot").Wrap(err)
	}

	expanded, err := cs.Expand()
	if err != nil {
		return errfmt.Newf("failed to expand snapshot").Wrap(err)
	}

	// Bulk create the objects. We use raw GraphConnection to bypass heavy validation and hook triggers for speed during bulk load
	pool, err := g.conn.(interface {
		GetPool(context.Context) (provider.ConnectionPool, error)
	}).GetPool(ctx)
	if err != nil {
		// Fallback to BulkCreate if we cannot get the raw pool
		_, err = g.BulkCreate(ctx, secCtx, expanded)
		return err
	}
	defer pool.Close()

	conn, err := pool.GetConnection(ctx)
	if err != nil {
		return errfmt.Newf("failed to get connection for bulk import").Wrap(err)
	}
	defer pool.ReturnConnection(conn)

	// Since we are hydrating, we can just run a big transaction or individual inserts
	for _, obj := range expanded {
		kind, ok := obj[objects.FieldKeyKind].(string)
		if !ok || kind == "" {
			continue
		}
		id, ok := obj[objects.FieldKeyID].(string)
		if !ok || id == "" {
			continue
		}

		node := provider.Node{
			ID:         id,
			Labels:     []string{kind},
			Properties: obj,
		}

		err = conn.CreateNode(ctx, node)
		if err != nil {
			err = conn.UpdateNode(ctx, id, provider.NodeUpdates{
				Properties: obj,
			})
			if err != nil {
				if isGraphRetryable(err) {
					logging.FluentEvent(logging.GetLogger()).Error("Transient error in GraphObjectStorage.ImportSnapshot UpdateNode", err).Log()
				}
				return errfmt.Newf("failed to import node %s", id).Wrap(err)
			}
		}
	}

	return nil
}
