# Tavily 代理池 & 管理面板

简体中文 | [English](./README_EN.md)

把多个 Tavily API Key 汇聚成一个统一入口的反向代理。客户端只需持有 Master Key 或访问密钥，按与 Tavily 官方完全相同的方式调用 REST API 或 MCP；代理负责挑选 Key、处理失效与额度耗尽、记录用量，并自带 Web 管理面板。

Go 单二进制部署，前端（Vue 3 + Naive UI）编译后内嵌，数据存放在单个 SQLite 文件中。

---

## 功能

**代理与 Key 池**

- 透明转发 Tavily 全部路径与方法（`/search`、`/extract`、`/crawl`、`/map`、`/research`、`/usage` 等）。
- 三种选 Key 策略，可在面板中切换：
  - `best_remaining`（默认）：优先剩余额度最高的 Key，同额度随机打散。
  - `round_robin`：轮询。
  - `sequential_fill`：按添加顺序用满一个再用下一个。
- 自动故障切换：
  - `401`：标记 Key 失效，换下一个。
  - `432` / `433`：标记额度耗尽，换下一个。
  - `429` / `403`：直接换下一个，不标记该 Key。
  - 已发出、但结果未知的非 GET 请求不会重试，避免重复扣费。
- 按 Tavily 官方计费规则记账 credits；可开启自动额度同步，以官方 `/usage` 为准。
- 可选的 `POST /search` 结果缓存，TTL 可配置，命中缓存时不消耗额度。

**访问控制**

- **Master Key**：管理员凭据，可调用全部 API 与管理面板，首次启动时自动生成。
- **访问密钥（Access Key）**：可下发给他人调用代理与 MCP，用量按密钥归因。管理员可以停用、删除密钥，或只禁止其登录门户；持有者可以在门户中自行重置。
- **Key 捐赠**：公开首页可提交 Tavily Key。Key 经上游校验后入池，并为捐赠者签发一把访问密钥。
- **捐赠者门户**：用访问密钥登录，查看自己名下 Key 的用量，可追加或删除 Key、重置访问密钥。

**MCP**

- `/mcp`：Streamable HTTP 端点。`/sse`：旧版 SSE 端点，仅为兼容保留。
- 提供 7 个工具：`tavily-search`、`tavily-extract`、`tavily-crawl`、`tavily-map`、`tavily-research`、`tavily-research-get`、`tavily-usage`。
- 默认无状态模式，部署在反向代理后面时无需会话粘性。

**管理面板与自动化**

- 管理面板：仪表盘与用量趋势图、Key 管理（批量添加、额度同步、导出、清理失效 Key）、访问密钥管理、请求日志（按状态码筛选）、系统设置。
- 公开首页：展示号池总额度、各客户端接入模板与捐赠入口。
- 定时任务：每月 1 日重置已用额度；按间隔自动同步额度；按保留天数清理日志；清理过期缓存。

---

## 快速开始

### Docker Compose（推荐）

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

镜像支持 `linux/amd64` 与 `linux/arm64`。除 `latest` 外，也可固定到具体版本（如 `1.2.1`）或次版本（如 `1.2`）。

### Docker

```bash
docker run -d --name tavily-proxy \
  -p 8080:8080 \
  -v "$(pwd)/data:/app/data" \
  -e DATABASE_PATH=/app/data/proxy.db \
  ghcr.io/piexian/tavilyproxymanager:latest
```

### 二进制

从 [Releases](https://github.com/piexian/TavilyProxyManager/releases) 下载对应平台的压缩包，解压后直接运行，前端已内嵌：

```bash
DATABASE_PATH=./data/proxy.db ./tavily-proxy
```

### 获取 Master Key

首次启动时会自动生成 Master Key 并写入日志：

```bash
docker logs tavily-proxy 2>&1 | grep "master key"
# level=INFO msg="generated master key" master_key=xxxxxxxx
```

打开 `http://localhost:8080`，点击登录并输入 Master Key 即可进入管理面板。之后可以在「设置」中查看或重置 Master Key。

### 升级

```bash
docker compose pull && docker compose up -d
```

所有数据都在挂载的 `data/` 目录里，数据库结构在启动时自动迁移。

---

## 使用

### 凭据

所有入口共用同一套凭据，任选其一：

| 凭据 | 可访问 |
| :--- | :--- |
| Master Key | 代理、MCP、管理 API（`/api/*`）、管理面板 |
| 访问密钥 | 代理、MCP、捐赠者门户（`/api/portal/*`） |

推荐用 `Authorization: Bearer <KEY>` 传递凭据。为兼容旧客户端，代理端点也接受请求体里的 `api_key` / `apiKey`，以及查询参数 `?api_key=`；转发前会把这些字段剥离。

### REST API

调用方式与 [Tavily 官方 API](https://docs.tavily.com) 相同，把地址换成代理地址即可：

```bash
curl -X POST "http://localhost:8080/search" \
  -H "Authorization: Bearer <KEY>" \
  -H "Content-Type: application/json" \
  -d '{"query": "最新 AI 技术趋势", "search_depth": "basic"}'
```

### MCP

端点：`http://<host>:8080/mcp`。鉴权方式同上（Bearer Master Key 或访问密钥）。

**Claude Code**

```bash
claude mcp add tavily-proxy --transport http http://<host>:8080/mcp \
  --header "Authorization: Bearer <KEY>"
```

**claude.ai / Claude Desktop（自定义连接器）**

打开 Customize > Connectors > Add custom connector，按下面填写：

- URL：`https://<你的域名>/mcp`
- Authentication：选择 **No sign-in**
- Request headers：名称选 `authorization`，值填 `Bearer <KEY>`（`Bearer` 和空格要自己写上）

Request headers 目前是 Claude 的 beta 功能，只对部分账号开放。本项目暂不支持 OAuth 登录，看不到该选项的账号目前无法接入。连接器由 Claude 的服务器发起访问，所以服务必须能通过公网 HTTPS 访问到。

**VS Code**（`.vscode/mcp.json`）

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

**其他支持 `mcpServers` 的客户端**（Cursor 等）

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

服务首页（`http://<host>:8080`）提供 MCP、Claude Code、SSE、REST API 与 AstrBot 的接入模板，地址已自动填好，复制即可用。

> ChatGPT 的自定义 MCP 连接器只支持 OAuth，暂时无法接入。

---

## 配置

### 环境变量

| 变量 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `LISTEN_ADDR` | 监听地址 | `:8080` |
| `PORT` | 监听端口，仅在未设置 `LISTEN_ADDR` 时生效 | `8080` |
| `DATABASE_PATH` / `DB_PATH` | SQLite 数据库路径 | `./server/data/app.db` |
| `TAVILY_BASE_URL` | 上游 Tavily API 地址 | `https://api.tavily.com` |
| `UPSTREAM_TIMEOUT` | 上游请求超时（如 `30s`，或纯数字秒数） | `150s` |
| `MCP_STATELESS` | MCP 无状态模式 | `true` |
| `MCP_SESSION_TTL` | 有状态模式下的会话空闲超时 | `10m` |
| `LOG_DIR` | 日志文件目录，按天轮转；留空则只输出到标准输出 | 空 |
| `LOG_LEVEL` | 日志级别：`debug` / `info` / `warn` / `error` | `info` |

### 面板内设置

以下设置保存在数据库中，在管理面板的「设置」页修改，即时生效：

| 设置 | 说明 | 默认 |
| :--- | :--- | :--- |
| Key 池策略 | `best_remaining` / `round_robin` / `sequential_fill` | `best_remaining` |
| 搜索缓存 | 缓存 `POST /search` 的结果，TTL 可设 60 秒至 7 天 | 关闭，TTL 12 小时 |
| 自动额度同步 | 定期调用官方 `/usage` 校准额度，可设间隔、并发与请求间隔 | 关闭 |
| 请求日志 | 是否记录请求日志 | 开启 |
| 日志保留 | 自动清理超过保留天数的日志，设为 0 表示不清理 | 30 天 |

### 部署在反向代理后面

- 客户端 IP 取自 `X-Forwarded-For`。只信任来自本机和私有网段（`10/8`、`172.16/12`、`192.168/16`）的代理头。
- `UPSTREAM_TIMEOUT` 应小于前置代理的超时时间。例如 Cloudflare 的默认超时约为 100 秒，否则长请求会先被代理断开。
- 只有在把 `MCP_STATELESS` 设为 `false` 时，才需要透传 `Mcp-Session-Id` 并开启会话粘性。

健康检查端点：`GET /healthz`。

---

## 开发

环境要求：Go `1.27.3+`，Node.js `20+`。

```bash
# 后端
go run ./server

# 前端（另开一个终端；开发服务器会把 API 请求代理到后端）
cd web && npm install && npm run dev

# 测试
go test ./...
```

构建：

```bash
# 前端构建并内嵌，输出 Windows 与 Linux 二进制到 build/
./scripts/build_all.sh        # Windows 下用 .\scripts\build_all.ps1

# Docker 镜像（当前架构）
docker build -t tavily-proxy .
```

发布：推送 `v*` 标签后，GitHub Actions 会构建多架构镜像推送到 GHCR，并把各平台二进制发布到 Releases。

---

## 致谢

本项目基于 [xuncv/TavilyProxyManager](https://github.com/xuncv/TavilyProxyManager) 二次开发。

## 开源协议

MIT
