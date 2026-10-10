[根目录](../CLAUDE.md) > **web**

# web -- 前端管理面板

## 模块职责

Tavily Proxy Manager 的浏览器端管理界面。基于 **Vue 3 + TypeScript + Naive UI** 构建，提供以下功能：

- 仪表盘 (Dashboard)：展示总额度、已用额度、活跃密钥数、今日请求数、额度消耗进度条、请求时序图表 (ECharts)。
- 密钥管理 (Key Management)：CRUD 单个/批量 API 密钥、同步用量、导出密钥、删除无效密钥。
- 请求日志 (Logs)：分页浏览代理请求历史，按状态码筛选，查看请求/响应体详情，清空日志。
- 系统设置 (Settings)：查看/重置主密钥、配置额度自动同步策略、配置日志清理策略。
- 认证：通过 Master Key 模态框进行登录验证，Token 存储于 `localStorage`。
- 国际化 (i18n)：内置中文 (`zh-CN`) 和英文 (`en`) 两种语言，支持浏览器语言自动检测与手动切换。
- 暗色/亮色主题切换，主题偏好持久化到 `localStorage`。

## 入口与构建命令

| 命令 | 说明 |
|------|------|
| `npm run dev` | 启动 Vite 开发服务器（默认 `http://localhost:5173`），API 请求代理到 `http://localhost:8080` |
| `npm run build` | 先运行 `vue-tsc -b` 类型检查，再执行 `vite build`，产物输出到 `dist/` |
| `npm run preview` | 本地预览生产构建产物 |

- **HTML 入口**：`index.html` -> 加载 `src/main.ts`
- **应用入口**：`src/main.ts` -- 调用 `applyLocaleToDocument()` 设置文档语言与标题，然后创建 Vue 应用并挂载到 `#app`。
- **根组件**：`src/App.vue` -- 包含 Naive UI 全局配置 (`NConfigProvider`)、侧边栏菜单导航、顶栏（主题切换、语言切换、退出按钮）、四个视图的条件渲染、以及 `MasterKeyModal` 认证弹窗。

## 目录结构

```
web/
  index.html                  HTML 入口
  package.json                依赖与脚本
  vite.config.ts              Vite 配置（含 /api 代理）
  tsconfig.json               TypeScript 配置（应用源码）
  tsconfig.node.json          TypeScript 配置（Vite 节点端）
  public/
    favicon.ico               网站图标
  src/
    main.ts                   应用入口
    App.vue                   根组件（布局、导航、认证）
    env.d.ts                  Vite 客户端类型声明
    types.ts                  全局 TypeScript 类型定义
    i18n.ts                   国际化模块（中英双语）
    api/
      client.ts               Axios 实例与 Master Key 管理
    components/
      MasterKeyModal.vue      认证弹窗组件
    views/
      DashboardView.vue       仪表盘视图
      KeyManagementView.vue   密钥管理视图
      LogsView.vue            请求日志视图
      SettingsView.vue        系统设置视图
    utils/
      clipboard.ts            剪贴板写入工具函数
```

## 关键依赖

| 包名 | 版本 | 用途 |
|------|------|------|
| `vue` | ^3.4.27 | 前端框架 |
| `naive-ui` | ^2.38.2 | UI 组件库 |
| `axios` | ^1.6.8 | HTTP 客户端 |
| `echarts` | ^5.5.0 | 图表可视化（仪表盘时序图） |
| `@vicons/ionicons5` | ^0.13.0 | 图标库 |
| `@vicons/utils` | ^0.1.4 | 图标工具 |
| `vite` | ^5.2.11 | 构建工具 |
| `typescript` | ^5.4.5 | 类型系统 |
| `vue-tsc` | ^2.0.19 | Vue 文件类型检查 |
| `@vitejs/plugin-vue` | ^5.0.5 | Vite Vue 插件 |

## 组件清单

### 视图组件 (`src/views/`)

| 组件 | 路径 | 职责 |
|------|------|------|
| `DashboardView` | `src/views/DashboardView.vue` | 统计卡片（剩余额度、已用额度、活跃密钥、今日请求）、资源使用进度条、ECharts 请求时序图（支持小时/天/月粒度切换）。通过 `refreshNonce` prop 实现认证后自动刷新。 |
| `KeyManagementView` | `src/views/KeyManagementView.vue` | 密钥数据表格（别名、密钥脱敏、用量/额度进度条、状态开关、操作按钮）。支持单个添加、批量添加、编辑、删除、复制原始密钥、同步用量、重置额度、导出全部密钥、批量同步（后台轮询进度）、删除无效密钥。 |
| `LogsView` | `src/views/LogsView.vue` | 请求日志分页表格，支持按状态码筛选、查看请求/响应 JSON 详情、复制内容、清空全部日志。 |
| `SettingsView` | `src/views/SettingsView.vue` | 三块设置卡片：主密钥认证（查看/复制/重置）、额度自动同步（开关/间隔/延迟/历史状态）、日志清理（开关/保留天数/历史状态）。 |

### 公共组件 (`src/components/`)

| 组件 | 路径 | 职责 |
|------|------|------|
| `MasterKeyModal` | `src/components/MasterKeyModal.vue` | 全屏不可关闭的认证模态框。接收 `show`、`initialValue`、`error` props，发出 `submit` 事件。内置语言切换下拉。 |

## API 客户端说明

**文件**：`src/api/client.ts`

- 创建一个无 `baseURL` 的 Axios 实例 (`api`)，开发环境下由 Vite 代理 `/api` 路径到后端 `http://localhost:8080`。
- **Master Key 管理**：通过三个导出函数 (`getMasterKey`、`setMasterKey`、`clearMasterKey`) 管理存储于 `localStorage` (`tavily_proxy_master_key`) 的认证令牌。内部使用 Vue `ref` 实现响应式。
- **请求拦截器**：自动在每个请求的 `Authorization` 头中附加 `Bearer <masterKey>`。
- **响应拦截器**：当收到 401 响应时，自动清除 Master Key 并通过 `window.dispatchEvent(new Event('auth-required'))` 触发全局认证事件，`App.vue` 监听此事件以弹出认证模态框。

### 后端 API 端点列表（前端调用）

| 方法 | 端点 | 调用方 |
|------|------|--------|
| `GET` | `/api/stats` | DashboardView -- 获取统计摘要 |
| `GET` | `/api/stats/timeseries?granularity=hour\|day\|month` | DashboardView -- 获取时序数据 |
| `GET` | `/api/keys` | KeyManagementView -- 获取密钥列表 |
| `POST` | `/api/keys` | KeyManagementView -- 创建单个密钥 |
| `PUT` | `/api/keys/:id` | KeyManagementView -- 更新密钥（含 sync_usage、reset_quota） |
| `DELETE` | `/api/keys/:id` | KeyManagementView -- 删除单个密钥 |
| `GET` | `/api/keys/:id/raw` | KeyManagementView -- 获取原始密钥（用于复制） |
| `DELETE` | `/api/keys/invalid` | KeyManagementView -- 批量删除无效密钥 |
| `GET` | `/api/keys/sync` | KeyManagementView -- 查询批量同步任务状态 |
| `POST` | `/api/keys/sync` | KeyManagementView -- 启动批量同步任务 |
| `GET` | `/api/keys/export` | KeyManagementView -- 导出全部密钥为文本文件 |
| `GET` | `/api/logs?page=&page_size=&status_code=` | LogsView -- 分页获取日志 |
| `GET` | `/api/logs/status-codes` | LogsView -- 获取状态码统计 |
| `DELETE` | `/api/logs` | LogsView -- 清空全部日志 |
| `GET` | `/api/settings/master-key` | SettingsView -- 获取当前主密钥 |
| `POST` | `/api/settings/master-key/reset` | SettingsView -- 重置主密钥 |
| `GET` | `/api/settings/auto-sync` | SettingsView -- 获取自动同步配置 |
| `PUT` | `/api/settings/auto-sync` | SettingsView -- 更新自动同步配置 |
| `GET` | `/api/settings/log-cleanup` | SettingsView -- 获取日志清理配置 |
| `PUT` | `/api/settings/log-cleanup` | SettingsView -- 更新日志清理配置 |

## 国际化 (i18n)

**文件**：`src/i18n.ts`

- 使用纯函数式方案，无第三方 i18n 库。通过 Vue `ref` 管理当前 locale。
- 导出 `t(key, params?)` 翻译函数，支持 `{placeholder}` 插值。
- 语言偏好持久化到 `localStorage` (`tavily_proxy_locale`)，首次访问自动检测浏览器语言。
- 支持的语言：`en`（英文，默认回退语言）、`zh-CN`（简体中文）。

## TypeScript 类型定义

**文件**：`src/types.ts`

| 类型 | 说明 |
|------|------|
| `Stats` | 仪表盘统计数据（total_quota、total_used、total_remaining、key_count、active_key_count、today_requests） |
| `TimeSeries` | 时序图表数据（granularity、labels、series） |
| `KeyItem` | API 密钥条目（id、key、alias、total_quota、used_quota、is_active、is_invalid、last_used_at、created_at） |
| `LogItem` | 请求日志条目（id、request_id、key_used、key_alias、endpoint、status_code、latency、request_body、response_body、client_ip、created_at） |
| `LogStatusCount` | 状态码统计（status_code、count） |

## 工具函数

| 函数 | 文件 | 说明 |
|------|------|------|
| `writeClipboardText(text)` | `src/utils/clipboard.ts` | 优先使用 `navigator.clipboard.writeText`，降级使用 `document.execCommand('copy')`。创建隐藏 textarea 实现兼容性复制。 |

## 路由说明

本项目**未使用 vue-router**。`App.vue` 通过 `active` ref 变量（类型为 `'dashboard' | 'keys' | 'logs' | 'settings'`）配合 `v-if`/`v-else-if` 条件渲染实现视图切换。侧边栏 `n-menu` 的 `v-model:value` 绑定此变量。

## 测试与质量

- 当前未发现测试文件或测试配置。
- `tsconfig.json` 中有 `exclude: ["src/**/__tests__/*"]` 配置，预留了测试目录结构。
- 无 ESLint / Prettier / Stylelint 等代码质量工具配置。

## Vite 配置要点

- 使用 `@vitejs/plugin-vue` 插件。
- 开发服务器代理：`/api` -> `http://localhost:8080`（后端服务地址），开启 `changeOrigin`。
- 生产构建产物默认输出到 `dist/`。

## 常见问题 (FAQ)

**Q: 如何添加新页面/视图?**
A: 在 `src/views/` 下创建新的 `.vue` 文件，在 `App.vue` 中添加 import 和条件渲染分支，并在 `menuOptions` 中添加对应菜单项。同时更新 `active` ref 的类型联合。

**Q: 如何添加新的 i18n 翻译键?**
A: 在 `src/i18n.ts` 的 `MESSAGES` 对象中同时为 `en` 和 `zh-CN` 添加新的键值对。

**Q: 前端如何与后端通信?**
A: 所有 API 请求通过 `src/api/client.ts` 导出的 `api` Axios 实例发送。开发环境使用 Vite 代理，生产环境需要后端直接提供 `/api` 路径（或通过反向代理）。

## 相关文件清单

```
web/index.html
web/package.json
web/vite.config.ts
web/tsconfig.json
web/tsconfig.node.json
web/public/favicon.ico
web/src/main.ts
web/src/App.vue
web/src/env.d.ts
web/src/types.ts
web/src/i18n.ts
web/src/api/client.ts
web/src/components/MasterKeyModal.vue
web/src/views/DashboardView.vue
web/src/views/KeyManagementView.vue
web/src/views/LogsView.vue
web/src/views/SettingsView.vue
web/src/utils/clipboard.ts
```

## 变更记录 (Changelog)

| 时间 | 操作 | 说明 |
|------|------|------|
| 2026-02-13 19:14:52 | 初始生成 | 扫描 web/ 全部 18 个源文件，生成模块级文档。覆盖率 100%。 |
