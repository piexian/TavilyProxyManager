[根目录](../CLAUDE.md) > **server**

# server -- Go 后端核心

---

## 模块职责

Go 后端服务，提供 Tavily API 反向代理、Key 池管理、REST API、MCP 端点、嵌入式 Web UI 静态文件服务，以及定时任务（月度额度重置、自动额度同步、日志清理）。

---

## 入口与启动

- **入口文件**：`server/main.go`
- **启动命令**：`go run ./server`
- **流程**：
  1. 加载环境变量配置（`config.FromEnv`）
  2. 打开 SQLite 数据库 + AutoMigrate
  3. 初始化 Master Key（首次运行自动生成）
  4. 初始化所有 Service（Settings、Key、Log、Stats、TavilyProxy、QuotaSync）
  5. 构建 HTTP Server（Gin router + 嵌入式前端）
  6. 启动定时任务（MonthlyReset、AutoQuotaSync、LogCleanup）
  7. 监听信号，优雅关闭

---

## 对外接口

### REST API（`/api/*`，需 Bearer Master Key）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/keys` | 列出所有 Key（掩码显示） |
| POST | `/api/keys` | 添加新 Key |
| GET | `/api/keys/export` | 导出有效 Key（纯文本下载） |
| GET | `/api/keys/:id/raw` | 获取 Key 原文 |
| PUT | `/api/keys/:id` | 更新 Key 属性 |
| DELETE | `/api/keys/:id` | 删除 Key |
| DELETE | `/api/keys/invalid` | 批量删除无效 Key |
| GET | `/api/keys/sync` | 获取同步任务状态 |
| POST | `/api/keys/sync` | 启动批量额度同步 |
| GET | `/api/logs` | 请求日志分页查询 |
| GET | `/api/logs/status-codes` | 日志状态码分布 |
| DELETE | `/api/logs` | 清空日志 |
| GET | `/api/stats` | 仪表盘统计数据 |
| GET | `/api/stats/timeseries` | 时间序列数据（hour/day/month） |
| GET | `/api/settings/master-key` | 获取 Master Key |
| POST | `/api/settings/master-key/reset` | 重置 Master Key |
| GET/PUT | `/api/settings/auto-sync` | 自动同步配置 |
| GET/PUT | `/api/settings/log-cleanup` | 日志清理配置 |

### 代理端点（任意路径，需 Master Key）

- 所有非 `/api/*` 路径的请求，若携带有效 Master Key，则透明代理到 Tavily
- 支持 Bearer header、body `api_key`/`apiKey`、query `api_key` 三种鉴权方式
- 自动从请求体/query 中剥离 Key 字段后转发

### MCP 端点

- `ANY /mcp` -- Streamable HTTP MCP，工具：`tavily-search`、`tavily-extract`、`tavily-crawl`、`tavily-map`、`tavily-usage`

### 其他

- `GET /healthz` -- 健康检查

---

## 关键依赖与配置

### 依赖

| 依赖 | 用途 |
|------|------|
| `github.com/gin-gonic/gin` | HTTP 路由框架 |
| `github.com/glebarez/sqlite` | 纯 Go SQLite 驱动 |
| `gorm.io/gorm` | ORM |
| `github.com/google/uuid` | 请求 ID 生成 |
| `github.com/modelcontextprotocol/go-sdk` | MCP 协议支持 |

### 配置

通过环境变量加载（`internal/config/config.go`）：
- `LISTEN_ADDR` / `PORT`
- `DB_PATH` / `DATABASE_PATH`
- `TAVILY_BASE_URL`
- `UPSTREAM_TIMEOUT`

---

## 数据模型

定义于 `internal/models/models.go`：

| 模型 | 说明 | 关键字段 |
|------|------|----------|
| `APIKey` | Tavily API Key | `Key`, `Alias`, `TotalQuota`, `UsedQuota`, `IsActive`, `IsInvalid` |
| `RequestLog` | 代理请求日志 | `RequestID`, `KeyUsed`, `Endpoint`, `StatusCode`, `LatencyMs`, `RequestBody`, `ResponseBody` |
| `RequestStat` | 聚合统计（hour/day/month） | `Granularity`, `Bucket`, `Endpoint`, `Count` |
| `Setting` | 键值对配置 | `Key`, `Value` |

数据库使用 GORM `AutoMigrate` 自动建表/迁移。

---

## 内部包结构

| 包 | 路径 | 职责 |
|-----|------|------|
| `config` | `internal/config/` | 环境变量配置加载 |
| `db` | `internal/db/` | SQLite 数据库初始化与迁移 |
| `models` | `internal/models/` | GORM 数据模型定义 |
| `httpserver` | `internal/httpserver/` | HTTP Server 创建、Gin 路由注册、请求处理 |
| `services` | `internal/services/` | 业务逻辑层（Key 管理、代理、日志、统计、设置、额度同步、Master Key） |
| `jobs` | `internal/jobs/` | 定时任务（月度重置、自动同步、日志清理） |
| `mcpserver` | `internal/mcpserver/` | MCP 协议处理、Tavily 工具注册 |
| `util` | `internal/util/` | 工具函数（API Key 掩码） |

---

## 测试与质量

- **测试文件**：
  - `internal/httpserver/router_export_test.go` -- Key 导出排除无效 Key
  - `internal/httpserver/router_proxy_legacy_test.go` -- Legacy body/query API Key 兼容
  - `internal/httpserver/router_logs_filter_test.go` -- 日志状态码过滤
  - `internal/services/tavily_proxy_logging_test.go` -- 日志开关测试
  - `internal/services/quota_sync_service_test.go` -- 额度同步状态机（429/433/401）
  - `internal/services/quota_sync_job_service_test.go` -- 后台同步任务进度
- **运行**：`go test ./server/...`
- **测试模式**：使用 `gin.TestMode`，`httptest.Server` 模拟上游，SQLite 临时数据库

---

## 常见问题 (FAQ)

1. **首次运行如何获取 Master Key？** -- 查看容器日志 `grep "master key"`
2. **如何添加 Tavily Key？** -- Web UI Key 管理页面或 `POST /api/keys`
3. **Key 自动切换逻辑？** -- 按剩余额度降序排列，同额度随机打散，遇 401/429/432/433 自动尝试下一个
4. **月度额度重置？** -- 每月 1 日 00:00 自动重置所有 Key 的 `used_quota`

---

## 相关文件清单

```
server/
  main.go                                    -- 入口
  public/                                    -- 嵌入式前端构建产物
  internal/
    config/config.go                         -- 环境变量配置
    db/db.go                                 -- 数据库初始化
    models/models.go                         -- GORM 数据模型
    httpserver/
      server.go                              -- HTTP Server 创建
      router.go                              -- 路由注册与请求处理
      router_export_test.go                  -- 测试
      router_proxy_legacy_test.go            -- 测试
      router_logs_filter_test.go             -- 测试
    services/
      master_key.go                          -- Master Key 管理
      key_service.go                         -- API Key CRUD 与候选排序
      tavily_proxy.go                        -- 代理核心引擎
      log_service.go                         -- 请求日志服务
      stats_service.go                       -- 统计聚合服务
      settings_service.go                    -- 键值对设置服务
      settings_keys.go                       -- 设置键名常量
      quota_sync_service.go                  -- 额度同步服务
      quota_sync_job_service.go              -- 后台同步任务管理
      tavily_proxy_logging_test.go           -- 测试
      quota_sync_service_test.go             -- 测试
      quota_sync_job_service_test.go         -- 测试
    jobs/
      monthly_reset.go                       -- 月度额度重置
      auto_quota_sync.go                     -- 自动额度同步
      log_cleanup.go                         -- 日志清理
    mcpserver/mcpserver.go                   -- MCP 端点与工具
    util/masking.go                          -- API Key 掩码
```

---

## 变更记录 (Changelog)

| 时间 | 操作 | 说明 |
|------|------|------|
| 2026-02-13 19:14:52 | 初始生成 | 首次扫描，覆盖全部源文件与测试 |
