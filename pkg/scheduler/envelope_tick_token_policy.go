package scheduler

import (
	"encoding/json"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const (
	// EnvKeyEnvelopeTickDispatchDenyTokensJSON is JSON array of operational envelope discovery token strings.
	// Matching edges are excluded before job_type dedupe (see pkg/datacell ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy).
	EnvKeyEnvelopeTickDispatchDenyTokensJSON = "ENVELOPE_TICK_DISPATCH_DENY_TOKENS_JSON"
	// EnvKeyEnvelopeTickDispatchAllowTokensJSON is optional JSON array; when this key is present on the job,
	// only listed tokens contribute to resolved job types (restrictive whitelist). Empty array suppresses all token-derived types.
	EnvKeyEnvelopeTickDispatchAllowTokensJSON = "ENVELOPE_TICK_DISPATCH_ALLOW_TOKENS_JSON"
)

// envelopeTickTokenPolicyFromEnv parses optional JSON string arrays into sets. Missing keys leave sets nil and flags false.
// Empty JSON array yields empty map (non-nil) so callers can distinguish "key absent" vs "explicit empty list".
func envelopeTickTokenPolicyFromEnv(env map[string]string) (deny, allow map[string]struct{}, denyKeyPresent, allowKeyPresent bool, err error) {
	if env == nil {
		return nil, nil, false, false, nil
	}
	if raw, ok := env[EnvKeyEnvelopeTickDispatchDenyTokensJSON]; ok {
		denyKeyPresent = true
		deny, err = envelopeTickParseTokenJSONArray(raw)
		if err != nil {
			return nil, nil, true, false, errfmt.Newf("deny tokens JSON").Wrap(err)
		}
	}
	if raw, ok := env[EnvKeyEnvelopeTickDispatchAllowTokensJSON]; ok {
		allowKeyPresent = true
		allow, err = envelopeTickParseTokenJSONArray(raw)
		if err != nil {
			return deny, nil, denyKeyPresent, true, errfmt.Newf("allow tokens JSON").Wrap(err)
		}
	}
	return deny, allow, denyKeyPresent, allowKeyPresent, nil
}

func envelopeTickParseTokenJSONArray(raw string) (map[string]struct{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == emptyValue {
		return nil, errfmt.Errorf("envelope tick token JSON: empty value")
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(arr))
	for _, s := range arr {
		s = strings.TrimSpace(s)
		if s == emptyValue {
			continue
		}
		out[s] = struct{}{}
	}
	return out, nil
}
