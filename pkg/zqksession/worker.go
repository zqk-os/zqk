package zqksession

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	enumzqksession "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/zqk_session"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
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
		func(builder instance_builders.InstanceBuilder) {
			builder.SetField(objects.FieldKeyAgentID, in.AgentID)
			if parent != EmptyValue {
				builder.SetField(objects.FieldKeyParentSessionRef, parent)
			}
			builder.SetField(objects.FieldKeyPersonaRef, in.PersonaRef)
			builder.SetField(objects.FieldKeyExecutorType, in.ExecutorType)
			builder.SetField(objects.FieldKeyProvider, in.Provider)
			builder.SetField(objects.FieldKeyProviderProfileRef, in.ProviderProfileRef)
			builder.SetField(objects.FieldKeyModelID, in.ModelID)
		},
	)
}

// persistableParentSessionRef returns parent only when it can survive persist-time
// reference validation. Readability is not enough: stream-backed zqk_session
// locators are segmentPath::offset, and Tier-2 os.Stat's that string as a file.
// Kind membership comes from high_volume_kinds.yaml via StreamStorageEnabledForKind.
// restore the ref when stream
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
