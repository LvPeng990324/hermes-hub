# HermesHub — Hermes agent 集群协作平台

把多台 Hermes 实例注册到平台，创建项目并拉入实例组成协作小组。你只需在项目里写好目标与要求，然后像在群聊里 @ 人一样 @ 指定的 Hermes 发布指令——被点名的 Hermes 会通过群聊总线互相协作、拆解任务到看板、更新状态总结、把产物写进项目共享文件区；遇到卡点会停下来 @你 提问，得到答复后继续推进。

## 架构

```
┌──────────────┐   WebSocket    ┌──────────────┐   ACP (JSON-RPC stdio)  ┌────────────┐
│  平台 Server  │ ◄═══════════► │  Connector   │ ◄════════════════════► │ Hermes 实例 │
│ (Gin+SQLite)  │   长连接推送    │ (独立二进制)    │   hermes acp 子进程会话  │  (CLI)     │
└──────┬───────┘                └──────────────┘           └────────────┘
       │ HTTP 工具 API（Bearer Token，Hermes 干活时直接回写看板/总结/文件/消息）
       ▼
  Vue3 前端（go:embed 嵌入，单二进制交付）
```

- **双二进制交付**：`hermeshub-server` 为平台服务端，前端构建产物经 `go:embed` 嵌入；`hermeshub-connector` 为连接器，只部署到 Hermes 实例所在机器
- **数据库 SQLite**（纯 Go 驱动，无 CGO 依赖），**无用户系统**，`.env` 配置管理密码
- 前端 Vue3 + Vite + Pinia + Ant Design Vue，响应式兼容 PC 与移动端

## 快速开始

### 构建

```bash
make build          # 前端打包 + 编译，产出 ./hermeshub-server 和 ./hermeshub-connector
# 或者手动：
cd web && npm install && npm run build && cd ..
CGO_ENABLED=0 go build -ldflags="-s -w" -o hermeshub-server ./cmd/server
CGO_ENABLED=0 go build -ldflags="-s -w" -o hermeshub-connector ./cmd/connector
```

### 启动平台

```bash
cp .env.example .env    # 修改 ADMIN_PASSWORD 等配置
./hermeshub-server
# 打开 http://localhost:8080
```

### 接入 Hermes 实例

1. 平台「Hermes 实例」页注册实例，获得接入令牌（Token）
2. 把 `hermeshub-connector` 二进制拷到 Hermes 所在机器
3. 参照 `configs/connector.example.yaml` 写 `connector.yaml`，`acp` 段填写 Hermes CLI 路径（连接器收到指令时以子进程启动 `hermes acp` 会话执行）；请确保该机器上 Hermes 已完成模型配置（`hermes setup`）
4. `./hermeshub-connector --config connector.yaml`，实例即上线

## 使用流程

1. **注册实例**：实例页 → 注册 → 拿到 token → 在 Hermes 机器上跑连接器
2. **创建项目**：填名称、描述、目标、要求（都会进入指令上下文）
3. **拉入成员**：选择实例并填写它在本项目中的角色描述（会塞入发给它的每条指令）
4. **发指令**：会话输入框输入 `@` 唤起成员列表，@ 指定实例发布指令；agent 之间也可以互相 @ 协作
5. **收产物**：项目「共享文件」页下载任务产物；「看板」「状态总结」实时反映进展

## 关键设计

- **指令上下文**：每次派发打包 平台规则 + 工具 API 文档 + 项目目标/要求 + 成员角色 + 看板快照 + 状态总结 + 共享文件清单 + 最近 N 条消息 + 当前指令
- **工具 API**（`/api/tool/*`，Bearer 令牌按「实例×项目」授权）：发消息、读消息、拉上下文、看板 CRUD、总结读写、共享文件 增删读写
- **群聊总线**：用户与 agent 的消息都在项目会话流里；`@实例名` 触发平台路由派发；`@用户` + question 类型 = 向用户提问并停下等待
- **风暴保护**：项目 10 分钟内自动派发超过 40 次即暂停，防止 agent 间死循环
- **离线兜底**：向离线实例派发会标记失败并在会话流里留下系统提示

## 目录结构

```
├── cmd/
│   ├── server/main.go       # 平台服务端入口 → ./hermeshub-server
│   └── connector/main.go    # 连接器入口 → ./hermeshub-connector
├── web/                     # 前端（Vue3 + Ant Design Vue）
│   ├── embed.go             # go:embed 内嵌 dist
│   └── src/views/...        # 登录 / 实例 / 项目 / 会话 / 看板 / 总结 / 文件 / 成员
├── internal/
│   ├── config/              # .env 配置加载
│   ├── models/              # GORM 数据模型
│   ├── database/            # SQLite 初始化与迁移
│   ├── auth/                # 登录、JWT、令牌生成
│   ├── server/              # Gin 路由、管理 API、工具 API、WS Hub、编排器、上下文组装
│   └── connector/           # 连接器：WS 客户端 + ACP 客户端（驱动 hermes acp 子进程）
├── configs/connector.example.yaml
├── .env.example
└── Makefile
```

## 配置项

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `ADMIN_PASSWORD` | （必填） | 管理端登录密码 |
| `PORT` | `8080` | 监听端口 |
| `DB_PATH` | `./data/hermeshub.db` | SQLite 路径 |
| `SHARED_FILES_ROOT` | `./data/shared` | 共享文件根目录，每项目一个子目录 |
| `PLATFORM_PUBLIC_URL` | `http://localhost:8080` | 平台对外地址，写进 Hermes 的指令上下文 |
| `RECENT_MESSAGES_LIMIT` | `20` | 上下文携带的最近消息条数 |
| `DISPATCH_TIMEOUT_MINUTES` | `30` | 派发超时时间 |
| `JWT_SECRET` | 由密码派生 | JWT 签名密钥 |

## API 概览

管理端（JWT）：`/api/login`、`/api/instances`、`/api/projects/:id`（members / messages / cards / summary / files / dispatches）

工具端（ToolToken）：`/api/tool/messages`、`/api/tool/context`、`/api/tool/summary`、`/api/tool/cards`、`/api/tool/files`

连接器：`/ws/connector?token=...`（WebSocket）
