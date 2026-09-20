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

// BLI-CEF-R28-SEC-STUB-AUTH-REMEDIATION-001: StubAuth residual remediation verification.
// Asserts that permissive stubs ("stub", "stubauth", "stub_auth", "mock") are rejected
// with a DenyAuthHook and produce explicit errors, and that production auth hooks
// (JWT, APIKey, X509) are instantiated correctly with non-nil authentication boundaries.
func TestCreateAuthHook_StubAuthRemediatedFailClosed(t *testing.T) {
	t.Parallel()

	// 1. Stub/mock variants must fail closed and never permit traffic
	stubVariants := []string{"stub", "stub_auth", "stubauth", "mock", "permissive", "none", ""}
	for _, variant := range stubVariants {
		t.Run("stub_variant_"+variant, func(t *testing.T) {
			t.Parallel()
			hook := createAuthHook(variant, nil)
			if hook == nil {
				t.Fatalf("createAuthHook(%q) returned nil, expected DenyAuthHook", variant)
			}
			auth, subj, perms, err := hook.Authenticate(context.Background(), newCallbackRequest(t))
			if auth {
				t.Errorf("createAuthHook(%q) authenticated=true, want fail-closed", variant)
			}
			if err == nil {
				t.Errorf("createAuthHook(%q) returned nil error, want explicit denial error", variant)
			}
			if subj != "" || perms != nil {
				t.Errorf("createAuthHook(%q) leaked subject=%q or perms=%v", variant, subj, perms)
			}
		})
	}

	// 2. Production auth hooks replace stub implementations
	t.Run("production_jwt_hook_instantiation", func(t *testing.T) {
		t.Parallel()
		hook := createAuthHook("jwt", map[string]any{"secret_key": "secret", "issuer": "iss"})
		if _, ok := hook.(*JWTAuthHook); !ok {
			t.Errorf("createAuthHook('jwt') type = %T, want *JWTAuthHook", hook)
		}
	})

	t.Run("production_apikey_hook_instantiation", func(t *testing.T) {
		t.Parallel()
		hook := createAuthHook("api_key", map[string]any{"valid_keys": map[string]any{"k1": "sub1"}})
		if _, ok := hook.(*APIKeyAuthHook); !ok {
			t.Errorf("createAuthHook('api_key') type = %T, want *APIKeyAuthHook", hook)
		}
	})

	t.Run("production_x509_hook_instantiation", func(t *testing.T) {
		t.Parallel()
		hook := createAuthHook("x509", map[string]any{"ca_cert_path": "/tmp/ca.pem"})
		if _, ok := hook.(*X509AuthHook); !ok {
			t.Errorf("createAuthHook('x509') type = %T, want *X509AuthHook", hook)
		}
	})
}
