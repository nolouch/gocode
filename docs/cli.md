# CLI reference

This document describes the currently implemented CLI surface in `gocode`.

## Top-level commands

```bash
gocode tui
gocode run
gocode serve
gocode mcp
gocode config
```

When no subcommand is provided, `gocode` defaults to `tui` mode.

## `gocode tui`

Start interactive TUI mode.

```bash
gocode tui [--dir <path>] [--agent <name>] [--addr :4096] [--socket <path>] [--attach]
```

Flags:

- `--dir, -d` working directory for the session (defaults to current directory)
- `--agent, -a` agent name (defaults to `build`)
- `--addr` also expose server on TCP while running TUI
- `--socket` unix socket path for local API access
- `--attach` connect to an existing server instead of starting an embedded runtime

Examples:

```bash
gocode tui
gocode tui -d /path/to/repo
gocode tui --attach --addr :4096
```

## `gocode run`

Run a single prompt in non-interactive mode.

```bash
gocode run -p "<prompt>" [--dir <path>] [--agent <name>]
```

Flags:

- `--prompt, -p` required prompt text
- `--dir, -d` working directory
- `--agent, -a` agent name (defaults to `build`)

Example:

```bash
gocode run -p "Summarize this repository architecture"
```

## `gocode serve`

Run headless API server only.

```bash
gocode serve [--dir <path>] [--addr :4096] [--socket <path>]
```

Flags:

- `--dir, -d` working directory
- `--addr` TCP address to listen on
- `--socket` unix socket path (default `~/.gocode/run/gcode.sock`)

Examples:

```bash
gocode serve --addr :4096
gocode serve --socket /tmp/gocode.sock
```

## `gocode mcp`

Manage configured MCP servers.

Subcommands:

- `gocode mcp list` list configured MCP servers
- `gocode mcp status` show enabled/disabled status and total discovered tools
- `gocode mcp auth <server-name>` run OAuth flow for a remote MCP server
- `gocode mcp logout <server-name>` remove stored OAuth credentials

Examples:

```bash
gocode mcp list
gocode mcp status
gocode mcp auth my-remote-server
gocode mcp logout my-remote-server
```

## `gocode config`

Print effective configuration as JSON with API key redacted.

```bash
gocode config
```
