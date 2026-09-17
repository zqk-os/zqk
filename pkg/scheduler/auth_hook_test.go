package scheduler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/golang-jwt/jwt/v5"
)

func TestAPIKeyAuthHook_Authenticate_ValidKeyFromHeader(t *testing.T) {
	t.Parallel()
	validKeys := map[string]string{"secret-key-123": "subject:service-a"}
	hook := NewAPIKeyAuthHook(validKeys)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("X-API-Key", "secret-key-123")

	auth, subject, perms, err := hook.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth {
		t.Error("expected authenticated=true")
	}
	if subject != "subject:service-a" {
		t.Errorf("subject: got %q", subject)
	}
	if len(perms) != 1 || perms[0] != "callback:write" {
		t.Errorf("permissions: got %v", perms)
	}
}

func TestAPIKeyAuthHook_Authenticate_ValidKeyFromQuery(t *testing.T) {
	t.Parallel()
	validKeys := map[string]string{"qkey": "subject:query"}
	hook := NewAPIKeyAuthHook(validKeys)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback?api_key=qkey", nil)

	auth, subject, _, err := hook.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth {
		t.Error("expected authenticated=true")
	}
	if subject != "subject:query" {
		t.Errorf("subject: got %q", subject)
	}
}

func TestAPIKeyAuthHook_Authenticate_HeaderPreferredOverQuery(t *testing.T) {
	t.Parallel()
	validKeys := map[string]string{"header-key": "from-header", "query-key": "from-query"}
	hook := NewAPIKeyAuthHook(validKeys)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback?api_key=query-key", nil)
	req.Header.Set("X-API-Key", "header-key")

	auth, subject, _, _ := hook.Authenticate(context.Background(), req)
	if !auth || subject != "from-header" {
		t.Errorf("expected subject from header: got auth=%v subject=%q", auth, subject)
	}
}

func TestAPIKeyAuthHook_Authenticate_InvalidKey(t *testing.T) {
	t.Parallel()
	validKeys := map[string]string{"valid": "subject"}
	hook := NewAPIKeyAuthHook(validKeys)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("X-API-Key", "wrong-key")

	auth, subject, perms, err := hook.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if auth {
		t.Error("expected authenticated=false")
	}
	if subject != emptyValue || perms != nil {
		t.Errorf("expected empty subject and perms: got subject=%q perms=%v", subject, perms)
	}
}

func TestAPIKeyAuthHook_Authenticate_NoKey(t *testing.T) {
	t.Parallel()
	validKeys := map[string]string{"secret": "subject"}
	hook := NewAPIKeyAuthHook(validKeys)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when no key provided")
	}
}

func TestAPIKeyAuthHook_Authenticate_QueryOnly(t *testing.T) {
	t.Parallel()
	validKeys := map[string]string{"q": "subject"}
	hook := NewAPIKeyAuthHook(validKeys)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.URL, _ = url.Parse("https://example.com/callback?api_key=q")

	auth, subject, _, _ := hook.Authenticate(context.Background(), req)
	if !auth || subject != "subject" {
		t.Errorf("expected auth=true subject=subject: got auth=%v subject=%q", auth, subject)
	}
}

func TestAPIKeyAuthHook_Authenticate_NilValidKeys(t *testing.T) {
	t.Parallel()
	hook := NewAPIKeyAuthHook(nil)

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("X-API-Key", "any")

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when validKeys is nil")
	}
}

// --- JWT tests ---

func TestJWTAuthHook_Authenticate_ValidToken(t *testing.T) {
	t.Parallel()
	secret := "test-secret"
	issuer := "https://issuer.example.com"
	hook := NewJWTAuthHook(secret, issuer)

	claims := jwt.MapClaims{
		"sub": "subject:jwt-user",
		"iss": issuer,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	auth, subject, perms, err := hook.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth {
		t.Error("expected authenticated=true")
	}
	if subject != "subject:jwt-user" {
		t.Errorf("subject: got %q", subject)
	}
	if len(perms) != 1 || perms[0] != "callback:write" {
		t.Errorf("permissions: got %v", perms)
	}
}

func TestJWTAuthHook_Authenticate_NoBearer(t *testing.T) {
	t.Parallel()
	hook := NewJWTAuthHook("secret", "")

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when no Bearer")
	}
}

func TestJWTAuthHook_Authenticate_WrongSecret(t *testing.T) {
	t.Parallel()
	claims := jwt.MapClaims{"sub": "user", "exp": time.Now().Add(time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte("wrong-secret"))

	hook := NewJWTAuthHook("correct-secret", "")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when secret wrong")
	}
}

func TestJWTAuthHook_Authenticate_ExpiredToken(t *testing.T) {
	t.Parallel()
	secret := "secret"
	claims := jwt.MapClaims{"sub": "user", "exp": time.Now().Add(-time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(secret))

	hook := NewJWTAuthHook(secret, "")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when token expired")
	}
}

func TestJWTAuthHook_Authenticate_WrongIssuer(t *testing.T) {
	t.Parallel()
	secret := "secret"
	claims := jwt.MapClaims{"sub": "user", "iss": "other", "exp": time.Now().Add(time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(secret))

	hook := NewJWTAuthHook(secret, "required-issuer")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when issuer wrong")
	}
}

func TestJWTAuthHook_Authenticate_PermissionsFromClaims(t *testing.T) {
	t.Parallel()
	secret := "secret"
	hook := NewJWTAuthHook(secret, "")

	type customClaims struct {
		jwt.RegisteredClaims
		Permissions []string `json:"permissions"`
	}
	claims := &customClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Permissions: []string{"callback:write", "callback:read"},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	auth, subject, perms, err := hook.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth || subject != "user" {
		t.Errorf("auth=%v subject=%q", auth, subject)
	}
	if len(perms) != 2 || perms[0] != "callback:write" || perms[1] != "callback:read" {
		t.Errorf("permissions: got %v", perms)
	}
}

// --- x.509 tests ---

func TestX509AuthHook_Authenticate_NoTLS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	caPath := dir + "/ca.pem"
	_ = fileutil.WriteFile(caPath, []byte("not a cert"), paths.FilePerm644)

	hook := NewX509AuthHook(caPath)
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when no TLS")
	}
}

func TestX509AuthHook_Authenticate_EmptyCaCertPath(t *testing.T) {
	t.Parallel()
	hook := NewX509AuthHook("")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when caCertPath is empty")
	}
}

func TestX509AuthHook_Authenticate_ValidCert(t *testing.T) {
	t.Parallel()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate CA: %v", err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey leaf: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "callback-client"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate leaf: %v", err)
	}
	leafCert, _ := x509.ParseCertificate(leafDER)

	dir := t.TempDir()
	caPath := dir + "/ca.pem"
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := fileutil.WriteFile(caPath, caPEM, paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile CA: %v", err)
	}

	hook := NewX509AuthHook(caPath)
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leafCert}}

	auth, subject, perms, err := hook.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !auth {
		t.Error("expected authenticated=true")
	}
	if subject == emptyValue {
		t.Error("expected non-empty subject")
	}
	if len(perms) != 1 || perms[0] != "callback:write" {
		t.Errorf("permissions: got %v", perms)
	}
}

func TestX509AuthHook_Authenticate_InvalidCAFile(t *testing.T) {
	t.Parallel()
	hook := NewX509AuthHook("/nonexistent/ca.pem")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}

	auth, _, _, _ := hook.Authenticate(context.Background(), req)
	if auth {
		t.Error("expected authenticated=false when CA file missing")
	}
}

// --- OAuth2 stub test ---

func TestOAuth2AuthHook_Authenticate_NotImplemented(t *testing.T) {
	t.Parallel()
	hook := NewOAuth2AuthHook()
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/callback", nil)

	auth, subject, perms, err := hook.Authenticate(context.Background(), req)
	if err == nil {
		t.Error("expected error from OAuth2 stub")
	}
	if err != nil && err.Error() != "OAuth2 authentication not yet implemented" {
		t.Errorf("error: got %q", err.Error())
	}
	if auth {
		t.Error("expected authenticated=false")
	}
	if subject != emptyValue || perms != nil {
		t.Errorf("expected empty subject and perms: subject=%q perms=%v", subject, perms)
	}
}
