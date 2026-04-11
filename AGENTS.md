# AGENTS.md — AI 智能体开发维护指南

> 本文档面向未来接手本项目的 AI 智能体，提供项目全景、开发规范、部署流程和关键决策记录。

---

## 一、项目概览

**agent-memory** 是一个自托管的 AI Agent 结构化记忆管理框架。它为多个 AI Agent 提供共享的记忆存储服务，支持多 Agent 隔离、语义检索、自动分类去重、TTL 生命周期管理等能力。

### 核心能力

| 能力 | 说明 |
|------|------|
| 多 Agent 隔离 | 通过 `X-User-Id` header 实现每个 Agent 独立的记忆空间 |
| 自动分类 | 将记忆自动归类为 identity / principle / knowledge / working 四大类 |
| 语义去重 | 基于向量相似度检测重复记忆并合并，各分类阈值不同（identity: 0.95, principle: 0.90, knowledge: 0.85, working: 0.70） |
| TTL 生命周期 | active → degraded → archived → deleted，自动扫描降级 |
| 多维评分检索 | 相似度(40%) + 优先级(25%) + 热度(15%) + 分类(10%) + 紧迫度(10%) |
| 三级可见性 | private（仅创建者）/ team（同组只读）/ user（同用户读写） |
| Web Dashboard | 内嵌式管理界面，JWT 认证登录，支持记忆/Agent 管理、统计报表、系统配置 |
| OpenClaw 插件 | 配套 TypeScript 插件，实现 autoRecall（自动召回）和 autoCapture（自动捕获） |

### 技术栈

- **后端**：Go 1.25（模块路径 `github.com/lomehong/agent-memory`）
- **数据库**：SQLite（modernc.org/sqlite，纯 Go 无 CGO 依赖）+ Qdrant（可选向量库）
- **Embedding**：ONNX Runtime / OpenAI 兼容 API / Mock 三种模式
- **前端**：纯 HTML + CSS + JS（Chart.js 图表），通过 Go `embed.FS` 嵌入二进制
- **配套插件**：TypeScript，作为 OpenClaw 扩展运行

---

## 二、项目结构

```
agent-memory/
├── backend/                        # Go 后端源码（编译目标）
│   ├── cmd/server/main.go          # 服务入口（HTTP 路由 + embed 前端）
│   ├── cmd/migrate/main.go         # Mem0 迁移工具
│   ├── internal/
│   │   ├── api/
│   │   │   ├── middleware.go       # API Key / JWT 认证中间件 + AgentInfo 上下文
│   │   │   ├── auth_handler.go     # JWT 登录/登出/me 接口
│   │   │   └── json.go             # JSON 响应工具函数
│   │   ├── auth/auth.go            # JWT 令牌签发/验证
│   │   ├── config/config.go        # YAML 配置加载 + 环境变量展开
│   │   ├── core/
│   │   │   ├── classifier.go       # 自动分类器（规则 + 关键词匹配）
│   │   │   ├── writer.go           # 记忆写入（去重检查 + 建议生成）
│   │   │   ├── retriever.go        # 记忆检索（语义搜索 + 权限过滤 + 多维排序）
│   │   │   ├── compressor.go       # 批量压缩（相似合并 + 过期归档）
│   │   │   └── ttl_manager.go      # TTL 生命周期管理（定时扫描降级）
│   │   ├── embedding/
│   │   │   ├── provider.go         # EmbeddingProvider 接口定义
│   │   │   ├── mock.go             # Mock（零向量，测试用）
│   │   │   ├── onnx.go             # ONNX Runtime 本地推理
│   │   │   └── openai.go           # OpenAI 兼容 API 远程调用
│   │   ├── model/
│   │   │   ├── memory.go           # Memory 数据模型 + 常量定义
│   │   │   ├── agent.go            # Agent 数据模型
│   │   │   └── log.go              # 操作日志模型
│   │   ├── plugin/openclaw.go      # OpenClaw 工具 Schema 定义（供插件调用）
│   │   └── storage/
│   │       ├── dal.go              # DAL 接口定义
│   │       ├── sqlite.go           # SQLite 实现（CRUD + 聚合查询）
│   │       └── vector.go           # VectorStore 接口 + Memory/Qdrant 实现
│   ├── pkg/scoring/scorer.go       # 多维评分算法
│   ├── go.mod / go.sum
│   └── cmd/server/web/             # ⚠️ 构建时自动生成，勿手动编辑
│       └── (前端资源副本)
├── frontend/                       # 前端源码（✅ 编辑这里）
│   ├── index.html
│   ├── css/style.css
│   └── js/
│       ├── api.js                  # API 客户端封装
│       ├── app.js                  # 路由 + 登录 + 工具函数（openModal 等）
│       ├── dashboard.js            # 统计仪表盘（Chart.js）
│       ├── memories.js             # 记忆管理页面
│       ├── agents.js               # Agent 管理页面 + Agent 记忆查看
│       └── system.js               # 系统配置页面
├── config.yaml                     # 运行时配置文件
├── Dockerfile                      # 多阶段构建
├── docker-compose.yml              # Docker 编排（agent-memory + qdrant）
├── Makefile                        # 构建/部署命令
├── docs/                           # 设计和需求文档
│   ├── design.md                   # 系统设计文档
│   ├── requirements.md             # 需求规格
│   ├── web-design.md               # Web 界面设计
│   └── web-requirements.md         # Web 界面需求
└── README.md                       # 项目说明文档
```

### 配套仓库

| 仓库 | 说明 |
|------|------|
| [agent-memory](https://github.com/lomehong/agent-memory) | 本仓库（后端 + 前端） |
| [agent-memory-plugin](https://github.com/lomehong/agent-memory-plugin) | OpenClaw 插件（TypeScript） |

---

## 三、开发环境搭建

### 前置条件

- Go 1.25+
- Node.js 18+（仅插件开发需要）
- SQLite 命令行工具（可选，调试用）

### 本地编译和运行

```bash
# 克隆仓库
git clone https://github.com/lomehong/agent-memory.git
cd agent-memory

# 构建（自动将 frontend/ 复制到 backend/cmd/server/web/，然后编译 Go）
make build

# 启动服务（默认监听 0.0.0.0:8101）
./bin/agent-memory

# 或指定配置文件
./bin/agent-memory -config /path/to/config.yaml
```

### 前端开发

前端是纯静态文件（HTML/CSS/JS），**直接编辑 `frontend/` 目录下的文件**，然后重新 `make build` 即可。前端通过 `//go:embed all:web` 嵌入 Go 二进制。

> ⚠️ **不要直接编辑 `backend/cmd/server/web/`**，该目录是构建时从 `frontend/` 自动复制生成的。

### 配置文件

运行时依赖 `config.yaml`（默认在二进制同级目录查找）。关键配置项：

```yaml
server:
  host: "0.0.0.0"
  port: 8101

# Web Dashboard 管理员登录
web:
  jwt_secret: "${WEB_JWT_SECRET:-am-dashboard-2026-secret}"
  admins:
    - username: "admin"
      password_hash: "admin123"  # 启动时自动 bcrypt 化

# Agent 注册列表
agents:
  - id: m10s
    name: OpenClaw-M10S
    team: default
    api_key: ${M10S_API_KEY:-dev-api-key-001}

# 存储
storage:
  sqlite_path: "./data/memories.db"
  vector:
    provider: "memory"  # memory（内存向量）| qdrant

# Embedding 服务
embedding:
  provider: "mock"  # mock（测试）| onnx（本地）| openai（远程）
  dimensions: 384
```

配置支持 `${ENV_VAR:-default}` 环境变量替换。

---

## 四、生产部署

### 部署架构

当前生产环境部署在 **192.168.2.131** (lome-GEM12)：

```
┌─────────────────────────────────────────┐
│  131 服务器 (Ubuntu)                     │
│                                          │
│  /home/openclaw/agent-memory/            │
│  ├── agent-memory-server    # 运行中的二进制│
│  ├── config.yaml            # 配置文件    │
│  ├── data/memories.db       # SQLite 数据库│
│  └── logs/                  # 日志目录    │
│                                          │
│  /home/openclaw/agent-memory-src/        │
│  └── (源码，编译用)                       │
│                                          │
│  OpenClaw Gateway (端口 3000)             │
│  └── agent-memory-plugin (TypeScript)     │
│     → 连接 http://127.0.0.1:8101         │
└─────────────────────────────────────────┘
```

### 编译部署流程

```bash
# 1. 本地修改代码后同步到 131 源码目录
SSHPASS='IamOpenclaw' sshpass -e rsync -avz \
  --exclude='.git' --exclude='bin' --exclude='*.db' --exclude='qdrant' \
  /tmp/agent-memory/ openclaw@192.168.2.131:/home/openclaw/agent-memory-src/

# 2. SSH 到 131 编译
ssh openclaw@192.168.2.131
export PATH=$PATH:/usr/local/go/bin
cd /home/openclaw/agent-memory-src

# ⚠️ 重要：backend/ 目录包含最新的后端代码，编译前需同步到 cmd/server/
cp backend/cmd/server/main.go cmd/server/main.go
cp backend/internal/api/middleware.go internal/api/middleware.go
cp backend/internal/storage/dal.go internal/storage/dal.go
cp backend/internal/storage/sqlite.go internal/storage/sqlite.go
cp backend/internal/core/retriever.go internal/core/retriever.go

# 构建
go build -o /home/openclaw/agent-memory/agent-memory-server ./cmd/server/

# 3. 重启服务
kill $(pgrep -f agent-memory-server)
cd /home/openclaw/agent-memory && nohup ./agent-memory-server > /tmp/am.log 2>&1 &
```

> ⚠️ **关键注意**：项目中存在两份 `main.go` — `cmd/server/main.go`（旧入口）和 `backend/cmd/server/main.go`（最新代码）。**编译前务必将 `backend/` 下的最新文件复制到对应位置**，否则会编译出旧版本。这是历史遗留问题，未来应统一。

### 一键部署

```bash
make deploy DEPLOY_HOST=openclaw@192.168.2.131 DEPLOY_DIR=/home/openclaw/agent-memory
```

### 服务管理

```bash
# 检查服务状态
curl http://192.168.2.131:8101/api/v1/health

# 查看日志
tail -f /tmp/am.log

# 停止
kill $(pgrep -f agent-memory-server)
```

### Agent 注册与 API Key

所有 Agent 共享同一个 API Key `dev-api-key-001`（开发环境），通过 `X-User-Id` header 隔离记忆空间。生产环境应使用独立 API Key。

当前注册的 Agent（config.yaml 中定义）：

| Agent ID | Name | Team | userId |
|----------|------|------|--------|
| m10s | OpenClaw-M10S | default | m10s |
| devforge | OpenClaw-M10S-dev | default | devforge |
| qbot | OpenClaw-M10S-QA | default | qbot |
| sage | Sage | default | sage |
| clara | Clara | default | clara |

---

## 五、API 架构

### 认证机制

系统支持两种认证方式（可同时使用）：

1. **API Key**：`X-API-Key: <key>` header，用于 Agent/插件调用
2. **JWT Bearer**：`Authorization: Bearer <token>` header，用于 Web Dashboard

认证中间件在 `backend/internal/api/middleware.go` 中实现。通过认证后，Agent 信息存入请求上下文：

```go
type AgentInfo struct {
    ID      string  // Agent ID
    UserID  string  // 用户ID（Agent隔离依据）
    Team    string  // 团队
    IsAdmin bool    // JWT管理员标记
}
```

### 权限模型

- **普通 Agent**：只能读写自己的记忆（`user_id` 匹配），team 记忆只读
- **Admin 用户**（JWT 登录）：可以管理所有 Agent 的记忆，不受权限限制
- **权限检查函数**：`isAdmin(info)` 判断是否为 admin（`info.Team == "admin"`）

### 核心 API 端点

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/v1/health | 健康检查 | 无 |
| POST | /api/v1/auth/login | 管理员登录 | 无 |
| GET | /api/v1/auth/me | 当前用户信息 | JWT |
| POST | /api/v1/memories | 创建记忆 | API Key / JWT |
| GET | /api/v1/memories | 列出记忆 | API Key / JWT |
| GET | /api/v1/memories/search | 语义搜索 | API Key / JWT |
| GET | /api/v1/memories/{id} | 获取单条记忆 | API Key / JWT |
| PUT | /api/v1/memories/{id} | 更新记忆 | API Key / JWT |
| DELETE | /api/v1/memories/{id} | 删除记忆 | API Key / JWT |
| POST | /api/v1/memories/batch | 批量创建 | API Key / JWT |
| POST | /api/v1/memories/compress | 批量压缩 | API Key / JWT |
| GET | /api/v1/memories/report | 健康报告 | API Key / JWT |
| POST | /api/v1/memories/export | 导出记忆 | API Key / JWT |
| POST | /api/v1/agents | 注册 Agent | API Key / JWT |
| GET | /api/v1/agents | 列出 Agent | API Key / JWT |
| GET | /api/v1/agents/{id} | 获取 Agent 信息 | API Key / JWT |
| DELETE | /api/v1/agents/{id} | 删除 Agent | API Key / JWT |
| GET | /api/v1/system/config | 获取系统配置 | API Key / JWT |

### 数据模型

```go
type Memory struct {
    ID           string     `json:"id"`
    UserID       string     `json:"user_id"`       // Agent 隔离主键
    AgentID      string     `json:"agent_id"`      // Agent 标识
    Team         string     `json:"team"`          // 团队
    Visibility   string     `json:"visibility"`    // private/team/user
    Content      string     `json:"content"`       // 记忆内容
    Category     string     `json:"category"`      // identity/principle/knowledge/working
    Priority     int        `json:"priority"`      // 1-5（1=最高）
    Confidence   float64    `json:"confidence"`    // 0.0-1.0
    TTL          string     `json:"ttl"`           // permanent/year/month/week/session
    Tags         []string   `json:"tags"`          // 标签
    Version      int        `json:"version"`       // 乐观锁版本号
    Status       string     `json:"status"`        // active/degraded/archived/deleted
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
    LastAccessed time.Time  `json:"last_accessed"`
    AccessCount  int        `json:"access_count"`
    MergedFrom   []string   `json:"merged_from"`   // 被合并的源记忆 ID 列表
}
```

---

## 六、OpenClaw 插件

插件代码位于独立仓库 [agent-memory-plugin](https://github.com/lomehong/agent-memory-plugin)，部署在 OpenClaw 的 `~/.openclaw/extensions/agent-memory-plugin/` 目录。

### 插件功能

| 功能 | 说明 |
|------|------|
| **autoRecall** | 每轮对话前，根据当前对话上下文语义搜索相关记忆，注入到 system prompt |
| **autoCapture** | 每轮对话后，自动分析对话内容，提取有价值信息存入记忆 |
| **工具集** | 提供 memory_store / memory_search / memory_list / memory_get / memory_forget / memory_report 六个工具给 Agent 调用 |

### autoCapture 三层过滤架构

```
对话内容 → [噪声过滤层] → [内容提取层] → [质量门控层] → 存入记忆
```

1. **噪声过滤层**：过滤飞书元数据、系统提示、JSON 代码块等 20+ 种噪声模式
2. **内容提取层**：清洗并提取有价值的文本片段
3. **质量门控层**：基于正则启发式判断内容价值（IP/URL/配置/决策/偏好等 15+ 种信号）

### 插件配置（OpenClaw config.yaml）

```yaml
plugins:
  agent-memory-plugin:
    enabled: true
    config:
      host: "http://127.0.0.1:8101"
      apiKey: "dev-api-key-001"
      userId: "m10s"          # 当前 Agent 的 userId
      autoRecall: true
      autoCapture: true
      topK: 5
```

> ⚠️ **注意**：插件 `configSchema` 中的 `host` 和 `apiKey` 不要设为 `required`，应提供 `default` 值。因为新版 OpenClaw 有 chokidar 配置热重载机制，在某些重载路径中 config 可能为空对象，如果 AJV 验证失败会产生大量错误日志循环。参见 2026-04-11 修复记录。

---

## 七、关键设计决策与踩坑记录

### 7.1 Admin 权限问题

**问题**：JWT admin 用户登录后，`info.UserID` 为空字符串，导致所有按 `userID` 过滤的查询返回空结果。

**影响范围**：
- `handleGetMemory` — admin 无法查看单条记忆（404）
- `handleUpdateMemory` / `handleDeleteMemory` — admin 无法编辑/删除记忆（403）
- `handleListMemories` — admin 列表为空

**解决方案**：
- 添加 `isAdmin(info)` 函数（`info.Team == "admin"`）
- Admin 用户直接使用 `dal.GetMemory(ctx, id)` 按 ID 查询，跳过 userID 过滤
- 为 `Retriever` 添加 `DAL()` getter 方法暴露底层存储层

### 7.2 双 main.go 问题

**问题**：项目中存在两份 `main.go`：
- `cmd/server/main.go` — 旧版本，缺少 admin 权限修复
- `backend/cmd/server/main.go` — 最新版本

编译命令 `go build ./cmd/server/` 使用的是前者。

**解决方案**：编译前将 `backend/` 下的文件复制到对应位置。**未来应清理掉 `cmd/server/main.go`，统一使用 `backend/cmd/server/main.go`**。

### 7.3 OpenClaw 热重载与 configSchema

**问题**：新版 OpenClaw (2026.4.10+) 的插件框架使用 chokidar 监控配置文件变化，触发热重载。如果 `configSchema` 中有 `required` 字段，在某些重载路径中 config 为空对象 `{}`，导致 AJV 验证失败并产生每秒 5 次的错误日志循环。

**解决方案**：
1. `configSchema` 去掉 `required`，添加 `default` 值
2. `parseConfig` 不再抛异常，改为优雅降级（禁用 autoRecall/autoCapture）

### 7.4 前端嵌入机制

前端资源通过 Go `//go:embed all:web` 嵌入二进制。`Makefile build` 时自动执行 `cp -r frontend/* backend/cmd/server/web/`。

- **编辑**：修改 `frontend/` 目录
- **勿编辑**：`backend/cmd/server/web/`（构建产物）

### 7.5 SQLite 无 CGO

使用 `modernc.org/sqlite` 纯 Go 实现，无需安装 SQLite C 库。二进制完全静态编译（`CGO_ENABLED=0`）。

---

## 八、常见开发任务

### 添加新的 API 端点

1. 在 `backend/internal/api/` 或 `backend/cmd/server/main.go` 中添加 handler 函数
2. 在 `main()` 的路由注册块中注册路由
3. 如需新数据模型，在 `backend/internal/model/` 中定义
4. 如需数据库操作，在 `backend/internal/storage/dal.go` 添加接口，在 `sqlite.go` 实现

### 修改前端页面

1. 编辑 `frontend/js/` 对应的 JS 文件
2. 如需新样式，编辑 `frontend/css/style.css`
3. 重新 `make build` 并部署

### 添加新的 Embedding 提供商

1. 在 `backend/internal/embedding/` 新建文件实现 `EmbeddingProvider` 接口
2. 在 `config.go` 中添加对应配置项
3. 在 `main.go` 的初始化逻辑中添加 provider 创建分支

### 修改评分算法

编辑 `backend/pkg/scoring/scorer.go`，权重配置在 `config.yaml` 的 `search.scoring` 部分。

### 添加新的记忆分类

1. 在 `backend/internal/model/memory.go` 中添加常量
2. 更新 `ValidCategories` map
3. 在 `backend/internal/core/classifier.go` 中添加分类规则
4. 更新前端 `frontend/js/memories.js` 的筛选下拉框

---

## 九、测试

```bash
# 运行所有测试
cd backend && go test ./...

# 运行特定包测试
cd backend && go test ./internal/core/...
cd backend && go test ./pkg/scoring/...
```

### 手动 API 测试

```bash
# 健康检查
curl http://localhost:8101/api/v1/health

# API Key 认证测试
curl -H "X-API-Key: dev-api-key-001" http://localhost:8101/api/v1/memories

# JWT 登录测试
TOKEN=$(curl -s -X POST http://localhost:8101/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")

# JWT 认证测试
curl -H "Authorization: Bearer $TOKEN" http://localhost:8101/api/v1/memories

# 语义搜索测试
curl -H "X-API-Key: dev-api-key-001" \
  "http://localhost:8101/api/v1/memories/search?query=test&top_k=5"
```

---

## 十、监控与运维

### 日志

服务日志输出到 stdout（容器环境）或 `/tmp/am.log`（直接运行）。日志级别通过 `config.yaml` 的 `logging.level` 配置。

### 健康检查

```bash
curl http://localhost:8101/api/v1/health
# {"status":"ok"}
```

### 记忆健康报告

```bash
curl -H "X-API-Key: dev-api-key-001" http://localhost:8101/api/v1/memories/report
```

返回各类别/状态的记忆数量统计、零访问记忆列表、过期记忆列表等。

### 批量压缩

```bash
curl -X POST -H "X-API-Key: dev-api-key-001" \
  http://localhost:8101/api/v1/memories/compress
```

自动合并相似记忆（相似度 > 0.85）和归档过期记忆。

### 数据库备份

SQLite 数据库文件位于 `config.yaml` 中 `storage.sqlite_path` 指定的路径（默认 `./data/memories.db`）。

```bash
# 热备份（SQLite 支持读时复制）
cp /home/openclaw/agent-memory/data/memories.db /backup/memories-$(date +%Y%m%d).db
```

---

## 十一、代码规范

### Go 后端

- 遵循标准 Go 项目布局（`cmd/`、`internal/`、`pkg/`）
- 使用 `zerolog` 结构化日志
- HTTP 路由使用 `chi` 框架
- 错误处理：handler 层返回 JSON 错误，业务层返回 Go error
- 数据库操作通过 DAL 接口抽象，方便测试和替换

### 前端

- 纯 Vanilla JS，无框架依赖
- 全局 `api` 对象封装所有 API 调用
- 全局工具函数在 `app.js` 中定义（`escHtml`、`formatDate`、`catTag`、`openModal` 等）
- CSS 变量定义在 `:root` 中，深色主题
- 弹窗通过 `openModal(title, bodyHtml, width)` 创建，支持自定义宽度

### Git 提交

- 提交信息使用中文
- 格式：`type: 简要描述`，type 包括 `feat`、`fix`、`docs`、`refactor`、`chore`
- 示例：`feat: 前端增加 Agent 维度记忆管理功能`
- 示例：`fix: admin JWT 用户可以编辑/删除所有记忆`

---

## 十二、待改进事项

- [ ] **统一 main.go**：消除 `cmd/server/` 和 `backend/cmd/server/` 的双份源码问题，统一为单一入口
- [ ] **前端构建优化**：考虑引入轻量打包工具（如 Vite），支持模块化和热更新
- [ ] **API 分页优化**：当前列表接口先查全部再分页，数据量大时性能差，应改为 SQL LIMIT/OFFSET
- [ ] **配置热重载**：后端自身支持 config.yaml 热重载（无需重启）
- [ ] **Agent 记忆数统计**：当前前端通过 report API 粗略统计，应增加专门的 count 接口
- [ ] **单元测试覆盖**：核心业务逻辑（分类器、评分器、去重）需要补充测试
- [ ] **API 文档**：考虑集成 Swagger/OpenAPI 自动生成文档
- [ ] **日志轮转**：生产环境应配置日志轮转，避免日志文件无限增长
