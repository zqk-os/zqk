package zqksession

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	enumzqksession "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/zqk_session"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/storage"
)

// WorkerSessionInput identifies an agent seat-worker session.
type WorkerSessionInput struct {
	ProjectRoot        string
	AccountID          string
	Title              string
	ParentSessionID    string
	AgentID            string
	PersonaRef         string
	ExecutorType       string
	Provider           string
	ProviderProfileRef string
	ModelID            string
}

// StartWorkerSession creates a fresh agent-worker session.
func StartWorkerSession(
	ctx context.Context,
	in WorkerSessionInput,
	sp storage.ObjectStorageProvider,
) (string, error) {
	in.AgentID = strings.TrimSpace(in.AgentID)
	if in.AgentID == EmptyValue {
		return EmptyValue, fmt.Errorf("agent ID is required")
	}
	if in.ExecutorType == EmptyValue {
		in.ExecutorType = ExecutorTypeAgentX
	}

	parent := persistableParentSessionRef(ctx, sp, in.ParentSessionID)

	return createSession(
		ctx,
		in.ProjectRoot,
		in.Title,
		in.AccountID,
		enumzqksession.SessionTypeAgentWorker,
		sp,
		func(builder *bldr_instance_v1.ZqkSessionInstanceBuilder) {
			builder.SetAgentId(in.AgentID)
			if parent != EmptyValue {
				builder.SetParentSessionRef(parent)
			}
			builder.SetPersonaRef(in.PersonaRef)
			builder.SetExecutorType(in.ExecutorType)
			builder.SetProvider(in.Provider)
			builder.SetProviderProfileRef(in.ProviderProfileRef)
			builder.SetModelId(in.ModelID)
		},
	)
}

// persistableParentSessionRef returns parent only when it can survive persist-time
// reference validation. Readability is not enough: stream-backed zqk_session
// locators are segmentPath::offset, and Tier-2 os.Stat's that string as a file.
// Kind membership comes from high_volume_kinds.yaml via StreamStorageEnabledForKind.
// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001 — restore the ref when stream
// locators are first-class persistable refs.
func persistableParentSessionRef(ctx context.Context, sp storage.ObjectStorageProvider, parent string) string {
	parent = strings.TrimSpace(parent)
	if parent == EmptyValue || sp == nil {
		return EmptyValue
	}
	if _, err := sp.Read(ctx, sessionStorageSecCtx(ctx), parent); err != nil {
		return EmptyValue
	}
	if storage.StreamStorageEnabledForKind(objects.KindZqkSession) {
		return EmptyValue
	}
	return parent
}
