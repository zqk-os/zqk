package pipeline

import "github.com/zqk-os/zqk/pkg/nildecode"

// DecodeNonNilPayload delegates to [nildecode.DecodeNonNilPayload] so pipeline call sites share the
// same semantics; implementation lives in pkg/nildecode to avoid import cycles with pkg/objects.
// Call sites that already import pipeline may keep using this wrapper; others may import
// pkg/nildecode directly to avoid depending on pipeline for decode-only use.
func DecodeNonNilPayload[P comparable](payload any) (P, bool) {
	return nildecode.DecodeNonNilPayload[P](payload)
}
