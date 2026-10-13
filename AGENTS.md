# AGENTS.md — HermesHub

把多台 Hermes 实例注册到平台，创建项目并拉入实例组成协作小组；用户在项目会话里 @ 实例发布指令，被点名的 Hermes 通过连接器接收指令、协作干活、回写看板/总结/共享文件。

## 技术栈与布局

- 后端 Go 1.25+（Gin + GORM + SQLite 纯 Go 驱动 `glebarez/sqlite`，无 CGO）；前端 Vue3 + Vite + Ant Design Vue，产物经 `go:embed` 嵌入服务端二进制
- `cmd/server` → `hermeshub-server`（平台：Gin 路由 + WS Hub + 编排器 + 管理/工具 API + 内嵌前端）
- `cmd/connector` → `hermeshub-connector`（连接器：WS 客户端 + ACP 客户端 + 隐藏 `toolbridge` 子命令）
- 关键：**平台与 agent 之间的驱动协议是 ACP**（`hermes acp` 子进程，JSON-RPC over stdio），不是 MCP；MCP 仅用作 agent → 平台的反向工具通道（工具桥 sidecar）

```
平台 WS 下发指令 → connector 启动 hermes acp 子进程 → session/new（注入 mcpServers=工具桥）
→ session/prompt 发指令 → 收集 agent_message_chunk → 结果回传平台
```

## 常用命令

```bash
make build            # 前端打包 + 编译两个二进制（本机无 make 时见 README 手动命令）
go build ./...        # 编译检查
go vet ./...          # 静态检查

# 本地运行（项目根目录）
./hermeshub-server              # 平台，:8080（需 .env，见 .env.example）
./hermeshub-connector --config connector.yaml   # 连接器（示例配置 configs/connector.example.yaml）
HERMESHUB_ACP_DEBUG=1 ./hermeshub-connector ... # 打印 ACP 全部 JSON-RPC 流量（排查利器）

# 前端开发
cd web && npm install && npm run dev   # Vite dev server，代理 /api /ws 到 :8080
```

## 配置

- 平台：`.env`（`ADMIN_PASSWORD` 必填，`PORT/DB_PATH/SHARED_FILES_ROOT/PLATFORM_PUBLIC_URL/...` 见 README 配置表）
- 连接器：`connector.yaml`（`platform_url`、`token`（实例页获取）、`acp.command/args/env/work_dir`）

## 关键设计约定

- **单二进制双人格**：`hermeshub-connector toolbridge` 是隐藏子命令，以 stdio MCP Server 跑 13 个平台工具（转发 `/api/tool/*`，Bearer 令牌经 env 按次派发注入），由 hermes 在 ACP 会话内拉起；**勿手动执行**
- 工具令牌（ToolToken）按「实例×项目」发放，随 WS 派发消息 `tool_token` 字段下发；指令上下文同时含 MCP 工具说明与 HTTP 工具 API 文档（MCP 优先、curl 兜底）
- agent 写回平台一律走 `/api/tool/*`（toolAuthMiddleware 按 ToolToken 鉴权）；管理端走 `/api/*`（JWT）
- 派发是「每条 @ 指令一个 ACP 会话」；hermes 会把每次 ACP 会话持久化到其主 profile 的 `state.db`（产品上有意保持记忆/技能共享主 profile，接受会话记录可见）
- 风暴保护：项目 10 分钟内自动派发超过 40 次即暂停
- 派发状态流转：pending → running → done/failed/timeout；**实例 WS 断开即收尾**——该实例所有 pending/running 派发被条件更新为 failed（error=哨兵「实例连接断开」，hub.go）并发系统消息；连接器重连后孤儿 ACP 的迟到 result 只允许覆盖 error 恰为哨兵的 failed，其余终态不覆盖（orchestrator.go `OnResult`）
- question 消息支持 `options` 快捷选项（`POST /api/tool/messages` 请求体可选 `options` 数组，存 Message.options JSON 文本）；用户回复走 `POST /api/projects/:id/messages` 的 `reply_to`，服务端自动 @ 原发送者并派发、近似标记 replied=true

## Hermes 兼容性实测结论（改动前先读）

- 连接器的 `hermes acp` 子进程必须注入 `PYTHONUTF8=1` + `PYTHONIOENCODING=utf-8`，否则 Windows 管道 GBK 编码中文乱码（`acp.go` 已内置，cfg.Env 可覆盖）
- ACP 实现是 hermes 私有方言，与规范不同（`acp.go`/`connector.go` 注释有记录）：
  - `session/new` 的 `mcpServers` 是**数组**，条目**扁平**（`name/type/command/args/env` 全在顶层，非 `server` 嵌套）
  - `env` 必须是 `[{"name": "K", "value": "V"}]` 字典数组
  - 流式通知字段是 `sessionUpdate`（如 `agent_message_chunk`/`tool_call`），且 `session/prompt` 响应后可能还有末段分片（acp.go 有排空逻辑）
- MCP 工具桥握手须**回显对方请求的 protocolVersion**，并实现 `server/discover`，绕开 hermes 的版本协商坑（toolbridge.go）

## Windows 环境注意

- Bash 工具是 Git Bash：`tasklist`/`taskkill` 按映像名过滤时进程名**不带 `.exe`**（`hermeshub-connector`），CSV 输出同理；杀进程建议 `tasklist | grep` 拿到 PID 后 `taskkill //F //PID <pid>`
- **每台机器只允许跑一个连接器**：平台设计为同实例新连接踢旧连接，重复进程会互踢成 5 秒重连循环
- Git Bash 命令行参数是 GBK 编码，curl 发中文 JSON 会变乱码——测试发中文请用 Python urllib 写 UTF-8 请求体
- 测试期间 go build 可能报「文件被占用」：先确认无 connector/hermes 子进程残留再重试

## 改代码时

- 指令上下文模板在 `internal/server/context.go`（平台规则、工具说明、项目信息、成员、看板、总结、文件清单、最近消息、当前指令）
- 派发链路：`orchestrator.Dispatch` → `hub.SendDispatch`（WS）→ connector `execute` → `runACP` → 结果 `OnResult` 回写
- 改 WS 协议（hub.go 消息结构）时平台与连接器必须**同步重新编译**，两端版本要一起发
- 无单测约定：验证靠编译 + `go vet` + 真实链路（起 server/connector，用 Python 发 UTF-8 请求触发派发，看 connector 日志与 DB 内容）
