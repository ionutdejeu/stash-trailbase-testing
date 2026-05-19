# Stash

**Your AI has amnesia. We fixed it.**

Every LLM starts every conversation from zero. Stash gives your agent persistent memory — it remembers, recalls, consolidates, and learns across sessions. No more explaining yourself from scratch.

Open source. Self-hosted. Works with any MCP-compatible agent.

## Quick Start

```bash
git clone https://github.com/alash3al/stash.git
cd stash
cp .env.example .env   # edit with your API key + model
docker compose up
```

That's it. TrailBase, SQLite-compatible migrations, MCP server with background consolidation — all in one command.

## TrailBase Storage

Stash now targets TrailBase directly.

1. Start TrailBase with the provided `traildepot/migrations`.
2. Set `STASH_STORE_DSN` to the TrailBase SQLite database path, typically `traildepot/data/main.db`.
3. Start the Stash CLI or services as usual.

## MCP Client Setup

After `docker compose up`, Stash exposes an MCP server over SSE at:

```
http://localhost:8080/sse
```

Point any MCP-compatible client at that URL. Example configs:

**Cursor** — `~/.cursor/mcp.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**Claude Desktop** — `claude_desktop_config.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**OpenCode** — `~/.config/opencode/config.json`
```json
{
  "mcp": {
    "stash": {
      "type": "remote",
      "url": "http://localhost:8080/sse",
      "enabled": true
    }
  }
}
```

**Windsurf** — `~/.codeium/windsurf/mcp_config.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

Works with any agent that supports MCP over SSE — Claude Desktop, Cursor, Windsurf, Cline, Continue, OpenAI Agents, Ollama, OpenRouter, and more.

## GitHub Copilot CLI

Stash already exposes MCP over stdio, which is the transport GitHub Copilot CLI expects for local MCP servers.

1. Create your runtime config first:

```bash
cp .env.example .env
# edit .env with your API key, model, and TrailBase path
```

2. Generate a ready-to-paste Copilot CLI config for this repo:

```bash
go run ./cmd/cli mcp copilot-config
```

That command prints JSON for `~/.copilot/mcp-config.json` and points `STASHCONFIG` at this repo's `.env` file, so you do not need to duplicate your `STASH_*` values into Copilot CLI.

Example output:

```json
{
  "mcpServers": {
    "stash": {
      "type": "local",
      "command": "powershell",
      "args": [
        "-NoLogo",
        "-NoProfile",
        "-Command",
        "Set-Location -LiteralPath 'C:\\path\\to\\stash'; go run ./cmd/cli mcp execute"
      ],
      "env": {
        "STASHCONFIG": "C:\\path\\to\\stash\\.env"
      },
      "tools": [
        "*"
      ]
    }
  }
}
```

If you prefer the interactive flow inside Copilot CLI, run `/mcp add` and use the generated values for the local server command, args, and environment.

## What It Does

Stash is a cognitive layer between your AI agent and the world. Episodes become facts. Facts become relationships. Relationships become patterns. Patterns become wisdom.

An 8-stage consolidation pipeline turns raw observations into structured knowledge — facts, relationships, causal links, goal tracking, failure patterns, hypothesis verification, and confidence decay. Each stage only processes new data since the last run.

## Learn More

**[alash3al.github.io/stash →](https://alash3al.github.io/stash/)**

## License

Apache 2.0
