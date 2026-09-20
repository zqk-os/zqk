package scheduler

import (
	"testing"
)

func TestEnvelopeTickParseTokenJSONArray(t *testing.T) {
	t.Parallel()
	got, err := envelopeTickParseTokenJSONArray(`["a", " b "]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if _, ok := got["a"]; !ok {
		t.Fatal("want a")
	}
	if _, ok := got["b"]; !ok {
		t.Fatal("want trimmed b")
	}
}

func TestEnvelopeTickTokenPolicyFromEnv_roundTrip(t *testing.T) {
	t.Parallel()
	envVars := make(map[string]string)
	envVars[EnvKeyEnvelopeTickDispatchDenyTokensJSON] = `["segment_rotation"]`
	envVars[EnvKeyEnvelopeTickDispatchAllowTokensJSON] = `[]`
	deny, allow, dK, aK, err := envelopeTickTokenPolicyFromEnv(envVars)
	if err != nil {
		t.Fatal(err)
	}
	if !dK || len(deny) != 1 {
		t.Fatalf("deny %v", deny)
	}
	if !aK || len(allow) != 0 {
		t.Fatalf("allow %v", allow)
	}
}

func TestEnvelopeTickTokenPolicyFromEnv_invalidJSON(t *testing.T) {
	t.Parallel()
	_, _, _, _, err := envelopeTickTokenPolicyFromEnv(map[string]string{
		EnvKeyEnvelopeTickDispatchDenyTokensJSON: `{`,
	})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestEnvelopeTickParseTokenJSONArray_emptyRaw(t *testing.T) {
	t.Parallel()
	_, err := envelopeTickParseTokenJSONArray("")
	if err == nil {
		t.Fatal("want error for empty raw")
	}
}

func TestEnvelopeTickTokenPolicyFromEnv_invalidDenySkipsAllowParse(t *testing.T) {
	t.Parallel()
	// Deny JSON fails first; allow branch is never reached (no partial allow map).
	deny, allow, denyKey, allowKey, err := envelopeTickTokenPolicyFromEnv(map[string]string{
		EnvKeyEnvelopeTickDispatchDenyTokensJSON:  `{`,
		EnvKeyEnvelopeTickDispatchAllowTokensJSON: `["a"]`,
	})
	if err == nil {
		t.Fatal("want error from deny JSON")
	}
	if deny != nil || allow != nil {
		t.Fatalf("want nil maps on deny parse error; deny=%v allow=%v", deny, allow)
	}
	if !denyKey {
		t.Fatal("deny env key was present")
	}
	if allowKey {
		t.Fatal("allow key should not be marked present when deny parse fails before allow branch")
	}
}
