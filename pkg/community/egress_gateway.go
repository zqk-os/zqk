package community

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// ClassificationLevel represents a data sovereignty sensitivity tier.
type ClassificationLevel string

const (
	// ClassificationPublic marks data approved for unrestricted external distribution.
	ClassificationPublic ClassificationLevel = "public"

	// ClassificationInternal marks operational data restricted to authorized cluster peers.
	ClassificationInternal ClassificationLevel = "internal"

	// ClassificationConfidential marks sensitive data requiring redaction and clearance tokens.
	ClassificationConfidential ClassificationLevel = "confidential"

	// ClassificationRestricted marks sovereign data that must never leave the local boundary.
	ClassificationRestricted ClassificationLevel = "restricted"

	minEgressSigningKeyBytes = 16
	tokenEntropyBytes        = 16
	redactedMarker           = "[REDACTED]"

	alertSeverityHigh     = "high"
	alertSeverityCritical = "critical"

	egressReasonAllowed            = "allowed"
	egressReasonRestricted         = "restricted classification intercepted"
	egressReasonUnauthorizedDomain = "unauthorized destination domain"
	egressReasonClusterMismatch    = "cluster ID mismatch"
	egressReasonMissingToken       = "missing egress clearance token"
	egressReasonInvalidToken       = "invalid egress clearance token"
	egressReasonExpiredToken       = "expired egress clearance token"
	egressReasonDigestMismatch     = "payload digest mismatch"

	redactCategoryPrivateKey = "private_key"
	redactCategoryBearer     = "bearer_token"
	redactCategoryAPIKey     = "api_key"
	redactCategorySecretKV   = "secret_kv"
	redactCategorySSN        = "ssn_pattern"
)

const (
	pemPrivateKeySuffix = "PRIVATE KEY-----"
	pemBeginHeader      = "-----BEGIN "
	pemEndFooter        = "-----END "
)

var (
	// ErrInvalidClassification indicates an unrecognized or blank sovereignty classification label.
	ErrInvalidClassification = errors.New("invalid sovereignty classification level")

	// ErrInvalidDestinationDomain indicates a blank or malformed destination domain identifier.
	ErrInvalidDestinationDomain = errors.New("invalid destination domain")

	// ErrInvalidTransitPolicy indicates a malformed or contradictory transit rule or policy.
	ErrInvalidTransitPolicy = errors.New("invalid sovereignty transit policy")

	// ErrInvalidSigningKey indicates a missing or undersized cryptographic signing key.
	ErrInvalidSigningKey = errors.New("invalid egress signing key")

	// ErrRestrictedEgress indicates outbound transit was blocked by sovereignty policy.
	ErrRestrictedEgress = errors.New("egress blocked by sovereignty policy")

	// ErrUnauthorizedDomain indicates the destination domain is not permitted by policy.
	ErrUnauthorizedDomain = errors.New("unauthorized egress destination domain")

	// ErrInvalidEgressToken indicates a missing, malformed, or forged clearance token.
	ErrInvalidEgressToken = errors.New(egressReasonInvalidToken)

	// ErrExpiredEgressToken indicates the clearance token has passed its expiration timestamp.
	ErrExpiredEgressToken = errors.New(egressReasonExpiredToken)

	// ErrPayloadDigestMismatch indicates the payload digest does not match the clearance token.
	ErrPayloadDigestMismatch = errors.New(egressReasonDigestMismatch)

	domainLabelRegexp = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

	redactionPatterns = []struct {
		category string
		pattern  *regexp.Regexp
	}{
		{
			category: redactCategoryPrivateKey,
			pattern:  regexp.MustCompile(`(?s)` + pemBeginHeader + `[A-Z0-9 ]*` + pemPrivateKeySuffix + `.*?` + pemEndFooter + `[A-Z0-9 ]*` + pemPrivateKeySuffix),
		},
		{
			category: redactCategoryBearer,
			pattern:  regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9\-._~+/]+=*`),
		},
		{
			category: redactCategoryAPIKey,
			pattern:  regexp.MustCompile(`\b(?:sk-[A-Za-z0-9]{12,}|AKIA[0-9A-Z]{16})\b`),
		},
		{
			category: redactCategorySecretKV,
			pattern:  regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|password|token)\s*[:=]\s*[^\s,;]+`),
		},
		{
			category: redactCategorySSN,
			pattern:  regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		},
	}
)

// Validate verifies that the classification level is one of the canonical sovereignty tiers.
func (c ClassificationLevel) Validate() error {
	switch c {
	case ClassificationPublic, ClassificationInternal, ClassificationConfidential, ClassificationRestricted:
		return nil
	default:
		return ErrInvalidClassification
	}
}

// TransitRule defines egress handling requirements for a specific classification level.
type TransitRule struct {
	Classification   ClassificationLevel
	AllowEgress      bool
	RequireRedaction bool
	RequireToken     bool
}

// Validate enforces static invariants on a transit rule.
func (r TransitRule) Validate() error {
	if err := r.Classification.Validate(); err != nil {
		return err
	}
	if r.Classification == ClassificationRestricted && r.AllowEgress {
		return ErrInvalidTransitPolicy
	}
	return nil
}

// SovereigntyPolicy defines cluster boundary rules, allowed domains, and cryptographic signing material.
type SovereigntyPolicy struct {
	ClusterID      string
	AllowedDomains []string
	Rules          []TransitRule
	SigningKey     []byte
}

// Validate enforces static floor invariants on the sovereignty policy schema.
func (p SovereigntyPolicy) Validate() error {
	if strutil.IsBlank(p.ClusterID) {
		return ErrBlankClusterID
	}
	if len(p.SigningKey) < minEgressSigningKeyBytes {
		return ErrInvalidSigningKey
	}
	if len(p.AllowedDomains) == 0 {
		return ErrInvalidDestinationDomain
	}
	for _, domain := range p.AllowedDomains {
		if err := validateDestinationDomain(domain); err != nil {
			return err
		}
	}
	if len(p.Rules) == 0 {
		return ErrInvalidTransitPolicy
	}
	seen := make(map[ClassificationLevel]struct{}, len(p.Rules))
	for _, rule := range p.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		if _, exists := seen[rule.Classification]; exists {
			return ErrInvalidTransitPolicy
		}
		seen[rule.Classification] = struct{}{}
	}
	return nil
}

func validateDestinationDomain(domain string) error {
	if strutil.IsBlank(domain) || !domainLabelRegexp.MatchString(domain) {
		return ErrInvalidDestinationDomain
	}
	return nil
}

// RedactionResult summarizes sensitive credential scrubbing performed on an outbound payload.
type RedactionResult struct {
	RedactedPayload   string
	RedactionCount    int
	MatchedCategories []string
}

// RedactCredentials scrubs private keys, bearer tokens, API keys, secret key-values, and SSNs.
func RedactCredentials(payload string) RedactionResult {
	current := payload
	total := 0
	var categories []string
	for _, spec := range redactionPatterns {
		matches := spec.pattern.FindAllStringIndex(current, -1)
		if len(matches) == 0 {
			continue
		}
		total += len(matches)
		categories = append(categories, spec.category)
		current = spec.pattern.ReplaceAllString(current, redactedMarker)
	}
	return RedactionResult{
		RedactedPayload:   current,
		RedactionCount:    total,
		MatchedCategories: categories,
	}
}

// EgressClearanceToken binds an outbound payload digest, domain, classification, and expiry via HMAC-SHA256.
type EgressClearanceToken struct {
	TokenID        string
	ClusterID      string
	Domain         string
	Classification ClassificationLevel
	PayloadDigest  string
	IssuedAt       time.Time
	ExpiresAt      time.Time
	Signature      string
}

// EgressRequest describes an outbound payload attempting to cross the cluster sovereignty boundary.
type EgressRequest struct {
	ClusterID      string
	Domain         string
	Classification ClassificationLevel
	Payload        string
	Token          *EgressClearanceToken
}

// EgressDecision captures the outcome of policy evaluation, redaction, and token verification.
type EgressDecision struct {
	Allowed           bool
	Domain            string
	Classification    ClassificationLevel
	RedactedPayload   string
	RedactionCount    int
	MatchedCategories []string
	PayloadDigest     string
	Reason            string
}

// EgressAuditEvent records an immutable audit trail entry for each egress evaluation.
type EgressAuditEvent struct {
	Timestamp      time.Time
	ClusterID      string
	Domain         string
	Classification ClassificationLevel
	Allowed        bool
	RedactionCount int
	Reason         string
}

// IntrusionAlert records a structured security alert when restricted or unauthorized egress is attempted.
type IntrusionAlert struct {
	AlertID        string
	Timestamp      time.Time
	ClusterID      string
	Domain         string
	Classification ClassificationLevel
	Severity       string
	Reason         string
	PayloadDigest  string
}

// EgressTelemetry summarizes aggregate gateway counters.
type EgressTelemetry struct {
	TotalRequests    int
	AllowedCount     int
	InterceptedCount int
	RedactedCount    int
	AlertsCount      int
}

// EgressGateway enforces sovereignty policy, credential redaction, and cryptographic token gating.
type EgressGateway struct {
	mu             sync.RWMutex
	policy         SovereigntyPolicy
	allowedDomains map[string]struct{}
	rules          map[ClassificationLevel]TransitRule
	signingKey     []byte
	auditLog       []EgressAuditEvent
	alerts         []IntrusionAlert
	telemetry      EgressTelemetry
	nowFn          func() time.Time
}

// NewEgressGateway constructs a validated EgressGateway enforcing the given sovereignty policy.
func NewEgressGateway(policy SovereigntyPolicy) (*EgressGateway, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	domains := make(map[string]struct{}, len(policy.AllowedDomains))
	for _, d := range policy.AllowedDomains {
		domains[strings.ToLower(d)] = struct{}{}
	}
	rules := make(map[ClassificationLevel]TransitRule, len(policy.Rules))
	for _, r := range policy.Rules {
		rules[r.Classification] = r
	}
	keyCopy := make([]byte, len(policy.SigningKey))
	copy(keyCopy, policy.SigningKey)

	return &EgressGateway{
		policy:         policy,
		allowedDomains: domains,
		rules:          rules,
		signingKey:     keyCopy,
		nowFn:          time.Now,
	}, nil
}

// MintClearanceToken issues an HMAC-signed clearance token bound to domain, classification, and redacted payload digest.
func (g *EgressGateway) MintClearanceToken(
	domain string,
	classification ClassificationLevel,
	payload string,
	ttl time.Duration,
) (EgressClearanceToken, error) {
	if err := classification.Validate(); err != nil {
		return EgressClearanceToken{}, err
	}
	if classification == ClassificationRestricted {
		return EgressClearanceToken{}, ErrRestrictedEgress
	}
	if err := validateDestinationDomain(domain); err != nil {
		return EgressClearanceToken{}, err
	}
	normDomain := strings.ToLower(domain)
	if !g.isDomainAllowed(normDomain) {
		return EgressClearanceToken{}, ErrUnauthorizedDomain
	}
	if ttl <= 0 {
		return EgressClearanceToken{}, ErrInvalidTransitPolicy
	}

	redacted := RedactCredentials(payload)
	digest := computePayloadDigest(redacted.RedactedPayload)
	tokenID, err := randomHexID()
	if err != nil {
		return EgressClearanceToken{}, err
	}
	now := g.nowFn().UTC()
	tok := EgressClearanceToken{
		TokenID:        tokenID,
		ClusterID:      g.policy.ClusterID,
		Domain:         normDomain,
		Classification: classification,
		PayloadDigest:  digest,
		IssuedAt:       now,
		ExpiresAt:      now.Add(ttl),
	}
	tok.Signature = g.signToken(tok)
	return tok, nil
}

// GateEgress evaluates an outbound request, redacts credentials, verifies clearance tokens, and intercepts violations.
func (g *EgressGateway) GateEgress(req EgressRequest) (EgressDecision, error) {
	redacted := RedactCredentials(req.Payload)
	digest := computePayloadDigest(redacted.RedactedPayload)
	normDomain := strings.ToLower(strings.TrimSpace(req.Domain))

	decision := EgressDecision{
		Domain:            normDomain,
		Classification:    req.Classification,
		RedactedPayload:   redacted.RedactedPayload,
		RedactionCount:    redacted.RedactionCount,
		MatchedCategories: redacted.MatchedCategories,
		PayloadDigest:     digest,
	}

	if err := g.validateRequestEnvelope(req, normDomain, digest); err != nil {
		decision.Reason = err.Error()
		g.recordOutcome(req.ClusterID, normDomain, req.Classification, false, redacted.RedactionCount, decision.Reason)
		return decision, err
	}

	rule, ok := g.rules[req.Classification]
	if !ok || !rule.AllowEgress {
		g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityCritical, egressReasonRestricted, digest)
		decision.Reason = egressReasonRestricted
		g.recordOutcome(req.ClusterID, normDomain, req.Classification, false, redacted.RedactionCount, decision.Reason)
		return decision, ErrRestrictedEgress
	}

	if rule.RequireToken {
		if err := g.verifyClearanceToken(req.Token, normDomain, req.Classification, digest); err != nil {
			g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityHigh, err.Error(), digest)
			decision.Reason = err.Error()
			g.recordOutcome(req.ClusterID, normDomain, req.Classification, false, redacted.RedactionCount, decision.Reason)
			return decision, err
		}
	}

	decision.Allowed = true
	decision.Reason = egressReasonAllowed
	g.recordOutcome(req.ClusterID, normDomain, req.Classification, true, redacted.RedactionCount, egressReasonAllowed)
	return decision, nil
}

func (g *EgressGateway) validateRequestEnvelope(req EgressRequest, normDomain, digest string) error {
	if strutil.IsBlank(req.ClusterID) {
		return ErrBlankClusterID
	}
	if req.ClusterID != g.policy.ClusterID {
		g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityHigh, egressReasonClusterMismatch, digest)
		return ErrClusterMismatch
	}
	if err := req.Classification.Validate(); err != nil {
		g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityHigh, err.Error(), digest)
		return err
	}
	if req.Classification == ClassificationRestricted {
		g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityCritical, egressReasonRestricted, digest)
		return ErrRestrictedEgress
	}
	if err := validateDestinationDomain(normDomain); err != nil {
		g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityHigh, egressReasonUnauthorizedDomain, digest)
		return err
	}
	if !g.isDomainAllowed(normDomain) {
		g.recordAlert(req.ClusterID, normDomain, req.Classification, alertSeverityHigh, egressReasonUnauthorizedDomain, digest)
		return ErrUnauthorizedDomain
	}
	return nil
}

func (g *EgressGateway) verifyClearanceToken(
	tok *EgressClearanceToken,
	domain string,
	classification ClassificationLevel,
	digest string,
) error {
	if tok == nil || strutil.IsBlank(tok.TokenID) || strutil.IsBlank(tok.Signature) {
		return ErrInvalidEgressToken
	}
	if tok.ClusterID != g.policy.ClusterID {
		return ErrClusterMismatch
	}
	if strings.ToLower(tok.Domain) != domain || tok.Classification != classification {
		return ErrInvalidEgressToken
	}
	expectedSig := g.signToken(*tok)
	sigBytes, err := hex.DecodeString(tok.Signature)
	if err != nil {
		return ErrInvalidEgressToken
	}
	expectedBytes, err := hex.DecodeString(expectedSig)
	if err != nil || !hmac.Equal(sigBytes, expectedBytes) {
		return ErrInvalidEgressToken
	}
	if g.nowFn().UTC().After(tok.ExpiresAt) {
		return ErrExpiredEgressToken
	}
	if tok.PayloadDigest != digest {
		return ErrPayloadDigestMismatch
	}
	return nil
}

func (g *EgressGateway) isDomainAllowed(normDomain string) bool {
	_, ok := g.allowedDomains[normDomain]
	return ok
}

func (g *EgressGateway) signToken(tok EgressClearanceToken) string {
	mac := hmac.New(sha256.New, g.signingKey)
	canonical := fmt.Sprintf(
		"%s|%s|%s|%s|%s|%d|%d",
		tok.TokenID,
		tok.ClusterID,
		strings.ToLower(tok.Domain),
		tok.Classification,
		tok.PayloadDigest,
		tok.IssuedAt.UTC().UnixNano(),
		tok.ExpiresAt.UTC().UnixNano(),
	)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func (g *EgressGateway) recordOutcome(
	clusterID string,
	domain string,
	classification ClassificationLevel,
	allowed bool,
	redactionCount int,
	reason string,
) {
	event := EgressAuditEvent{
		Timestamp:      g.nowFn().UTC(),
		ClusterID:      clusterID,
		Domain:         domain,
		Classification: classification,
		Allowed:        allowed,
		RedactionCount: redactionCount,
		Reason:         reason,
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auditLog = append(g.auditLog, event)
	g.telemetry.TotalRequests++
	if allowed {
		g.telemetry.AllowedCount++
	} else {
		g.telemetry.InterceptedCount++
	}
	if redactionCount > 0 {
		g.telemetry.RedactedCount++
	}
}

func (g *EgressGateway) recordAlert(
	clusterID string,
	domain string,
	classification ClassificationLevel,
	severity string,
	reason string,
	digest string,
) {
	alertID, err := randomHexID()
	if err != nil {
		alertID = digest
	}
	alert := IntrusionAlert{
		AlertID:        alertID,
		Timestamp:      g.nowFn().UTC(),
		ClusterID:      clusterID,
		Domain:         domain,
		Classification: classification,
		Severity:       severity,
		Reason:         reason,
		PayloadDigest:  digest,
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.alerts = append(g.alerts, alert)
	g.telemetry.AlertsCount++
}

// AuditEvents returns a snapshot copy of all recorded egress audit events.
func (g *EgressGateway) AuditEvents() []EgressAuditEvent {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]EgressAuditEvent, len(g.auditLog))
	copy(out, g.auditLog)
	return out
}

// IntrusionAlerts returns a snapshot copy of all emitted intrusion alert records.
func (g *EgressGateway) IntrusionAlerts() []IntrusionAlert {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]IntrusionAlert, len(g.alerts))
	copy(out, g.alerts)
	return out
}

// Telemetry returns a snapshot of aggregate egress gateway counters.
func (g *EgressGateway) Telemetry() EgressTelemetry {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.telemetry
}

func computePayloadDigest(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func randomHexID() (string, error) {
	buf := make([]byte, tokenEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
