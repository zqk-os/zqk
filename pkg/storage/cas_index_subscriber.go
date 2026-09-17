package storage

import (
	"context"
	"strings"

	"github.com/lanceman/zqk/pkg/storage/filecas"
)

// CASIndexInvalidationSubscriber listens for mutation events on the InvalidationShockwaveBus
// and immediately updates the in-memory CAS index for the corresponding kind.
// TRACK: BLI-1789165691528268000-7bb48f71
type CASIndexInvalidationSubscriber struct {
	cas *filecas.ContentAddressableStorage
}

// NewCASIndexInvalidationSubscriber creates a new subscriber for the given CAS instance.
func NewCASIndexInvalidationSubscriber(cas *filecas.ContentAddressableStorage) *CASIndexInvalidationSubscriber {
	return &CASIndexInvalidationSubscriber{cas: cas}
}

// HandleMutation updates the in-memory CAS index upon receiving an object mutation event.
func (s *CASIndexInvalidationSubscriber) HandleMutation(ctx context.Context, event MutationEvent) error {
	if s.cas == nil {
		return nil
	}
	if event.Kind != "" && !strings.EqualFold(s.cas.GetKind(), event.Kind) {
		return nil
	}
	if event.Path != "" && s.cas.GetKindDir() != "" && !strings.HasPrefix(event.Path, s.cas.GetKindDir()) {
		return nil
	}
	switch event.Op {
	case MutationOpDelete:
		s.cas.RemoveIndexMappingInMemory(event.ID)
	case MutationOpPut:
		if event.NewHash != "" && filecas.CasHashFilenameRe.MatchString(event.NewHash+".yaml") {
			s.cas.SetIndexMappingInMemory(event.ID, event.NewHash)
		}
	}
	return nil
}
