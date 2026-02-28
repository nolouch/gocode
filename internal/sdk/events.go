package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nolouch/gcode/internal/bus"
)

type Event = bus.Event

func (c *Client) SubscribeEvents(ctx context.Context, sessionID string) (<-chan Event, <-chan error, error) {
	events := make(chan Event, 64)
	errs := make(chan error, 1)

	endpoint := c.endpoint("/v1/events")
	if sessionID != "" {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, nil, err
		}
		q := u.Query()
		q.Set("session_id", sessionID)
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("subscribe events: HTTP %d", resp.StatusCode)
	}

	go func() {
		defer close(events)
		defer close(errs)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

		var eventName string
		var dataLines []string
		emit := func() {
			if eventName == "" || eventName == "connected" || len(dataLines) == 0 {
				eventName = ""
				dataLines = nil
				return
			}
			payload := strings.Join(dataLines, "\n")
			var e Event
			if err := json.Unmarshal([]byte(payload), &e); err != nil {
				select {
				case errs <- err:
				default:
				}
				eventName = ""
				dataLines = nil
				return
			}
			select {
			case events <- e:
			case <-ctx.Done():
			}
			eventName = ""
			dataLines = nil
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				emit()
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}()

	return events, errs, nil
}
