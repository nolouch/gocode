package tool

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	webFetchMaxBytes    = 50000
	webFetchDefaultTimeout = 15 * time.Second
)

// WebFetchTool fetches a URL and returns its content as plain text.
type WebFetchTool struct{}

func (t *WebFetchTool) ID() string { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "Fetch a URL and return its content as plain text. HTML tags are stripped."
}
func (t *WebFetchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":        map[string]any{"type": "string", "description": "URL to fetch"},
			"max_length": map[string]any{"type": "integer", "description": fmt.Sprintf("Max characters to return (default %d)", webFetchMaxBytes)},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx Context, args map[string]any) (Result, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return Result{IsError: true, Output: "web_fetch requires a 'url'"}, nil
	}

	maxLen := webFetchMaxBytes
	if v, ok := args["max_length"].(float64); ok && v > 0 {
		maxLen = int(v)
	}

	client := &http.Client{Timeout: webFetchDefaultTimeout}
	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{IsError: true, Output: fmt.Sprintf("invalid URL: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "gcode/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return Result{IsError: true, Output: fmt.Sprintf("fetch error: %v", err)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return Result{IsError: true, Output: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxLen*3)))
	if err != nil {
		return Result{IsError: true, Output: fmt.Sprintf("read error: %v", err)}, nil
	}

	content := string(body)
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		content = htmlToText(content)
	}

	return Result{Output: truncate(content, maxLen), Title: url}, nil
}

var (
	reScript  = regexp.MustCompile(`(?is)<(script|style|nav|header|footer)[^>]*>.*?</(script|style|nav|header|footer)>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reSpaces  = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
)

func htmlToText(html string) string {
	s := reScript.ReplaceAllString(html, "")
	s = reTag.ReplaceAllString(s, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	// Normalize line breaks
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = reNewlines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
