package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/nolouch/opengocode/internal/mcp/oauth"
)

// Authenticate performs OAuth authentication for an MCP server.
// Returns the access token on success.
func Authenticate(mcpName string, cfg ServerConfig) (string, error) {
	if cfg.OAuth == nil || !cfg.OAuth.Enabled {
		return "", fmt.Errorf("OAuth not configured for %s", mcpName)
	}

	// Initialize OAuth store
	store, err := oauth.NewStore()
	if err != nil {
		return "", fmt.Errorf("init OAuth store: %w", err)
	}

	// Check for existing valid credentials
	creds, err := store.GetForURL(mcpName, cfg.URL)
	if err != nil {
		return "", fmt.Errorf("get credentials: %w", err)
	}

	// If we have valid credentials, return them
	if creds != nil && !creds.IsExpired() {
		return creds.AccessToken, nil
	}

	// If we have a refresh token, try to refresh
	if creds != nil && creds.RefreshToken != "" {
		provider := oauth.NewProvider(
			mcpName,
			cfg.URL,
			creds.ClientID,
			creds.ClientSecret,
			cfg.OAuth.Scope,
			store,
		)

		newCreds, err := provider.RefreshToken(context.Background(), creds.RefreshToken)
		if err == nil {
			// Save refreshed credentials
			if err := store.Set(mcpName, newCreds); err != nil {
				return "", fmt.Errorf("save refreshed credentials: %w", err)
			}
			return newCreds.AccessToken, nil
		}
		// If refresh fails, fall through to full authentication
	}

	// Perform full OAuth flow
	return performOAuthFlow(mcpName, cfg, store)
}

// performOAuthFlow executes the complete OAuth authorization flow.
func performOAuthFlow(mcpName string, cfg ServerConfig, store *oauth.Store) (string, error) {
	// Determine client credentials
	clientID := cfg.OAuth.ClientID
	clientSecret := cfg.OAuth.ClientSecret

	// If no client ID, try dynamic registration
	if clientID == "" {
		provider := oauth.NewProvider(mcpName, cfg.URL, "", "", cfg.OAuth.Scope, store)
		regCreds, err := provider.RegisterClient(context.Background())
		if err != nil {
			return "", fmt.Errorf("dynamic client registration: %w", err)
		}

		clientID = regCreds.ClientID
		clientSecret = regCreds.ClientSecret

		// Save registered client info
		if err := store.Set(mcpName, regCreds); err != nil {
			return "", fmt.Errorf("save registered client: %w", err)
		}
	}

	// Generate state and code verifier for PKCE
	state, err := oauth.GenerateState()
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	codeVerifier, err := oauth.GenerateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("generate code verifier: %w", err)
	}

	// Save state for CSRF validation
	if err := store.UpdateOAuthState(mcpName, state); err != nil {
		return "", fmt.Errorf("save OAuth state: %w", err)
	}

	// Start callback server
	callbackServer := oauth.GetCallbackServer()
	if err := callbackServer.Start(); err != nil {
		return "", fmt.Errorf("start callback server: %w", err)
	}

	// Create provider
	provider := oauth.NewProvider(mcpName, cfg.URL, clientID, clientSecret, cfg.OAuth.Scope, store)

	// Get authorization URL
	authURL, err := provider.GetAuthorizationURL(state, codeVerifier)
	if err != nil {
		return "", fmt.Errorf("get authorization URL: %w", err)
	}

	// Open browser
	fmt.Printf("Opening browser for authentication...\n")
	fmt.Printf("If the browser doesn't open, visit: %s\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	// Wait for callback (5 minute timeout)
	code, err := callbackServer.WaitForCallback(state, 5*time.Minute)
	if err != nil {
		return "", fmt.Errorf("wait for callback: %w", err)
	}

	// Validate state
	storedState, err := store.GetOAuthState(mcpName)
	if err != nil {
		return "", fmt.Errorf("get stored state: %w", err)
	}
	if storedState != state {
		return "", fmt.Errorf("OAuth state mismatch - potential CSRF attack")
	}

	// Exchange code for tokens
	creds, err := provider.ExchangeCode(context.Background(), code, codeVerifier)
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}

	// Save credentials
	if err := store.Set(mcpName, creds); err != nil {
		return "", fmt.Errorf("save credentials: %w", err)
	}

	fmt.Printf("Authentication successful!\n")
	return creds.AccessToken, nil
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}

// RemoveAuth removes stored OAuth credentials for an MCP server.
func RemoveAuth(mcpName string) error {
	store, err := oauth.NewStore()
	if err != nil {
		return fmt.Errorf("init OAuth store: %w", err)
	}

	return store.Delete(mcpName)
}
