# Configuration

This document covers the config keys currently read by `gocode`.

## Config file load order

At startup, `gocode` reads the first existing file from this list:

1. `<workdir>/.gocode/config.yaml`
2. `<workdir>/.opencode/config.yaml`
3. `<workdir>/config.yaml`
4. `$HOME/.config/gcode/config.yaml`

`<workdir>` is the directory selected by `--dir` (or the current directory).

## Top-level schema

```yaml
provider:
  name: openai
  base_url: https://api.openai.com/v1
  api_key: ""
  model: gpt-4o

default_agent: build

agent: {}

mcp: {}

skills:
  paths: []
```

## `provider`

Fields:

- `name`: `openai`, `anthropic`, `google`, `openrouter`, or `openai-compat`
- `base_url`: provider base URL (trimmed of trailing slash)
- `api_key`: API key (if not provided via env)
- `model`: model ID

Notes:

- `provider.name` defaults to `openai`
- `provider.model` defaults to `gpt-4o`
- Runtime exits if no provider API key can be resolved

## `default_agent`

Default primary agent used by `tui` and `run` when `--agent build` is passed.

Built-in agents:

- `build` (primary): read/write/run
- `explore` (subagent): read-only exploration profile
- `plan` (primary): read-only planning profile

## `agent` overrides

You can override built-ins or define custom agents under `agent.<name>`.

Supported fields:

- `disable`
- `name`
- `description`
- `mode` (`primary`, `subagent`, `all`)
- `prompt`
- `provider_id`
- `model_id`
- `steps`
- `temperature`
- `denied_tools`
- `permission`

Example:

```yaml
agent:
  reviewer:
    description: Read-only reviewer
    mode: subagent
    prompt: Review code and provide feedback only.
    permission:
      "*": deny
      read: allow
      list: allow
      glob: allow
      grep: allow
```

## `mcp`

Define MCP servers by name.

Supported fields per server:

- `type`: `local` or `remote`
- `command`: array command for local server
- `url`: remote MCP endpoint URL
- `headers`: map of extra HTTP headers
- `env`: map of environment variables for local server process
- `timeout_ms`: request/handshake timeout
- `enabled`: `true` or `false` (defaults to enabled)

Example:

```yaml
mcp:
  my-local-tools:
    type: local
    command: ["npx", "-y", "@my-org/mcp-server"]
    enabled: true

  my-remote-tools:
    type: remote
    url: https://example.com/mcp
    headers:
      Authorization: Bearer ${MY_TOKEN}
    timeout_ms: 30000
```

## `skills`

Additional directories to scan for `SKILL.md` files.

```yaml
skills:
  paths:
    - ~/my-custom-skills
```

Standard locations are still scanned automatically.

## Environment variable overrides

`gocode` applies these environment variables after loading YAML:

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GOOGLE_API_KEY`
- `OPENROUTER_API_KEY`
- `GCODE_MODEL`
- `GCODE_BASE_URL`

API key selection behavior:

- first configured YAML key wins
- if YAML has no key, the first available provider key env var is used

## Effective config output

Use this command to inspect runtime config:

```bash
gocode config
```

The printed API key is masked.
