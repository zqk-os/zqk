package scheduler

import (
	"context"
	"crypto/x509"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// AuthHook is an interface for authenticating callback listener requests
// This is stubbed for initial testing but should be implemented with proper
// JWT, x.509, or PKI-based authentication for production use
type AuthHook interface {
	// Authenticate validates the incoming request and returns:
	// - authenticated: true if request is authenticated
	// - subject: authenticated subject/principal identifier
	// - permissions: list of permissions granted to the subject
	// - error: any error encountered during authentication
	Authenticate(ctx context.Context, r *http.Request) (authenticated bool, subject string, permissions []string, err error)
}

// DenyAuthHook rejects every request. It backs unrecognized auth types so a
// config typo ("JWT", "jwtt") closes the endpoint instead of opening it.
type DenyAuthHook struct {
	authType string
}

// NewDenyAuthHook creates an auth hook that denies all requests.
func NewDenyAuthHook(authType string) AuthHook {
	return &DenyAuthHook{authType: authType}
}

// Authenticate always denies, naming the unsupported type for operators.
func (h *DenyAuthHook) Authenticate(ctx context.Context, r *http.Request) (authenticated bool, subject string, permissions []string, err error) {
	return false, "", nil, errfmt.Errorf("unknown auth type %q: denying request", h.authType)
}

// JWTAuthHook validates JWT tokens (HMAC with shared secret).
// Expects Authorization: Bearer <token>. Validates signature, issuer, and expiration;
// subject and optional permissions come from claims (sub, permissions or scope).
type JWTAuthHook struct {
	secretKey string
	issuer    string
}

// NewJWTAuthHook creates a new JWT auth hook for HMAC-signed tokens.
func NewJWTAuthHook(secretKey, issuer string) AuthHook {
	return &JWTAuthHook{
		secretKey: secretKey,
		issuer:    issuer,
	}
}

// jwtClaims extends RegisteredClaims with optional permissions.
type jwtClaims struct {
	jwt.RegisteredClaims
	Permissions []string `json:"permissions,omitempty"`
	Scope       string   `json:"scope,omitempty"`
}

// Authenticate validates JWT from Authorization: Bearer <token>.
// Uses HMAC (e.g. HS256) with secretKey, checks issuer and exp, returns sub and permissions from claims.
func (h *JWTAuthHook) Authenticate(ctx context.Context, r *http.Request) (authenticated bool, subject string, permissions []string, err error) {
	if h.secretKey == emptyValue {
		return false, "", nil, nil
	}
	auth := r.Header.Get("Authorization")
	if auth == emptyValue || !strings.HasPrefix(strings.TrimSpace(auth), "Bearer ") {
		return false, "", nil, nil
	}
	tokenStr := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if tokenStr == emptyValue {
		return false, "", nil, nil
	}

	claims := &jwtClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errfmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(h.secretKey), nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}))
	if err != nil || !token.Valid {
		return false, "", nil, nil
	}

	if h.issuer != emptyValue && claims.Issuer != h.issuer {
		return false, "", nil, nil
	}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return false, "", nil, nil
	}

	subject = claims.Subject
	if subject == emptyValue {
		return false, "", nil, nil
	}
	permissions = claims.Permissions
	if len(permissions) == 0 && claims.Scope != emptyValue {
		permissions = strings.Fields(strings.ReplaceAll(claims.Scope, ",", " "))
	}
	if len(permissions) == 0 {
		permissions = []string{"callback:write"}
	}
	return true, subject, permissions, nil
}

// X509AuthHook validates x.509 client certificates.
// Uses r.TLS.PeerCertificates, verifies the chain against the CA at caCertPath,
// checks validity (NotBefore/NotAfter), and maps the leaf cert subject to subject.
type X509AuthHook struct {
	caCertPath string
}

// NewX509AuthHook creates a new x.509 auth hook.
func NewX509AuthHook(caCertPath string) AuthHook {
	return &X509AuthHook{
		caCertPath: caCertPath,
	}
}

// Authenticate validates the client certificate from r.TLS.PeerCertificates.
func (h *X509AuthHook) Authenticate(ctx context.Context, r *http.Request) (authenticated bool, subject string, permissions []string, err error) {
	if h.caCertPath == emptyValue {
		return false, "", nil, nil
	}
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return false, "", nil, nil
	}

	caPEM, err := fileutil.ReadFile(h.caCertPath)
	if err != nil {
		return false, "", nil, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return false, "", nil, nil
	}

	leaf := r.TLS.PeerCertificates[0]
	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:       pool,
		CurrentTime: time.Now(),
	})
	if err != nil {
		return false, "", nil, nil
	}

	subject = leaf.Subject.String()
	if subject == emptyValue {
		// Fallback to first CN
		if len(leaf.Subject.Names) > 0 {
			for _, n := range leaf.Subject.Names {
				if n.Type.String() == "2.5.4.3" { // commonName
					if s, ok := n.Value.(string); ok {
						subject = s
						break
					}
				}
			}
		}
	}
	if subject == emptyValue {
		return false, "", nil, nil
	}
	return true, subject, []string{"callback:write"}, nil
}

// APIKeyAuthHook validates API keys via X-API-Key header or api_key query parameter.
type APIKeyAuthHook struct {
	validKeys map[string]string // key -> subject mapping
}

// NewAPIKeyAuthHook creates a new API key auth hook with the given key -> subject map.
func NewAPIKeyAuthHook(validKeys map[string]string) AuthHook {
	return &APIKeyAuthHook{
		validKeys: validKeys,
	}
}

// Authenticate validates API key from X-API-Key header or api_key query parameter.
// If both are empty, returns false. Otherwise uses the first non-empty value and looks it up
// in validKeys; on match returns true, subject, and default permissions (callback:write).
func (h *APIKeyAuthHook) Authenticate(ctx context.Context, r *http.Request) (authenticated bool, subject string, permissions []string, err error) {
	key := r.Header.Get("X-API-Key")
	if key == emptyValue {
		key = r.URL.Query().Get("api_key")
	}
	if key == emptyValue {
		return false, "", nil, nil
	}
	if h.validKeys == nil {
		return false, "", nil, nil
	}
	subject, ok := h.validKeys[key]
	if !ok || subject == emptyValue {
		return false, "", nil, nil
	}
	return true, subject, []string{"callback:write"}, nil
}

// OAuth2AuthHook is a stub for OAuth2 token introspection (not yet implemented).
type OAuth2AuthHook struct{}

// NewOAuth2AuthHook creates a new OAuth2 auth hook stub.
func NewOAuth2AuthHook() AuthHook {
	return &OAuth2AuthHook{}
}

// Authenticate returns not implemented. OAuth2 introspection can be added later.
func (h *OAuth2AuthHook) Authenticate(ctx context.Context, r *http.Request) (authenticated bool, subject string, permissions []string, err error) {
	return false, "", nil, errfmt.Errorf("OAuth2 authentication not yet implemented")
}

// createAuthHook creates an auth hook based on auth type and config
//
//nolint:unused // Helper retained for future auth hook wiring
func createAuthHook(authType string, authConfig map[string]any) AuthHook {
	switch authType {
	case "jwt":
		secretKey, _ := authConfig["secret_key"].(string)
		issuer, _ := authConfig["issuer"].(string)
		return NewJWTAuthHook(secretKey, issuer)
	case "x509":
		caCertPath, _ := authConfig["ca_cert_path"].(string)
		return NewX509AuthHook(caCertPath)
	case "api_key":
		keys, _ := authConfig["valid_keys"].(map[string]any)
		validKeys := make(map[string]string)
		for key, subject := range keys {
			if subjectStr, ok := subject.(string); ok {
				validKeys[key] = subjectStr
			}
		}
		return NewAPIKeyAuthHook(validKeys)
	case "oauth2":
		return NewOAuth2AuthHook()
	case "none", "stub", "stub_auth":
		return NewDenyAuthHook(authType)
	default:
		// Unknown or empty auth type: deny (K:F-L-RELIABILITY-002).
		return NewDenyAuthHook(authType)
	}
}
