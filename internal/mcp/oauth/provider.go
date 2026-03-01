package oauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider implements OAuth 2.0 client functionality for MCP servers.
type Provider struct {
	mcpName      string
	serverURL    string
	clientID     string
	clientSecret string
	scope        string
	store        *Store
}

// NewProvider creates a new OAuth provider for an MCP server.
func NewProvider(mcpName, serverURL, clientID, clientSecret, scope string, store *Store) *Provider {
	return &Provider{
		mcpName:      mcpName,
		serverURL:    serverURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        scope,
		store:        store,
	}
}

// GetAuthorizationURL generates the OAuth authorization URL.
func (p *Provider) GetAuthorizationURL(state, codeVerifier string) (string, error) {
	// Get OAuth endpoints from server
	endpoints, err := p.discoverEndpoints()
	if err != nil {
		return "", fmt.Errorf("discover endpoints: %w", err)
	}

	// Generate PKCE code challenge
	codeChallenge := generateCodeChallenge(codeVerifier)

	// Build authorization URL
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.clientID)
	params.Set("redirect_uri", GetCallbackURL())
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	if p.scope != "" {
		params.Set("scope", p.scope)
	}

	authURL := endpoints.AuthorizationEndpoint + "?" + params.Encode()
	return authURL, nil
}

// ExchangeCode exchanges an authorization code for access tokens.
func (p *Provider) ExchangeCode(ctx context.Context, code, codeVerifier string) (*Credentials, error) {
	endpoints, err := p.discoverEndpoints()
	if err != nil {
		return nil, fmt.Errorf("discover endpoints: %w", err)
	}

	// Prepare token request
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", GetCallbackURL())
	data.Set("client_id", p.clientID)
	data.Set("code_verifier", codeVerifier)

	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	// Make token request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoints.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: %s - %s", resp.Status, string(body))
	}

	// Parse token response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	// Calculate expiration time
	var expiresAt int64
	if tokenResp.ExpiresIn > 0 {
		expiresAt = currentTimestamp() + tokenResp.ExpiresIn
	}

	creds := &Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		ServerURL:    p.serverURL,
	}

	return creds, nil
}

// RefreshToken refreshes an expired access token.
func (p *Provider) RefreshToken(ctx context.Context, refreshToken string) (*Credentials, error) {
	endpoints, err := p.discoverEndpoints()
	if err != nil {
		return nil, fmt.Errorf("discover endpoints: %w", err)
	}

	// Prepare refresh request
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", p.clientID)

	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	// Make refresh request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoints.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh request failed: %s - %s", resp.Status, string(body))
	}

	// Parse token response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	// Calculate expiration time
	var expiresAt int64
	if tokenResp.ExpiresIn > 0 {
		expiresAt = currentTimestamp() + tokenResp.ExpiresIn
	}

	creds := &Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Scope:        tokenResp.Scope,
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		ServerURL:    p.serverURL,
	}

	return creds, nil
}

// RegisterClient performs dynamic client registration (RFC 7591).
func (p *Provider) RegisterClient(ctx context.Context) (*Credentials, error) {
	endpoints, err := p.discoverEndpoints()
	if err != nil {
		return nil, fmt.Errorf("discover endpoints: %w", err)
	}

	if endpoints.RegistrationEndpoint == "" {
		return nil, fmt.Errorf("server does not support dynamic client registration")
	}

	// Prepare registration request
	regReq := map[string]any{
		"client_name":   "gcode",
		"redirect_uris": []string{GetCallbackURL()},
		"grant_types":   []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_method": "client_secret_post",
	}

	reqBody, err := json.Marshal(regReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoints.RegistrationEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registration failed: %s - %s", resp.Status, string(body))
	}

	// Parse registration response
	var regResp struct {
		ClientID              string `json:"client_id"`
		ClientSecret          string `json:"client_secret"`
		ClientIDIssuedAt      int64  `json:"client_id_issued_at"`
		ClientSecretExpiresAt int64  `json:"client_secret_expires_at"`
	}

	if err := json.Unmarshal(body, &regResp); err != nil {
		return nil, fmt.Errorf("parse registration response: %w", err)
	}

	creds := &Credentials{
		ClientID:              regResp.ClientID,
		ClientSecret:          regResp.ClientSecret,
		ClientIDIssuedAt:      regResp.ClientIDIssuedAt,
		ClientSecretExpiresAt: regResp.ClientSecretExpiresAt,
		ServerURL:             p.serverURL,
	}

	return creds, nil
}

// OAuthEndpoints contains OAuth 2.0 server endpoints.
type OAuthEndpoints struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

// discoverEndpoints discovers OAuth endpoints from the server.
func (p *Provider) discoverEndpoints() (*OAuthEndpoints, error) {
	// Try .well-known/oauth-authorization-server first
	wellKnownURL := strings.TrimSuffix(p.serverURL, "/") + "/.well-known/oauth-authorization-server"

	resp, err := http.Get(wellKnownURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var endpoints OAuthEndpoints
		if err := json.NewDecoder(resp.Body).Decode(&endpoints); err == nil {
			return &endpoints, nil
		}
	}

	// Fallback to standard endpoints
	baseURL := strings.TrimSuffix(p.serverURL, "/")
	return &OAuthEndpoints{
		AuthorizationEndpoint: baseURL + "/oauth/authorize",
		TokenEndpoint:         baseURL + "/oauth/token",
		RegistrationEndpoint:  baseURL + "/oauth/register",
	}, nil
}

// generateCodeChallenge generates a PKCE code challenge from a verifier.
func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
}

// currentTimestamp returns the current Unix timestamp.
func currentTimestamp() int64 {
	return time.Now().Unix()
}
