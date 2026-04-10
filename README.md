# AI Agent 通用记忆框架 (agent-memory)

一个为AI Agent设计的结构化记忆管理框架，支持多用户/多Agent隔离、语义检索、自动去重、TTL过期等能力。

## 特性

- **多级记忆分类**：identity / principle / knowledge / working
- **多Agent隔离**：private / team / user 三级可见性
- **按Agent隔离记忆空间**：通过 `X-User-Id` header 实现每个 Agent 独立记忆
- **语义去重**：自动检测相似记忆并合并
- **TTL过期**：自动降级→归档→清理
- **多维排序检索**：相似度+优先级+热度+分类+紧迫度
- **批量治理**：压缩、健康报告、导出
- **Mem0迁移**：从现有Mem0无缝导入
- **JWT认证**：支持 Web Dashboard 登录
- **Web Dashboard**：内嵌式管理界面（统计/记忆/Agent/系统配置）
- **OpenAI Embedding**：支持 OpenAI 兼容 API（vLLM/Ollama 等）
- **Mem0迁移**：从现有Mem0无缝导入

## 技术栈

- **后端**：Go 1.25
- **元数据存储**：SQLite (modernc.org/sqlite)
- **向量数据库**：Qdrant
- **Embedding**：ONNX Runtime / OpenAI 兼容 API / Mock
- **前端**：纯 HTML + CSS + JS（Chart.js），Go embed 嵌入

## 项目结构

```
agent-memory/
├── frontend/              # 前端源码（独立，可直接编辑）
│   ├── index.html
│   ├── css/style.css
│   └── js/
│       ├── api.js         # API 封装
│       ├── app.js         # 路由、登录、工具函数
│       ├── dashboard.js   # 统计仪表盘
│       ├── memories.js    # 记忆管理
│       ├── agents.js      # Agent 管理
│       └── system.js      # 系统配置
├── backend/               # Go 后端
│   ├── cmd/server/        # 服务入口（Go embed 嵌入前端）
│   ├── cmd/migrate/       # Mem0 迁移工具
│   ├── internal/
│   │   ├── api/           # HTTP 接口 + 认证中间件
│   │   ├── auth/          # JWT 认证
│   │   ├── config/        # 配置管理
│   │   ├── core/          # 业务逻辑（分类/去重/检索/TTL/压缩）
│   │   ├── embedding/     # Embedding 服务（ONNX/OpenAI/Mock）
│   │   ├── model/         # 数据模型
│   │   ├── plugin/        # OpenClaw 工具 Schema 定义
│   │   └── storage/       # SQLite + Qdrant 数据存储
│   ├── pkg/scoring/       # 多维评分算法
│   ├── go.mod
│   └── go.sum
├── docs/                  # 需求和设计文档
├── config.yaml            # 配置文件
├── Dockerfile             # 多阶段构建
├── docker-compose.yml     # Docker 编排
├── Makefile               # 构建/部署命令
└── README.md
```

## 快速开始

### 本地开发

```bash
# 构建（自动复制 frontend/ → backend/cmd/server/web/）
make build

# 启动服务（需先启动 Qdrant）
make run
```

### Docker 部署

```bash
# 启动全部服务（agent-memory + qdrant）
make docker-up

# 查看日志
make docker-logs
```

### 远程部署

```bash
# 一键部署到远程服务器
make deploy DEPLOY_HOST=openclaw@192.168.2.131
```

## API 文档

启动后访问 `http://localhost:8101/api/v1/health` 检查服务状态。

### 核心接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/memories | 创建记忆 |
| GET | /api/v1/memories/search | 语义检索 |
| GET | /api/v1/memories | 列出记忆 |
| GET | /api/v1/memories/{id} | 获取单条记忆 |
| PUT | /api/v1/memories/{id} | 更新记忆 |
| DELETE | /api/v1/memories/{id} | 删除记忆 |
| POST | /api/v1/memories/compress | 批量压缩 |
| GET | /api/v1/memories/report | 健康报告 |
| GET | /api/v1/system/config | 系统配置 |
| POST | /api/v1/agents | 注册 Agent |
| POST | /api/v1/auth/login | 登录 |
| POST | /api/v1/auth/logout | 登出 |
| GET | /api/v1/auth/me | 当前用户信息 |
| GET | /api/v1/health | 健康检查 |

### 认证

**API Key 认证**：所有请求需携带 `X-API-Key` header，框架自动识别 Agent 身份。

**X-User-Id 覆盖**：支持 `X-User-Id` header 覆盖 userId，用于多 Agent 记忆隔离。OpenClaw 插件自动根据当前 Agent 设置此 header。

**JWT 认证**：Web Dashboard 使用 Bearer Token 认证。

### Agent 隔离

每个 OpenClaw Agent 自动映射到独立的 userId：

| Agent | ID | userId |
|-------|-----|--------|
| M10S | main | m10s |
| DevForge | dev | devforge |
| Sage | researcher | sage |
| Clara | secretary | clara |
| QBot | tester | qbot |

## 配套 OpenClaw 插件

插件仓库：[agent-memory-plugin](https://github.com/lomehong/agent-memory-plugin)

### 插件特性

- **autoRecall**：每轮对话前自动注入相关记忆到上下文
- **autoCapture**：每轮对话后智能提取有价值内容存入记忆
  - 三层架构：过滤层（噪声检测）→ 提取层（内容清洗）→ 质量门控（价值判断）
  - 自动过滤飞书元数据、heartbeat 系统提示、JSON 代码块等 20+ 种噪声模式
  - 基于正则启发式判断内容价值（IP/URL/配置/决策/偏好等 15+ 种信号）
  - 批内去重，避免重复存储
- **Agent 隔离**：通过 `before_tool_call` hook 追踪当前 Agent，自动设置 `X-User-Id`
- **工具集**：memory_store / memory_search / memory_list / memory_get / memory_forget / memory_report

### 插件配置

```json
{
  "agent-memory-plugin": {
    "enabled": true,
    "config": {
      "host": "http://192.168.2.131:8101",
      "apiKey": "dev-api-key-001",
      "userId": "default",
      "autoRecall": true,
      "autoCapture": true,
      "topK": 5
    }
  }
}
```

## License

MIT
