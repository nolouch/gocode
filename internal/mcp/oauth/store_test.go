package oauth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_GetSet(t *testing.T) {
	// Create temporary store
	tmpDir := t.TempDir()
	store := &Store{
		filePath: filepath.Join(tmpDir, "test-auth.json"),
		data:     make(map[string]*Credentials),
	}

	// Test Set and Get
	creds := &Credentials{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Unix() + 3600,
		ClientID:     "test-client",
		ServerURL:    "https://example.com",
	}

	err := store.Set("test-server", creds)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := store.Get("test-server")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.AccessToken != creds.AccessToken {
		t.Errorf("AccessToken = %q, want %q", retrieved.AccessToken, creds.AccessToken)
	}
	if retrieved.ClientID != creds.ClientID {
		t.Errorf("ClientID = %q, want %q", retrieved.ClientID, creds.ClientID)
	}
}

func TestStore_GetForURL(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{
		filePath: filepath.Join(tmpDir, "test-auth.json"),
		data:     make(map[string]*Credentials),
	}

	creds := &Credentials{
		AccessToken: "test-token",
		ServerURL:   "https://example.com",
	}

	store.Set("test-server", creds)

	// Test matching URL
	retrieved, err := store.GetForURL("test-server", "https://example.com")
	if err != nil {
		t.Fatalf("GetForURL failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected credentials, got nil")
	}

	// Test non-matching URL
	retrieved, err = store.GetForURL("test-server", "https://different.com")
	if err != nil {
		t.Fatalf("GetForURL failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected nil for non-matching URL")
	}
}

func TestStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{
		filePath: filepath.Join(tmpDir, "test-auth.json"),
		data:     make(map[string]*Credentials),
	}

	creds := &Credentials{AccessToken: "test-token"}
	store.Set("test-server", creds)

	err := store.Delete("test-server")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	retrieved, _ := store.Get("test-server")
	if retrieved != nil {
		t.Error("Expected nil after delete")
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-auth.json")

	// Create store and save data
	store1 := &Store{
		filePath: filePath,
		data:     make(map[string]*Credentials),
	}

	creds := &Credentials{
		AccessToken: "test-token",
		ClientID:    "test-client",
	}
	store1.Set("test-server", creds)

	// Create new store and load data
	store2 := &Store{
		filePath: filePath,
		data:     make(map[string]*Credentials),
	}
	err := store2.load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	retrieved, _ := store2.Get("test-server")
	if retrieved == nil {
		t.Fatal("Expected credentials after load")
	}
	if retrieved.AccessToken != creds.AccessToken {
		t.Errorf("AccessToken = %q, want %q", retrieved.AccessToken, creds.AccessToken)
	}
}

func TestCredentials_IsExpired(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name      string
		expiresAt int64
		want      bool
	}{
		{"not expired", now + 3600, false},
		{"expired", now - 3600, true},
		{"no expiration", 0, false},
		{"just expired", now, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &Credentials{ExpiresAt: tt.expiresAt}
			if got := creds.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentials_IsClientSecretExpired(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name      string
		expiresAt int64
		want      bool
	}{
		{"not expired", now + 3600, false},
		{"expired", now - 3600, true},
		{"no expiration", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &Credentials{ClientSecretExpiresAt: tt.expiresAt}
			if got := creds.IsClientSecretExpired(); got != tt.want {
				t.Errorf("IsClientSecretExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	if state1 == state2 {
		t.Error("Expected different state values")
	}

	if len(state1) == 0 {
		t.Error("Expected non-empty state")
	}
}

func TestGenerateCodeVerifier(t *testing.T) {
	verifier1, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier failed: %v", err)
	}

	verifier2, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier failed: %v", err)
	}

	if verifier1 == verifier2 {
		t.Error("Expected different verifier values")
	}

	if len(verifier1) == 0 {
		t.Error("Expected non-empty verifier")
	}
}

func TestStore_OAuthState(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{
		filePath: filepath.Join(tmpDir, "test-auth.json"),
		data:     make(map[string]*Credentials),
	}

	// Update state
	err := store.UpdateOAuthState("test-server", "test-state-123")
	if err != nil {
		t.Fatalf("UpdateOAuthState failed: %v", err)
	}

	// Get state
	state, err := store.GetOAuthState("test-server")
	if err != nil {
		t.Fatalf("GetOAuthState failed: %v", err)
	}

	if state != "test-state-123" {
		t.Errorf("State = %q, want %q", state, "test-state-123")
	}
}

func TestNewStore(t *testing.T) {
	// This test requires a real home directory
	// Skip if HOME is not set
	if os.Getenv("HOME") == "" {
		t.Skip("HOME not set")
	}

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if store.filePath == "" {
		t.Error("Expected non-empty file path")
	}

	if store.data == nil {
		t.Error("Expected initialized data map")
	}
}
