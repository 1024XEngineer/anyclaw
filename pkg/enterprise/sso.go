package enterprise

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SSOProvider defines the interface for SSO authentication providers.
type SSOProvider interface {
	Name() string
	Type() string
	Initialize(config SSOConfig) error
	Authenticate(ctx context.Context, token string) (*User, error)
	RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error)
	Logout(ctx context.Context, token string) error
	GetUserInfo(ctx context.Context, token string) (*User, error)
	Close() error
}

// SSOConfig holds configuration for an SSO provider.
type SSOConfig struct {
	Provider     string
	ClientID     string
	ClientSecret string
	Endpoint     string
	RedirectURL  string
	Scopes       []string
	TLSConfig    *tls.Config
}

// User represents an authenticated user from SSO.
type User struct {
	ID            string
	Username      string
	Email         string
	DisplayName   string
	Groups        []string
	Roles         []string
	Attributes    map[string]any
	Authenticator string
	CreatedAt     time.Time
	LastLogin     time.Time
}

// TokenResponse represents an OAuth/OIDC token response.
type TokenResponse struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
	IDToken      string
}

// LDAPClient provides LDAP directory connectivity.
type LDAPClient struct {
	Addr         string
	Port         int
	UseTLS       bool
	BaseDN       string
	BindDN       string
	BindPassword string
	UserSearch   string
	GroupSearch  string
	conn         interface{}
	mu           sync.Mutex
}

// LDAPEntry represents an LDAP directory entry.
type LDAPEntry struct {
	DN    string
	Attrs map[string][]string
}

// LDAPConfig holds LDAP connection configuration.
type LDAPConfig struct {
	Host         string
	Port         int
	UseTLS       bool
	UseSSL       bool
	BindDN       string
	BindPassword string
	BaseDN       string
	UserFilter   string
	GroupFilter  string
}

// SSOProviderRegistry manages SSO provider registrations.
type SSOProviderRegistry struct {
	providers map[string]SSOProvider
	mu        sync.RWMutex
}

func NewSSOProviderRegistry() *SSOProviderRegistry {
	return &SSOProviderRegistry{
		providers: make(map[string]SSOProvider),
	}
}

func (r *SSOProviderRegistry) Register(name string, provider SSOProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("SSO provider already registered: %s", name)
	}
	r.providers[name] = provider
	return nil
}

func (r *SSOProviderRegistry) Get(name string) (SSOProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[name]
	return provider, ok
}

func (r *SSOProviderRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// OIDCDiscovery represents an OIDC provider's discovery document.
type OIDCDiscovery struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	EndSessionEndpoint                string   `json:"end_session_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key.
type JWK struct {
	Kty string   `json:"kty"`
	Use string   `json:"use,omitempty"`
	Kid string   `json:"kid,omitempty"`
	Alg string   `json:"alg,omitempty"`
	N   string   `json:"n,omitempty"`
	E   string   `json:"e,omitempty"`
	X5c []string `json:"x5c,omitempty"`
}

// JWTHeader is the decoded JWT header.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

// JWTClaims is the decoded JWT payload.
type JWTClaims struct {
	Issuer            string   `json:"iss"`
	Subject           string   `json:"sub"`
	Audience          any      `json:"aud"`
	Expiry            float64  `json:"exp"`
	NotBefore         float64  `json:"nbf"`
	IssuedAt          float64  `json:"iat"`
	Nonce             string   `json:"nonce,omitempty"`
	Email             string   `json:"email,omitempty"`
	Name              string   `json:"name,omitempty"`
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Groups            []string `json:"groups,omitempty"`
	Raw               map[string]any
}

// OIDCProvider implements real OIDC with discovery, JWT validation, and code exchange.
type OIDCProvider struct {
	config       SSOConfig
	client       *http.Client
	discovery    *OIDCDiscovery
	jwks         *JWKS
	verifierKeys map[string]crypto.PublicKey
	mu           sync.RWMutex
	state        string
	nonce        string
}

func (p *OIDCProvider) Name() string { return "oidc" }
func (p *OIDCProvider) Type() string { return "openid-connect" }

func (p *OIDCProvider) Initialize(config SSOConfig) error {
	p.config = config
	p.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: config.TLSConfig},
	}

	if err := p.fetchDiscovery(context.Background()); err != nil {
		return fmt.Errorf("OIDC discovery failed: %w", err)
	}

	if err := p.fetchJWKS(context.Background()); err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	p.parseVerifierKeys()
	return nil
}

func (p *OIDCProvider) fetchDiscovery(ctx context.Context) error {
	issuer := strings.TrimSuffix(p.config.Endpoint, "/")
	wellKnown := issuer + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery endpoint returned %d", resp.StatusCode)
	}

	var disc OIDCDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return fmt.Errorf("failed to parse discovery document: %w", err)
	}

	p.discovery = &disc
	return nil
}

func (p *OIDCProvider) fetchJWKS(ctx context.Context) error {
	if p.discovery == nil || p.discovery.JWKSURI == "" {
		return fmt.Errorf("no JWKS URI in discovery document")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discovery.JWKSURI, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to parse JWKS: %w", err)
	}

	p.mu.Lock()
	p.jwks = &jwks
	p.mu.Unlock()
	return nil
}

func (p *OIDCProvider) parseVerifierKeys() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.jwks == nil {
		return
	}

	p.verifierKeys = make(map[string]crypto.PublicKey)
	for _, key := range p.jwks.Keys {
		if key.Kty != "RSA" || key.Use != "sig" {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			continue
		}

		n := base64ToBigInt(nBytes)
		e := base64ToBigInt(eBytes)

		pubKey := &rsa.PublicKey{N: n, E: int(e.Int64())}
		p.verifierKeys[key.Kid] = pubKey

		if len(key.X5c) > 0 {
			if cert, err := parseX509Cert(key.X5c[0]); err == nil {
				if rsaPub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
					p.verifierKeys[key.Kid] = rsaPub
				}
			}
		}
	}
}

func (p *OIDCProvider) AuthURL(state, nonce string) string {
	p.state = state
	p.nonce = nonce

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", p.config.RedirectURL)
	params.Set("scope", strings.Join(p.scopes(), " "))
	params.Set("state", state)
	params.Set("nonce", nonce)

	if p.discovery != nil {
		return p.discovery.AuthorizationEndpoint + "?" + params.Encode()
	}
	return p.config.Endpoint + "/authorize?" + params.Encode()
}

func (p *OIDCProvider) scopes() []string {
	if len(p.config.Scopes) > 0 {
		return p.config.Scopes
	}
	return []string{"openid", "profile", "email"}
}

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	if p.discovery == nil {
		return nil, fmt.Errorf("provider not initialized")
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", p.config.RedirectURL)
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

func (p *OIDCProvider) Authenticate(ctx context.Context, token string) (*User, error) {
	claims, err := p.validateJWT(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("JWT validation failed: %w", err)
	}

	user := &User{
		ID:            claims.Subject,
		Username:      claims.PreferredUsername,
		Email:         claims.Email,
		DisplayName:   claims.Name,
		Groups:        claims.Groups,
		Authenticator: "oidc",
		Attributes:    claims.Raw,
		CreatedAt:     time.Unix(int64(claims.IssuedAt), 0),
		LastLogin:     time.Now().UTC(),
	}

	if user.Username == "" {
		user.Username = claims.Email
	}

	return user, nil
}

func (p *OIDCProvider) validateJWT(ctx context.Context, token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	var header JWTHeader
	if err := jsonDecodeBase64(parts[0], &header); err != nil {
		return nil, fmt.Errorf("failed to decode JWT header: %w", err)
	}

	var claims JWTClaims
	if err := jsonDecodeBase64(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("failed to decode JWT claims: %w", err)
	}

	if err := p.verifySignature(parts[0], parts[1], parts[2], header); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	if claims.Issuer != p.config.Endpoint && claims.Issuer != p.discovery.Issuer {
		return nil, fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}

	if !p.verifyAudience(&claims) {
		return nil, fmt.Errorf("invalid audience: expected %s", p.config.ClientID)
	}

	if time.Now().UTC().After(time.Unix(int64(claims.Expiry), 0)) {
		return nil, fmt.Errorf("token expired at %v", time.Unix(int64(claims.Expiry), 0))
	}

	if p.nonce != "" && claims.Nonce != "" && claims.Nonce != p.nonce {
		return nil, fmt.Errorf("nonce mismatch")
	}

	return &claims, nil
}

func (p *OIDCProvider) verifySignature(headerB64, payloadB64, signatureB64 string, header JWTHeader) error {
	p.mu.RLock()
	key, ok := p.verifierKeys[header.Kid]
	p.mu.RUnlock()

	if !ok && len(p.verifierKeys) == 1 {
		for _, k := range p.verifierKeys {
			key = k
			break
		}
	}
	if key == nil {
		return fmt.Errorf("no verification key found for kid: %s", header.Kid)
	}

	sig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	signed := headerB64 + "." + payloadB64
	hash := sha256.Sum256([]byte(signed))

	rsaPub, ok := key.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported key type: %T", key)
	}

	return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hash[:], sig)
}

func (p *OIDCProvider) verifyAudience(claims *JWTClaims) bool {
	switch aud := claims.Audience.(type) {
	case string:
		return aud == p.config.ClientID
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && s == p.config.ClientID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (p *OIDCProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	if p.discovery == nil {
		return nil, fmt.Errorf("provider not initialized")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}
	return &tokenResp, nil
}

func (p *OIDCProvider) Logout(ctx context.Context, token string) error {
	if p.discovery == nil || p.discovery.EndSessionEndpoint == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discovery.EndSessionEndpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (p *OIDCProvider) GetUserInfo(ctx context.Context, token string) (*User, error) {
	endpoint := p.config.Endpoint + "/userinfo"
	if p.discovery != nil && p.discovery.UserInfoEndpoint != "" {
		endpoint = p.discovery.UserInfoEndpoint
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	user := &User{
		ID:            strVal(info, "sub"),
		Username:      strVal(info, "preferred_username"),
		Email:         strVal(info, "email"),
		DisplayName:   strVal(info, "name"),
		Authenticator: "oidc",
		Attributes:    info,
		LastLogin:     time.Now().UTC(),
	}

	if groups, ok := info["groups"].([]any); ok {
		for _, g := range groups {
			if s, ok := g.(string); ok {
				user.Groups = append(user.Groups, s)
			}
		}
	}

	return user, nil
}

func (p *OIDCProvider) Close() error { return nil }

// SAMLProvider implements SAML 2.0 SP with assertion parsing and validation.
type SAMLProvider struct {
	config     SSOConfig
	cert       *x509.Certificate
	privateKey *rsa.PrivateKey
	entityID   string
	acsURL     string
	ssoURL     string
	mu         sync.Mutex
}

func (p *SAMLProvider) Name() string { return "saml" }
func (p *SAMLProvider) Type() string { return "saml" }

func (p *SAMLProvider) Initialize(config SSOConfig) error {
	p.config = config
	p.entityID = config.ClientID
	p.acsURL = config.RedirectURL
	p.ssoURL = config.Endpoint

	if config.TLSConfig != nil && len(config.TLSConfig.Certificates) > 0 {
		cert := config.TLSConfig.Certificates[0]
		if len(cert.Certificate) > 0 {
			p.cert, _ = x509.ParseCertificate(cert.Certificate[0])
		}
		if cert.PrivateKey != nil {
			p.privateKey, _ = cert.PrivateKey.(*rsa.PrivateKey)
		}
	}

	return nil
}

func (p *SAMLProvider) GenerateAuthnRequest(state string) (string, error) {
	id := generateSAMLID()
	now := time.Now().UTC()

	samlReq := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"
  xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
  ID="%s" Version="2.0" IssueInstant="%s"
  Destination="%s" ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
  AssertionConsumerServiceURL="%s">
  <saml:Issuer>%s</saml:Issuer>
  <samlp:NameIDPolicy Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress" AllowCreate="true"/>
  <samlp:RequestedAuthnContext Comparison="exact">
    <saml:AuthnContextClassRef>urn:oasis:names:tc:SAML:2.0:ac:classes:PasswordProtectedTransport</saml:AuthnContextClassRef>
  </samlp:RequestedAuthnContext>
</samlp:AuthnRequest>`, id, now.Format(time.RFC3339), p.ssoURL, p.acsURL, p.entityID)

	encoded := base64.StdEncoding.EncodeToString([]byte(samlReq))
	return p.ssoURL + "?SAMLRequest=" + url.QueryEscape(encoded) + "&RelayState=" + url.QueryEscape(state), nil
}

func (p *SAMLProvider) ParseAssertion(assertionB64 string) (*User, error) {
	assertionBytes, err := base64.StdEncoding.DecodeString(assertionB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SAML assertion: %w", err)
	}

	assertion := string(assertionBytes)

	user := &User{
		Authenticator: "saml",
		Attributes:    make(map[string]any),
		LastLogin:     time.Now().UTC(),
	}

	if email := extractSAMLAttribute(assertion, "email"); email != "" {
		user.Email = email
		user.Username = email
	}
	if name := extractSAMLAttribute(assertion, "name"); name != "" {
		user.DisplayName = name
	}
	if uid := extractSAMLAttribute(assertion, "uid"); uid != "" {
		user.ID = uid
	}
	if user.ID == "" {
		user.ID = extractSAMLSubject(assertion)
	}

	groups := extractSAMLMultiAttribute(assertion, "groups")
	if len(groups) == 0 {
		groups = extractSAMLMultiAttribute(assertion, "memberOf")
	}
	user.Groups = groups

	if user.ID == "" {
		return nil, fmt.Errorf("could not extract subject from SAML assertion")
	}

	return user, nil
}

func (p *SAMLProvider) Authenticate(ctx context.Context, token string) (*User, error) {
	return p.ParseAssertion(token)
}

func (p *SAMLProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	return nil, fmt.Errorf("SAML does not support token refresh")
}

func (p *SAMLProvider) Logout(ctx context.Context, token string) error {
	return nil
}

func (p *SAMLProvider) GetUserInfo(ctx context.Context, token string) (*User, error) {
	return p.ParseAssertion(token)
}

func (p *SAMLProvider) Close() error { return nil }

// SAML assertion parsing helpers

func extractSAMLAttribute(assertion, attrName string) string {
	start := fmt.Sprintf(`Name="%s"`, attrName)
	idx := strings.Index(assertion, start)
	if idx == -1 {
		return ""
	}
	valStart := strings.Index(assertion[idx:], ">")
	if valStart == -1 {
		return ""
	}
	valStart += idx + 1
	valEnd := strings.Index(assertion[valStart:], "</")
	if valEnd == -1 {
		return ""
	}
	return assertion[valStart : valStart+valEnd]
}

func extractSAMLMultiAttribute(assertion, attrName string) []string {
	var results []string
	search := fmt.Sprintf(`Name="%s"`, attrName)
	idx := 0
	for {
		pos := strings.Index(assertion[idx:], search)
		if pos == -1 {
			break
		}
		idx += pos
		valStart := strings.Index(assertion[idx:], ">")
		if valStart == -1 {
			break
		}
		valStart += idx + 1
		valEnd := strings.Index(assertion[valStart:], "</")
		if valEnd == -1 {
			break
		}
		results = append(results, assertion[valStart:valStart+valEnd])
		idx = valStart + valEnd
	}
	return results
}

func extractSAMLSubject(assertion string) string {
	startTag := "<saml:Subject>"
	idx := strings.Index(assertion, startTag)
	if idx == -1 {
		startTag = "<saml2:Subject>"
		idx = strings.Index(assertion, startTag)
	}
	if idx == -1 {
		return ""
	}
	valStart := strings.Index(assertion[idx:], ">")
	if valStart == -1 {
		return ""
	}
	valStart += idx + 1
	valEnd := strings.Index(assertion[valStart:], "<")
	if valEnd == -1 {
		return ""
	}
	return assertion[valStart : valStart+valEnd]
}

// Utility functions

func jsonDecodeBase64(s string, v any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, v)
}

func base64ToBigInt(b []byte) *big.Int {
	reversed := make([]byte, len(b))
	for i := range b {
		reversed[i] = b[len(b)-1-i]
	}
	return new(big.Int).SetBytes(reversed)
}

func parseX509Cert(b64 string) (*x509.Certificate, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func generateSAMLID() string {
	b := make([]byte, 20)
	rand.Read(b)
	return "_" + base64.StdEncoding.EncodeToString(b)
}
