# AI Agent 通用记忆框架 (agent-memory)

一个为AI Agent设计的结构化记忆管理框架，支持多用户/多Agent隔离、语义检索、自动去重、TTL过期等能力。

## 特性

- **多级记忆分类**：identity / principle / knowledge / working
- **多Agent隔离**：private / team / user 三级可见性
- **语义去重**：自动检测相似记忆并合并
- **TTL过期**：自动降级→归档→清理
- **多维排序检索**：相似度+优先级+热度+分类+紧迫度
- **批量治理**：压缩、健康报告、导出
- **Mem0迁移**：从现有Mem0无缝导入

## 技术栈

- **后端**：Go 1.23
- **元数据存储**：SQLite (modernc.org/sqlite)
- **向量数据库**：Qdrant
- **Embedding**：ONNX Runtime (sentence-transformers) / Mock

## 快速开始

### 本地开发

```bash
# 安装依赖
make build

# 启动服务（需先启动Qdrant）
make run
```

### Docker部署

```bash
# 启动全部服务（agent-memory + qdrant）
make docker-up

# 查看日志
make docker-logs
```

## API文档

启动后访问 http://localhost:8100/api/v1/health 检查服务状态。

### 核心接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/memories | 创建记忆 |
| GET | /api/v1/memories/search | 语义检索 |
| GET | /api/v1/memories | 列出记忆 |
| PUT | /api/v1/memories/{id} | 更新记忆 |
| DELETE | /api/v1/memories/{id} | 删除记忆 |
| POST | /api/v1/memories/compress | 批量压缩 |
| GET | /api/v1/memories/report | 健康报告 |
| POST | /api/v1/agents | 注册Agent |
| GET | /api/v1/health | 健康检查 |

### 认证

所有请求需携带 `X-API-Key` header，框架自动识别Agent身份并执行权限控制。

## 项目结构

```
├── cmd/server/       # 服务入口
├── cmd/migrate/      # Mem0迁移工具
├── internal/
│   ├── api/          # HTTP接口
│   ├── config/       # 配置管理
│   ├── core/         # 业务逻辑
│   ├── embedding/    # Embedding服务
│   ├── model/        # 数据模型
│   ├── plugin/       # OpenClaw插件
│   └── storage/      # 数据存储
├── pkg/scoring/      # 评分算法
├── docs/             # 需求和设计文档
├── config.yaml
├── Dockerfile
├── docker-compose.yml
└── Makefile
```
