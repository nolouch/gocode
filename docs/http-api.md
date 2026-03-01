# HTTP API

`gocode serve` and `gocode tui` expose the same HTTP API.

Transport options:

- TCP: set `--addr` (for example `:4096`)
- Unix socket: default `~/.gocode/run/gcode.sock` or custom `--socket`

## Endpoints

### Health and runtime info

- `GET /v1/health` -> `{ "status": "ok" }`
- `GET /v1/config` -> runtime metadata (`work_dir`, `version`)

### Agent and tool metadata

- `GET /v1/agents` -> registered agents
- `GET /v1/tools` -> effective tool list with schemas

### Sessions

- `POST /v1/sessions`
  - body: `{ "work_dir": "/path", "parent_id": "optional" }`
  - creates a new session
- `GET /v1/sessions`
  - list sessions
- `GET /v1/sessions/{id}`
  - get one session
- `GET /v1/sessions/{id}/messages`
  - list message history
- `GET /v1/sessions/{id}/children`
  - list child sessions

### Runs

- `POST /v1/sessions/{id}/messages`
  - body: `{ "text": "your prompt", "agent": "build" }`
  - starts an async run and returns run metadata
- `GET /v1/runs/{id}`
  - get run status (`running`, `completed`, `failed`, `aborted`)
- `POST /v1/runs/{id}/abort`
  - abort a running task

### Events (SSE)

- `GET /v1/events`
- optional query: `?session_id=<session-id>`

SSE emits:

- `connected` (initial)
- `turn.done`, `turn.error`
- `message.part.upsert`, `message.part.delta`, `message.part.done`
- heartbeat comments every 10 seconds

## Curl examples (TCP)

Assume server is started with `gocode serve --addr :4096`.

Create session:

```bash
curl -sS http://127.0.0.1:4096/v1/sessions \
  -H 'content-type: application/json' \
  -d '{"work_dir":"."}'
```

Start run:

```bash
curl -sS http://127.0.0.1:4096/v1/sessions/<session-id>/messages \
  -H 'content-type: application/json' \
  -d '{"text":"Summarize this project","agent":"build"}'
```

Check run:

```bash
curl -sS http://127.0.0.1:4096/v1/runs/<run-id>
```

Stream events:

```bash
curl -N http://127.0.0.1:4096/v1/events?session_id=<session-id>
```

## Curl examples (unix socket)

```bash
curl --unix-socket ~/.gocode/run/gcode.sock \
  http://localhost/v1/health
```

```bash
curl --unix-socket ~/.gocode/run/gcode.sock \
  -H 'content-type: application/json' \
  -d '{"work_dir":"."}' \
  http://localhost/v1/sessions
```

## Go SDK

`pkg/sdk` provides a typed client for sessions, runs, and SSE subscriptions.

Example:

```go
client := sdk.New(sdk.Config{BaseURL: "http://127.0.0.1:4096"})
sess, _ := client.CreateSession(ctx, ".")
run, _ := client.CreateRun(ctx, sess.ID, "Analyze this repo", "build")
_ = run
```
