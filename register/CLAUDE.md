[根目录](../CLAUDE.md) > **register**

# register -- Tavily 批量注册工具

## 模块职责

批量自动注册 Tavily 账户并获取 API Key 的辅助工具。通过 GPTMail 临时邮箱服务生成一次性邮箱地址，借助视觉大模型（OpenAI 兼容接口）自动识别验证码，完成注册、邮箱验证、登录、API Key 获取的全流程自动化。

## 入口与运行命令

| 用途 | 命令 |
|------|------|
| 查看帮助 | `uv run python batch_signup.py --help` |
| 批量注册（默认 1 个） | `uv run python batch_signup.py` |
| 批量注册 N 个 | `uv run python batch_signup.py -n 10` |
| 使用自定义邮箱列表 | `uv run python batch_signup.py -i email.txt` |
| 重试失败记录 | `uv run python batch_signup.py --retry` |
| 使用自定义 GPTMail Key | `uv run python batch_signup.py --gptmail-api-key <key>` |
| 调试首次登录初始化 | `uv run python batch_signup.py --debug-init` |

- **入口文件**: `batch_signup.py`（CLI，`argparse` 驱动）
- **环境要求**: Python >= 3.12，推荐使用 `uv` 管理依赖
- **安装依赖**: `uv sync`

## 目录结构与文件清单

```
register/
  batch_signup.py          # CLI 入口，批量注册编排逻辑
  signup.py                # 核心注册/登录/验证/取 Key 逻辑（requests.Session 驱动）
  gptmail_client.py        # GPTMail 临时邮箱 API 客户端
  config.yaml.example      # 配置文件模板
  banned_domains.txt        # 被禁域名黑名单（运行时动态追加）
  pyproject.toml            # 项目元数据与依赖声明
  .python-version           # Python 版本锁定 (3.12)
  .gitignore                # 忽略规则
  README.md                 # 原有说明文档
  uv.lock                   # uv 锁文件（二进制/大文件，不读内容）
```

**运行时生成的文件**（已在 `.gitignore` 中忽略）:

| 文件 | 说明 |
|------|------|
| `config.yaml` | 实际配置文件（含敏感信息，从 `config.yaml.example` 复制后填写） |
| `api_keys.txt` | 注册成功记录，格式 `email----api_key` |
| `failed.txt` | 注册失败记录，格式 `email----error` |

## 关键依赖

| 包 | 版本要求 | 用途 |
|----|---------|------|
| `requests` | >= 2.32.5 | HTTP 请求（注册/登录/验证全流程） |
| `pyaml` | >= 25.7.0 | 解析 `config.yaml` 配置文件 |
| `svglib` | >= 1.6.0 | SVG 验证码图片转 PNG |
| `reportlab` | >= 4.4.9 | 配合 svglib 渲染 SVG 到 PNG 位图 |
| `pysocks` | >= 1.7.1 | SOCKS 代理支持 |

**可选依赖**（代码中条件导入）:

| 包 | 用途 |
|----|------|
| `playwright` | 通过浏览器前端页面触发 API Key 异步生成（备选方案） |
| `rlPyCairo` / `pycairo` | Cairo 后端渲染 SVG（当编译版 renderPM 不可用时） |

## 核心流程说明

### 整体注册流程（单账户）

```
1. 生成临时邮箱 (GPTMail API)
   |-- 检查域名是否在 banned_domains.txt 中，命中则重新生成
2. 访问 Tavily 注册页面 (Auth0)
   |-- 获取 signup URL + state 参数
3. 提取 SVG 验证码 -> 转 PNG -> 视觉大模型识别
4. 提交注册表单 Step1（邮箱 + 验证码）
5. 提交注册表单 Step2（设置密码）
6. 通过 GPTMail 轮询验证邮件，提取验证链接
7. 访问验证链接完成邮箱验证
8. 登录并获取 API Key
   |-- 优先用验证后的 session 直接获取
   |-- 备选：重新登录 -> get_api_keys -> create_api_key
9. 保存结果到 api_keys.txt / failed.txt
```

### 三个源文件的职责划分

**`batch_signup.py`** -- 批量编排层
- CLI 参数解析（argparse）
- 批量循环调度：邮箱生成 -> 注册 -> 验证 -> 取 Key
- 禁用域名管理（加载/追加 `banned_domains.txt`）
- 去重（跳过已注册邮箱）、失败重试、IP 封禁检测
- 结果持久化（`api_keys.txt` / `failed.txt`）
- 统计输出（成功/失败/跳过/耗时）

**`signup.py`** -- 核心注册引擎
- `create_session()`: 创建带浏览器 UA 的 requests Session
- `load_config()`: 加载 `config.yaml`
- `get_signup_page()`: 获取 Auth0 注册页面 URL + state
- `fetch_page_with_captcha()`: 请求页面并提取 SVG 验证码
- `svg_to_png_base64()`: SVG 转 PNG（svglib + reportlab）
- `recognize_captcha_with_vision()`: 调用视觉大模型识别验证码
- `submit_signup_step1()`: 提交邮箱 + 验证码
- `submit_signup_password()`: 提交密码设置
- `verify_email()`: 访问验证链接完成邮箱验证
- `login_after_verification()`: 验证后登录
- `get_api_keys()`: 获取 API Key（含预热、自动创建、Playwright 备选）
- `create_api_key()`: 主动创建 API Key
- `run_first_login_init()`: 模拟新账号首次登录初始化流程
- `signup()`: 编排上述步骤的完整注册流程

**`gptmail_client.py`** -- 临时邮箱客户端
- `GPTMailClient` 类：封装 GPTMail API
  - `generate_email()`: 生成临时邮箱地址
  - `list_emails()`: 列出收件箱邮件
  - `get_email()`: 获取邮件详情
  - `delete_email()` / `clear_mailbox()`: 清理邮件
  - `wait_for_verification_link()`: 轮询等待 Tavily 验证邮件并提取链接
- `GPTMailAPIError`: 自定义异常

### 关键常量

| 常量 | 默认值 | 说明 |
|------|--------|------|
| `PASSWORD` | `Tavily@2024Test` | 注册使用的默认密码 |
| `REGISTER_INTERVAL` | 5 秒 | 批量注册间隔 |
| `VERIFY_TIMEOUT` | 180 秒 | 等待验证邮件超时 |
| `VERIFY_POLL_INTERVAL` | 5.0 秒 | 验证邮件轮询间隔 |
| `MAX_EMAIL_GENERATE_ATTEMPTS` | 30 | 生成未被禁域名邮箱的最大尝试次数 |
| `MAX_DOMAIN_BLOCKED_RETRIES` | 10 | 域名被禁后重试换邮箱的最大次数 |

## 配置说明

### config.yaml

从 `config.yaml.example` 复制并填写。用于配置视觉大模型（验证码识别）：

```yaml
OPENAI_BASEURL: https://example.com/v1    # OpenAI 兼容 API 地址
OPENAI_API_KEY: your_key                   # API 密钥
OPENAI_MODEL: gemini-3-flash              # 支持视觉的模型名称
```

### 环境变量（GPTMail 相关）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `GPTMAIL_BASE_URL` | `https://mail.chatgpt.org.uk` | GPTMail 服务地址 |
| `GPTMAIL_API_KEY` | `gpt-test` | GPTMail API 密钥 |
| `GPTMAIL_TIMEOUT` | `30` | 请求超时秒数 |
| `GPTMAIL_PREFIX` | (空) | 生成邮箱时的前缀 |
| `GPTMAIL_DOMAIN` | (空) | 指定邮箱域名 |

### CLI 参数一览

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--count` | `-n` | 1 | 注册数量 |
| `--input` | `-i` | (空) | 邮箱列表文件路径 |
| `--output` | `-o` | `api_keys.txt` | 成功输出文件 |
| `--failed` | | `failed.txt` | 失败记录文件 |
| `--banned-domains` | | `banned_domains.txt` | 禁用域名文件 |
| `--password` | | `Tavily@2024Test` | 注册密码 |
| `--interval` | | 5 | 注册间隔秒数 |
| `--retry` | | false | 重试失败记录 |
| `--gptmail-base-url` | | 环境变量 | GPTMail 服务地址 |
| `--gptmail-api-key` | | 环境变量 | GPTMail 密钥 |
| `--gptmail-timeout` | | 30 | GPTMail 超时 |
| `--gptmail-prefix` | | 环境变量 | 邮箱前缀 |
| `--gptmail-domain` | | 环境变量 | 邮箱域名 |
| `--max-generate-attempts` | | 30 | 邮箱生成最大重试 |
| `--verify-timeout` | | 180 | 验证邮件等待超时 |
| `--verify-interval` | | 5.0 | 验证轮询间隔 |
| `--debug-init` | | false | 打印首次登录初始化调试信息 |

## 常见问题 (FAQ)

| 错误 | 原因与处理 |
|------|-----------|
| `ip-signup-blocked` | 当前出口 IP 被 Tavily 禁止注册，脚本自动终止批量流程 |
| `custom-script-error-code_extensibility_error` | 邮箱域名被禁止，脚本自动将域名加入 `banned_domains.txt` 并换邮箱重试 |
| `invalid-captcha` | 验证码识别不正确，可降低并发或增加重试间隔 |
| IP 注册限制 | Tavily 策略：一个 IP 一段时间内只能注册约 5 个账号 |

## 相关文件清单

| 文件 | 行数 | 说明 |
|------|------|------|
| `batch_signup.py` | ~665 | CLI 入口与批量编排 |
| `signup.py` | ~1861 | 注册/登录/验证引擎 |
| `gptmail_client.py` | ~281 | GPTMail API 客户端 |
| `config.yaml.example` | 4 | 配置模板 |
| `banned_domains.txt` | 26 | 禁用域名黑名单 |
| `pyproject.toml` | 13 | 项目元数据 |

## 变更记录 (Changelog)

| 时间 | 操作 | 说明 |
|------|------|------|
| 2026-02-13 19:14:52 | 初始生成 | 首次生成模块级 CLAUDE.md 文档 |
