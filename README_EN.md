# Tavily Proxy Pool & Management Dashboard

[简体中文](./README.md) | English

A reverse proxy that pools multiple Tavily API keys behind a single entry point. Clients hold a Master Key or an access key and call the REST API or MCP exactly as they would call Tavily directly; the proxy picks a key, handles invalid and exhausted keys, tracks usage, and ships with a built-in web dashboard.

Deployed as a single Go binary with the frontend (Vue 3 + Naive UI) embedded; all data lives in one SQLite file.

---

## Features

**Proxy and key pool**

- Transparently forwards every Tavily path and method (`/search`, `/extract`, `/crawl`, `/map`, `/research`, `/usage`, ...).
- Three key selection strategies, switchable in the dashboard:
  - `best_remaining` (default): highest remaining quota first, ties shuffled.
  - `round_robin`: rotate through keys.
  - `sequential_fill`: use keys up in the order they were added.
- Automatic failover:
  - `401`: mark the key invalid and try the next one.
  - `432` / `433`: mark the key exhausted and try the next one.
  - `429` / `403`: try the next key without marking this one.
  - Non-GET requests that were sent but whose outcome is unknown are not retried, so you are never billed twice.
- Credits are accounted per Tavily's official pricing; optional automatic quota sync uses the official `/usage` as the source of truth.
- Optional result cache for `POST /search` with configurable TTL; cache hits cost no credits.

**Access control**

- **Master Key**: the admin credential with access to every API and the dashboard; generated automatically on first start.
- **Access keys**: hand them out so others can use the proxy and MCP, with usage attributed per key. Admins can disable or delete a key, or only block its portal login; holders can reset their own key in the portal.
- **Key donation**: the public homepage accepts Tavily keys. Each key is validated upstream before joining the pool, and the donor receives an access key.
- **Donor portal**: sign in with an access key to view usage of your own keys, add or remove keys, and reset your access key.

**MCP**

- `/mcp`: Streamable HTTP endpoint. `/sse`: legacy SSE endpoint, kept for compatibility only.
- Seven tools: `tavily-search`, `tavily-extract`, `tavily-crawl`, `tavily-map`, `tavily-research`, `tavily-research-get`, `tavily-usage`.
- Stateless by default, so no sticky sessions are needed behind a reverse proxy.

**Dashboard and automation**

- Dashboard: overview and usage charts, key management (batch add, quota sync, export, purge invalid keys), access key management, request logs (filter by status code), settings.
- Public homepage: total pool quota, connection templates for common clients, and the donation form.
- Scheduled jobs: reset used quota on the 1st of each month, sync quota on an interval, prune logs past the retention period, and purge expired cache entries.

---

## Quick start

### Docker Compose (recommended)

```yaml
services:
  tavily-proxy:
    image: ghcr.io/piexian/tavilyproxymanager:latest
    container_name: tavily-proxy
    ports:
      - "8080:8080"
    environment:
      - DATABASE_PATH=/app/data/proxy.db
      - UPSTREAM_TIMEOUT=30s
    volumes:
      - ./data:/app/data
      - /etc/localtime:/etc/localtime:ro
    restart: unless-stopped
```

```bash
docker compose up -d
```

Images are published for `linux/amd64` and `linux/arm64`. Besides `latest`, you can pin a version (e.g. `1.2.1`) or a minor line (e.g. `1.2`).

### Docker

```bash
docker run -d --name tavily-proxy \
  -p 8080:8080 \
  -v "$(pwd)/data:/app/data" \
  -e DATABASE_PATH=/app/data/proxy.db \
  ghcr.io/piexian/tavilyproxymanager:latest
```

### Binary

Download the archive for your platform from [Releases](https://github.com/piexian/TavilyProxyManager/releases), extract it, and run it; the frontend is embedded:

```bash
DATABASE_PATH=./data/proxy.db ./tavily-proxy
```

### Getting the Master Key

On first start a Master Key is generated and written to the log:

```bash
docker logs tavily-proxy 2>&1 | grep "master key"
# level=INFO msg="generated master key" master_key=xxxxxxxx
```

Open `http://localhost:8080`, click sign in, and enter the Master Key to reach the dashboard. You can view or reset the Master Key later under Settings.

### Upgrading

```bash
docker compose pull && docker compose up -d
```

All data lives in the mounted `data/` directory; the database schema is migrated automatically on startup.

---

## Usage

### Credentials

Every entry point accepts the same credentials:

| Credential | Grants access to |
| :--- | :--- |
| Master Key | Proxy, MCP, admin API (`/api/*`), dashboard |
| Access key | Proxy, MCP, donor portal (`/api/portal/*`) |

Pass the credential as `Authorization: Bearer <KEY>`. For older clients, the proxy endpoints also accept `api_key` / `apiKey` in the JSON body and `?api_key=` in the query string; these fields are stripped before forwarding.

### REST API

Call it exactly like the [official Tavily API](https://docs.tavily.com), just with the proxy's address:

```bash
curl -X POST "http://localhost:8080/search" \
  -H "Authorization: Bearer <KEY>" \
  -H "Content-Type: application/json" \
  -d '{"query": "latest AI trends", "search_depth": "basic"}'
```

### MCP

Endpoint: `http://<host>:8080/mcp`, authenticated the same way (Bearer Master Key or access key).

**Claude Code**

```bash
claude mcp add tavily-proxy --transport http http://<host>:8080/mcp \
  --header "Authorization: Bearer <KEY>"
```

**claude.ai / Claude Desktop (custom connector)**

Open Customize > Connectors > Add custom connector and fill it in as follows:

- URL: `https://<your-domain>/mcp`
- Authentication: **No sign-in**
- Request headers: name `authorization`, value `Bearer <KEY>` (type `Bearer` and the space yourself)

Request headers are a Claude beta feature available only to some accounts. This project does not support OAuth sign-in yet, so accounts without that option cannot connect for now. Connectors are called from Claude's servers, so the service must be reachable over public HTTPS.

**VS Code** (`.vscode/mcp.json`)

```json
{
  "servers": {
    "tavily-proxy": {
      "type": "http",
      "url": "http://<host>:8080/mcp",
      "headers": { "Authorization": "Bearer <KEY>" }
    }
  }
}
```

**Other clients that read `mcpServers`** (Cursor, etc.)

```json
{
  "mcpServers": {
    "tavily-proxy": {
      "url": "http://<host>:8080/mcp",
      "headers": { "Authorization": "Bearer <KEY>" }
    }
  }
}
```

The homepage (`http://<host>:8080`) provides ready-to-copy templates for MCP, Claude Code, SSE, REST API, and AstrBot with the address already filled in.

> ChatGPT custom MCP connectors only support OAuth and cannot connect yet.

---

## Configuration

### Environment variables

| Variable | Description | Default |
| :--- | :--- | :--- |
| `LISTEN_ADDR` | Listen address | `:8080` |
| `PORT` | Listen port, used only when `LISTEN_ADDR` is unset | `8080` |
| `DATABASE_PATH` / `DB_PATH` | SQLite database path | `./server/data/app.db` |
| `TAVILY_BASE_URL` | Upstream Tavily API base URL | `https://api.tavily.com` |
| `UPSTREAM_TIMEOUT` | Upstream request timeout (e.g. `30s`, or plain seconds) | `150s` |
| `MCP_STATELESS` | Run MCP in stateless mode | `true` |
| `MCP_SESSION_TTL` | Session idle timeout in stateful mode | `10m` |
| `LOG_DIR` | Directory for daily-rotated log files; empty means stdout only | empty |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` | `info` |

### Dashboard settings

These are stored in the database, edited on the dashboard's Settings page, and take effect immediately:

| Setting | Description | Default |
| :--- | :--- | :--- |
| Key pool strategy | `best_remaining` / `round_robin` / `sequential_fill` | `best_remaining` |
| Search cache | Cache `POST /search` results; TTL from 60 seconds to 7 days | Off, TTL 12 hours |
| Auto quota sync | Periodically calibrate quota via the official `/usage`; interval, concurrency, and request spacing are configurable | Off |
| Request logging | Whether to record request logs | On |
| Log retention | Prune logs older than the retention period; 0 disables pruning | 30 days |

### Behind a reverse proxy

- The client IP is taken from `X-Forwarded-For`, trusted only from localhost and private ranges (`10/8`, `172.16/12`, `192.168/16`).
- Keep `UPSTREAM_TIMEOUT` below the front proxy's timeout. For example, Cloudflare's default is about 100 seconds; otherwise long requests get cut off by the proxy first.
- Only when `MCP_STATELESS=false` do you need to forward `Mcp-Session-Id` and enable sticky sessions.

Health check: `GET /healthz`.

---

## Development

Requirements: Go `1.27.3+`, Node.js `20+`.

```bash
# Backend
go run ./server

# Frontend (in another terminal; the dev server proxies API calls to the backend)
cd web && npm install && npm run dev

# Tests
go test ./...
```

Build:

```bash
# Build and embed the frontend, output Windows and Linux binaries to build/
./scripts/build_all.sh        # on Windows: .\scripts\build_all.ps1

# Docker image (current architecture)
docker build -t tavily-proxy .
```

Release: pushing a `v*` tag makes GitHub Actions build multi-arch images to GHCR and publish per-platform binaries to Releases.

---

## Acknowledgements

This project is a derivative of [xuncv/TavilyProxyManager](https://github.com/xuncv/TavilyProxyManager).

## License

MIT
