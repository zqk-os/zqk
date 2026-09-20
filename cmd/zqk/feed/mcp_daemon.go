package feed

import (
	"context"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
)

func resolveFeedMCPTCP(raw string) string {
	return mcp.ResolveDaemonTCP(raw)
}

func queryMCPSubscriberCount(ctx context.Context, tcpAddr string, logger logging.Logger) (int, error) {
	pd := mcp.NewProxyDaemon(resolveFeedMCPTCP(tcpAddr), logger)
	qCtx, cancel := context.WithTimeout(ctx, mcp.DefaultFeedSteerMCPProbeTimeout)
	defer cancel()
	return pd.QueryEventsSubscriberCount(qCtx)
}

func publishMCPDaemonEvent(ctx context.Context, tcpAddr, message, agentID, eventID string, logger logging.Logger) error {
	pd := mcp.NewProxyDaemon(resolveFeedMCPTCP(tcpAddr), logger)
	pCtx, cancel := context.WithTimeout(ctx, mcp.DefaultFeedSteerMCPProbeTimeout)
	defer cancel()
	return pd.PublishDaemonEvent(pCtx, message, agentID, eventID)
}
