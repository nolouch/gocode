package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// CallbackPort is the port for the OAuth callback server
	CallbackPort = 19876
	// CallbackPath is the path for OAuth callbacks
	CallbackPath = "/oauth/callback"
)

// CallbackServer handles OAuth callbacks from authorization servers.
type CallbackServer struct {
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener
	running  bool

	// Pending callbacks: state -> channel
	pending map[string]chan string
}

var (
	globalCallbackServer *CallbackServer
	globalCallbackMu     sync.Mutex
)

// GetCallbackServer returns the global callback server instance.
func GetCallbackServer() *CallbackServer {
	globalCallbackMu.Lock()
	defer globalCallbackMu.Unlock()

	if globalCallbackServer == nil {
		globalCallbackServer = &CallbackServer{
			pending: make(map[string]chan string),
		}
	}
	return globalCallbackServer
}

// Start starts the OAuth callback server if not already running.
func (s *CallbackServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil // Already running
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", CallbackPort))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", CallbackPort, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, s.handleCallback)

	s.listener = listener
	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	s.running = true

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[oauth] callback server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the OAuth callback server.
func (s *CallbackServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}

	s.running = false
	return nil
}

// WaitForCallback registers a callback listener for a specific state parameter.
// Returns a channel that will receive the authorization code.
func (s *CallbackServer) WaitForCallback(state string, timeout time.Duration) (string, error) {
	s.mu.Lock()
	ch := make(chan string, 1)
	s.pending[state] = ch
	s.mu.Unlock()

	// Clean up after timeout
	defer func() {
		s.mu.Lock()
		delete(s.pending, state)
		close(ch)
		s.mu.Unlock()
	}()

	select {
	case code := <-ch:
		return code, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout waiting for OAuth callback")
	}
}

// handleCallback handles incoming OAuth callbacks.
func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	state := query.Get("state")
	code := query.Get("code")
	errorParam := query.Get("error")
	errorDesc := query.Get("error_description")

	// Handle error responses
	if errorParam != "" {
		msg := fmt.Sprintf("OAuth error: %s", errorParam)
		if errorDesc != "" {
			msg += fmt.Sprintf(" - %s", errorDesc)
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Validate required parameters
	if state == "" || code == "" {
		http.Error(w, "Missing state or code parameter", http.StatusBadRequest)
		return
	}

	// Find pending callback
	s.mu.Lock()
	ch, ok := s.pending[state]
	s.mu.Unlock()

	if !ok {
		http.Error(w, "Invalid or expired state parameter", http.StatusBadRequest)
		return
	}

	// Send code to waiting goroutine
	select {
	case ch <- code:
		// Success response
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Authentication Successful</title>
    <style>
        body { font-family: sans-serif; text-align: center; padding: 50px; }
        .success { color: #28a745; font-size: 24px; margin-bottom: 20px; }
        .message { color: #666; }
    </style>
</head>
<body>
    <div class="success">✓ Authentication Successful</div>
    <div class="message">You can close this window and return to the terminal.</div>
</body>
</html>
`)
	default:
		http.Error(w, "Callback already processed", http.StatusConflict)
	}
}

// GetCallbackURL returns the full callback URL for OAuth redirects.
func GetCallbackURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", CallbackPort, CallbackPath)
}
