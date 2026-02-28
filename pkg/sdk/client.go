package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

type Config struct {
	BaseURL    string
	SocketPath string
}

type Client struct {
	baseURL string
	http    *http.Client
}

type Run struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Prompt    string `json:"prompt"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

func New(cfg Config) *Client {
	baseURL := cfg.BaseURL
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if cfg.SocketPath != "" {
		baseURL = "http://unix"
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", cfg.SocketPath)
		}
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:4096"
	}

	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Transport: transport},
	}
}

func (c *Client) endpoint(path string) string {
	u, _ := url.JoinPath(c.baseURL, path)
	return u
}

func (c *Client) CreateSession(ctx context.Context, workDir string) (*Session, error) {
	reqBody, _ := json.Marshal(map[string]string{"work_dir": workDir})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/sessions"), bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create session: %s", string(body))
	}
	var out Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListSessions(ctx context.Context) ([]*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/sessions"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list sessions: %s", string(body))
	}
	var out []*Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/sessions/"+sessionID+"/messages"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get messages: %s", string(body))
	}
	var out []*Message
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateRun(ctx context.Context, sessionID string, text string, agent string) (*Run, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"text":  text,
		"agent": agent,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/sessions/"+sessionID+"/messages"), bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create run: %s", string(body))
	}
	var out Run
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (*Run, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/runs/"+runID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get run: %s", string(body))
	}
	var out Run
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AbortRun(ctx context.Context, runID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/runs/"+runID+"/abort"), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("abort run: %s", string(body))
	}
	return nil
}

func (c *Client) SendMessage(ctx context.Context, sessionID string, text string, agent string) error {
	_, err := c.CreateRun(ctx, sessionID, text, agent)
	return err
}
