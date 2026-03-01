// Package oauth provides OAuth 2.0 authentication for MCP servers.
package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Credentials stores OAuth tokens and client information for an MCP server.
type Credentials struct {
	// OAuth tokens
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"` // Unix timestamp
	Scope        string `json:"scope,omitempty"`

	// Dynamic client registration (RFC 7591)
	ClientID                string `json:"client_id,omitempty"`
	ClientSecret            string `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64  `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt   int64  `json:"client_secret_expires_at,omitempty"`

	// PKCE and CSRF protection
	CodeVerifier string `json:"code_verifier,omitempty"`
	OAuthState   string `json:"oauth_state,omitempty"`

	// Server URL for validation
	ServerURL string `json:"server_url,omitempty"`
}

// Store manages OAuth credentials persistence.
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     map[string]*Credentials // mcpName -> credentials
}

// NewStore creates a new OAuth credentials store.
// Credentials are stored in ~/.gcode/mcp-auth.json
func NewStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	gcodeDir := filepath.Join(homeDir, ".gcode")
	if err := os.MkdirAll(gcodeDir, 0700); err != nil {
		return nil, fmt.Errorf("create .gcode dir: %w", err)
	}

	filePath := filepath.Join(gcodeDir, "mcp-auth.json")
	store := &Store{
		filePath: filePath,
		data:     make(map[string]*Credentials),
	}

	// Load existing credentials
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	return store, nil
}

// Get retrieves credentials for an MCP server.
func (s *Store) Get(mcpName string) (*Credentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	creds, ok := s.data[mcpName]
	if !ok {
		return nil, nil
	}

	// Return a copy to prevent external modification
	credsCopy := *creds
	return &credsCopy, nil
}

// GetForURL retrieves credentials for an MCP server, validating the server URL.
// Returns nil if the stored URL doesn't match (prevents credential reuse for different servers).
func (s *Store) GetForURL(mcpName, serverURL string) (*Credentials, error) {
	creds, err := s.Get(mcpName)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, nil
	}

	// Validate server URL matches
	if creds.ServerURL != "" && creds.ServerURL != serverURL {
		return nil, nil // URL changed, credentials invalid
	}

	return creds, nil
}

// Set stores credentials for an MCP server.
func (s *Store) Set(mcpName string, creds *Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[mcpName] = creds
	return s.save()
}

// Delete removes credentials for an MCP server.
func (s *Store) Delete(mcpName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, mcpName)
	return s.save()
}

// UpdateOAuthState updates the OAuth state parameter for CSRF protection.
func (s *Store) UpdateOAuthState(mcpName, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds := s.data[mcpName]
	if creds == nil {
		creds = &Credentials{}
		s.data[mcpName] = creds
	}

	creds.OAuthState = state
	return s.save()
}

// GetOAuthState retrieves the OAuth state parameter.
func (s *Store) GetOAuthState(mcpName string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	creds := s.data[mcpName]
	if creds == nil {
		return "", nil
	}

	return creds.OAuthState, nil
}

// IsExpired checks if the access token is expired.
func (c *Credentials) IsExpired() bool {
	if c.ExpiresAt == 0 {
		return false // No expiration set
	}
	return time.Now().Unix() >= c.ExpiresAt
}

// IsClientSecretExpired checks if the client secret is expired.
func (c *Credentials) IsClientSecretExpired() bool {
	if c.ClientSecretExpiresAt == 0 {
		return false // No expiration set
	}
	return time.Now().Unix() >= c.ClientSecretExpiresAt
}

// load reads credentials from disk.
func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.data)
}

// save writes credentials to disk.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0600)
}

// GenerateState generates a random state parameter for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateCodeVerifier generates a random code verifier for PKCE.
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
