package scheduler

import (
	"context"
	"net/http"
	"testing"
)

func newCallbackRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

// A misconfigured or typo'd auth type ("JWT", "jwtt") must not authenticate the
// caller. StubAuthHook allows every request, so falling back to it turns a
// config typo into an open callback endpoint.
func TestCreateAuthHook_UnknownTypeFailsClosed(t *testing.T) {
	t.Parallel()

	hook := createAuthHook("totally_unknown_type", nil)
	if hook == nil {
		t.Fatal("createAuthHook returned nil for unknown type")
	}

	authenticated, subject, permissions, err := hook.Authenticate(context.Background(), newCallbackRequest(t))
	if authenticated {
		t.Error("unknown auth type authenticated the request; want fail-closed")
	}
	if err == nil {
		t.Error("unknown auth type returned no error; want fail-closed")
	}
	if subject != "" {
		t.Errorf("subject = %q, want empty", subject)
	}
	if permissions != nil {
		t.Errorf("permissions = %v, want nil", permissions)
	}
}

func TestCreateAuthHook_KnownTypesStillBuild(t *testing.T) {
	t.Parallel()

	for _, authType := range []string{"jwt", "x509", "api_key", "oauth2"} {
		t.Run(authType, func(t *testing.T) {
			t.Parallel()
			if hook := createAuthHook(authType, nil); hook == nil {
				t.Fatalf("createAuthHook(%q) = nil", authType)
			}
		})
	}
}

// "none" / "stub" fails closed (K:F-L-RELIABILITY-002).
func TestCreateAuthHook_NoneAndEmptyFailClosed(t *testing.T) {
	t.Parallel()

	for _, authType := range []string{"none", "stub", "stub_auth", ""} {
		t.Run("type_"+authType, func(t *testing.T) {
			t.Parallel()
			label := authType
			if label == "" {
				label = "empty"
			}
			hook := createAuthHook(authType, nil)
			authenticated, _, _, err := hook.Authenticate(context.Background(), newCallbackRequest(t))
			if authenticated {
				t.Errorf("createAuthHook(%q) authenticated; want fail-closed", label)
			}
			if err == nil {
				t.Errorf("createAuthHook(%q) returned nil error; want fail-closed", label)
			}
		})
	}
}
