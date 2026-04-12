# AI Agent 通用记忆框架 - 设计文档

> 项目代号：agent-memory  
> 需求文档：docs/requirements.md  
> 创建日期：2026-04-09  
> 文档版本：v1.2  
> 更新日期：2026-04-12  
> 更新内容：修复 Agent 身份隔离架构缺陷（DESIGN-020~DESIGN-023），消除 API Key 共享导致的多 Agent 注册失败、X-User-Id 覆盖不完整、Plugin 硬编码映射三大 Bug

---

## 1. 架构概览

```
┌─────────────────────────────────────────────────┐
│                   Agent 调用方                     │
│     M10S / DevForge / QBot / Sage / Clara          │
│         (OpenClaw Plugin, X-User-Id 隔离)           │
└──────────┬──────────────────────┬────────────────┘
           │                      │
    RESTful API              OpenClaw Plugin
    (X-API-Key + X-User-Id)  (自动映射 Agent→userId)
           │                      │
┌──────────┴──────────────────────┴────────────────┐
│                  API Gateway                      │
│     (认证 / Agent身份识别(X-User-Id覆盖) / 限流)    │
│                                                  │
│  ┌────────────────────────────────────────────┐  │
│  │         可见性权限控制器                       │  │
│  │  private: 仅创建者Agent                       │  │
│  │  team:    同team所有Agent可读                  │  │
│  │  user:    同user所有Agent可读写                │  │
│  └────────────────────────────────────────────┘  │
├─────────────────────────────────────────────────┤
│                                                   │
│  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │ 写入模块  │  │ 检索模块  │  │  治理模块      │  │
│  │          │  │          │  │               │  │
│  │·去重评估 │  │·语义检索 │  │·批量压缩       │  │
│  │·门控评估 │  │·多维排序 │  │·TTL过期       │  │
│  │·元数据   │  │·分类过滤 │  │·健康报告       │  │
│  │·embedding│  │·分页     │  │·归档/清理     │  │
│  └────┬─────┘  └────┬─────┘  └──────┬────────┘  │
│       │              │               │           │
│  ┌────┴──────────────┴───────────────┴────────┐  │
│  │              数据访问层 (DAL)                 │  │
│  │                                             │  │
│  │  ┌──────────┐    ┌──────────────────────┐  │  │
│  │  │ 元数据存储 │    │   向量存储            │  │  │
│  │  │ (SQLite)  │    │   (Qdrant)           │  │  │
│  │  └──────────┘    └──────────────────────┘  │  │
│  └─────────────────────────────────────────────┘  │
│                                                   │
│  ┌─────────────────────────────────────────────┐  │
│  │              Embedding 服务                   │  │
│  │         (本地 sentence-transformers)          │  │
│  └─────────────────────────────────────────────┘  │
│                                                   │
└─────────────────────────────────────────────────┘
```

**DESIGN-001** 整体架构采用分层设计  
对应 REQ-023（RESTful API）、REQ-024（OpenClaw插件接口）、NFR-007（独立部署）

系统分为四层：调用接口层（API Gateway）、业务逻辑层（写入/检索/治理模块）、数据访问层（DAL）、存储层（SQLite + Qdrant + Embedding服务）。各层通过明确接口解耦。

**DESIGN-020** 身份隔离架构（v1.2 新增）

身份隔离由三层协同保障，职责明确分离：

| 层 | 机制 | 职责 |
|----|------|------|
| **Plugin 层** | `config.userId` | 每个部署实例在 openclaw.json 中配置独立的 `userId`，作为 `X-User-Id` Header 发送给后端。**禁止在代码中硬编码 agentId→userId 映射表**，因为 OpenClaw 实例的 agentId 由运行时决定，无法预知。 |
| **API Gateway 层** | `X-API-Key` + `X-User-Id` | API Key 仅用于**权限验证**（判断调用方是否有资格使用记忆接口），不承担身份区分职责。`X-User-Id` Header 是**唯一的身份标识来源**，用于隔离不同 Agent/用户的记忆数据。 |
| **数据层** | `user_id` + `agent_id` + `visibility` | 记忆数据通过 `user_id` 做一级隔离，`agent_id` 做二级隔离，`visibility` 做三级访问控制（private/team/user）。 |

```
身份解析流程（DESIGN-020）：

  请求到达
     │
     ▼
  ┌─────────────────────────────┐
  │ 1. X-API-Key 验证            │  → 无效则 401（权限拒绝）
  │    不用于身份区分              │
  └──────────┬──────────────────┘
             ▼
  ┌─────────────────────────────┐
  │ 2. X-User-Id Header 读取     │  → 这是唯一身份来源
  │    由 Plugin 从 config 注入    │
  └──────────┬──────────────────┘
             ▼
  ┌─────────────────────────────┐
  │ 3. agents 表查找              │
  │    GetAgentByUserID(X-User-Id) │
  │    ├─ 找到 → 使用其 ID/Team    │
  │    └─ 未找到 → 以 X-User-Id    │
  │       作为 userID + agentID   │
  └──────────┬──────────────────┘
             ▼
       后续请求均使用
       {userID, agentID, team}
       进行数据隔离
```

**核心原则：**
- API Key = 权限验证（能不能用），X-User-Id = 身份标识（你是谁）
- 多个 Agent 可以共享同一个 API Key（如开发环境），但必须有不同的 userId
- Plugin 端的身份来源**唯一**是 openclaw.json 中的 `config.userId`，不依赖任何硬编码映射

---

## 2. 数据模型设计

### 2.1 记忆数据模型

**DESIGN-002** 记忆元数据存储模型  
对应 REQ-003（记忆元数据模型）、REQ-004（元数据自动填充）、REQ-020（Agent身份识别）

```go
// SQLite 表结构
type Memory struct {
    ID          string    `json:"id"`           // UUID，主键
    UserID      string    `json:"user_id"`      // 用户隔离标识
    AgentID     string    `json:"agent_id"`     // 所属Agent标识
    Team        string    `json:"team"`         // 所属团队（默认"default"）
    Visibility  string    `json:"visibility"`   // private/team/user
    Content     string    `json:"content"`      // 记忆正文
    Category    string    `json:"category"`     // identity/principle/knowledge/working
    Priority    int       `json:"priority"`     // 1-5，1最高
    Source      string    `json:"source"`       // 来源描述
    Confidence  float64   `json:"confidence"`   // 0.0-1.0
    TTL         string    `json:"ttl"`          // permanent/year/month/week/session
    Tags        []string  `json:"tags"`         // JSON数组存储
    Version     int       `json:"version"`      // 版本号
    Status      string    `json:"status"`       // active/degraded/archived/deleted
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
    LastAccessed time.Time `json:"last_accessed"`
    AccessCount int       `json:"access_count"`
    MergedFrom  []string  `json:"merged_from"`  // 被合并的原记忆ID列表
}
```

**DESIGN-003** 向量存储模型  
对应 REQ-011（语义检索）、NFR-006（向量一致性）

```go
// Qdrant Collection: memories_{user_id}
type MemoryVector struct {
    ID      string            `json:"id"`
    Vector  []float32         `json:"vector"`  // embedding向量
    Payload map[string]any    `json:"payload"` // 包含category、priority、tags、agent_id、team、visibility等
}
```

**DESIGN-003-A** Agent注册表  
对应 REQ-020（Agent身份识别）、REQ-022（Agent注册与管理）

```go
type Agent struct {
    ID        string    `json:"id"`        // Agent唯一标识
    Name      string    `json:"name"`      // 显示名称
    UserID    string    `json:"user_id"`   // 所属用户（身份隔离关键字段）
    Team      string    `json:"team"`      // 所属团队
    APIKeyHash string   `json:"-"`        // 认证密钥哈希（不暴露）
    CreatedAt time.Time `json:"created_at"`
}
```

**DESIGN-021** Agent 配置结构（v1.2 新增）

config.yaml 中的 agents 配置必须包含 `user_id` 字段，用于身份隔离：

```yaml
agents:
  - id: m10s
    name: OpenClaw-M10S
    user_id: m10s           # 必填：身份隔离标识
    team: default
    api_key: ${M10S_API_KEY}
  - id: gem12
    name: OpenClaw-GEM12-MAX-Master
    user_id: gem12          # 必填：身份隔离标识
    team: default
    api_key: ${GEM12_API_KEY}  # 可以与其他 Agent 共享
```

**设计约束：**
- `api_key_hash` 不再有 UNIQUE 约束，允许多个 Agent 共享同一个 API Key
- 每个 Agent 必须有独立的 `user_id`，这是记忆隔离的唯一依据
- `id` 和 `user_id` 可以相同（推荐），但语义不同：`id` 是 Agent 内部标识，`user_id` 是跨系统身份标识

向量与元数据通过id关联。更新记忆时，同步更新SQLite和Qdrant，保证一致性（NFR-005原子性、NFR-006向量一致性）。

### 2.2 记忆操作日志

**DESIGN-004** 操作审计日志  
对应 NFR-012（日志）

```go
type MemoryLog struct {
    ID        string    `json:"id"`
    MemoryID  string    `json:"memory_id"`
    UserID    string    `json:"user_id"`
    Action    string    `json:"action"`    // create/update/merge/degrade/archive/delete
    Detail    string    `json:"detail"`    // 操作详情（JSON）
    CreatedAt time.Time `json:"created_at"`
}
```

---

## 3. 核心模块设计

### 3.1 写入模块

**DESIGN-005** 写入流程  
对应 REQ-005（语义去重）、REQ-007（去重结果通知）、REQ-018（写入评估）、REQ-019（写入建议返回）、REQ-021（记忆可见性规则）

```
写入请求
    │
    ▼
┌─────────────────┐
│ 1. 参数解析      │  提取content、category（可选）、priority（可选）等
└────────┬────────┘
         ▼
┌─────────────────┐
│ 2. 分类推断      │  若未指定category，调用分类器推断
└────────┬────────┘
         ▼
┌─────────────────┐
│ 3. 语义去重      │  对content生成embedding，在Qdrant中检索相似记忆
│                  │  阈值由DESIGN-006定义
└────────┬────────┘
         ▼
    ┌────┴────┐
    │命中?    │
    ├─Yes─────┤
    │         │
    ▼         ▼
┌────────┐ ┌────────┐
│ 4a.合并 │ │ 4b.新增 │
│ 更新原  │ │ 创建新  │
│ 记忆    │ │ 记忆    │
└───┬────┘ └───┬────┘
    │          │
    ▼          ▼
┌─────────────────┐
│ 5. 写入评估      │  评估并建议ttl、priority
└────────┬────────┘
         ▼
┌─────────────────┐
│ 6. 持久化        │  SQLite + Qdrant 原子写入
└────────┬────────┘
         ▼
    返回结果
```

**DESIGN-006** 去重阈值配置  
对应 REQ-006（去重阈值可配置）

```yaml
# config.yaml
dedup:
  thresholds:
    identity: 0.95
    principle: 0.90
    knowledge: 0.85
    working: 0.70
  embedding_model: all-MiniLM-L6-v2  # 默认本地模型
```

合并策略：当命中去重时，保留priority更低（更优先）的那条，将另一条的内容差异追加到merged_from中，confidence取两者最大值。

**DESIGN-007** 分类推断  
对应 REQ-002（记忆分类标签）

分类推断使用基于规则+关键词的轻量分类器（不依赖LLM，保证速度）：

```go
func InferCategory(content string) string {
    // 基于关键词和规则推断记忆分类
    rules := map[string][]string{
        "identity":  {"名字", "userId", "时区", "邮箱", "GitHub账号", "地域", "职业"},
        "principle":  {"优先", "原则", "必须", "要求", "认为", "偏好", "习惯"},
        "working":   {"完成", "修复", "部署", "测试", "进度", "任务", "当前"},
    }
    // 按优先级匹配：identity > principle > knowledge > working
    // 匹配到最高优先级即返回
    // 全部未匹配返回knowledge（默认）
}
```

若规则无法确定，默认为knowledge。

---

### 3.2 检索模块

**DESIGN-008** 语义检索流程  
对应 REQ-011（语义检索）、REQ-012（多维度排序）、REQ-013（分类过滤）、REQ-014（分页检索）、REQ-021（记忆可见性规则）

```
检索请求(query, category?, page_size?, page_token?)
    │
    ▼
┌─────────────────┐
│ 0. 权限过滤      │  根据当前Agent身份，确定可见范围：
│                  │  WHERE user_id = ? AND (
│                  │    agent_id = ?                    -- private
│                  │    OR (visibility = 'team'
│                  │        AND team = ?)             -- team
│                  │    OR visibility = 'user'        -- user
│                  │  )
└────────┬────────┘
         ▼
┌─────────────────┐
│ 1. Embedding     │  将query转为向量
└────────┬────────┘
         ▼
┌─────────────────┐
│ 2. 向量检索      │  Qdrant中搜索，应用权限过滤条件
│                  │  若指定category，追加filter
└────────┬────────┘
         ▼
┌─────────────────┐
│ 3. 多维排序      │  综合打分（同原DESIGN-009）
└────────┬────────┘
         ▼
┌─────────────────┐
│ 4. 过滤与分页    │  排除archived/deleted状态
└────────┬────────┘
         ▼
    返回检索结果
```

**DESIGN-009** 多维度排序权重  
对应 REQ-012（多维度排序）

```yaml
# config.yaml
search:
  scoring:
    similarity_weight: 0.40
    priority_weight: 0.25
    access_count_weight: 0.15
    category_weight: 0.10
    urgency_weight: 0.10
  category_weights:
    identity: 1.0
    principle: 0.8
    knowledge: 0.6
    working: 0.4
  urgency:
    threshold_days: 7      # 距过期不足7天的working记忆提升
    boost: 0.2             # 紧迫度加成上限
  default_page_size: 10
  max_page_size: 50
```

---

### 3.3 治理模块

**DESIGN-010** TTL过期机制  
对应 REQ-008（TTL自动过期）、REQ-009（过期处理策略）、REQ-010（热度重置TTL）

```go
type TTLPolicy struct{}

var TTLDuration = map[string]time.Duration{
    "session":   24 * time.Hour,
    "week":      7 * 24 * time.Hour,
    "month":     30 * 24 * time.Hour,
    "year":      365 * 24 * time.Hour,
    "permanent": 0, // 永不过期
}

const (
    DegradeMultiplier = 2 // 降级 = TTL * 2
    ArchiveMultiplier = 3 // 归档 = TTL * 3
)
```

治理守护进程（后台goroutine）每6小时执行一次TTL扫描：

```go
func (s *Service) TTLScan(ctx context.Context) {
    memories := s.store.ListAll(ctx, StatusActive)
    for _, m := range memories {
        if m.TTL == "permanent" {
            continue
        }
        duration := TTLDuration[m.TTL]
        idle := time.Since(m.LastAccessed)
        if idle > duration*time.Duration(ArchiveMultiplier) {
            m.Status = "archived"
        } else if idle > duration*time.Duration(DegradeMultiplier) {
            m.Status = "degraded"
        }
    }
}
```

**DESIGN-011** 批量压缩算法  
对应 REQ-015（批量压缩）

```
批量压缩流程：
1. 获取用户所有active状态记忆
2. 按category分组
3. 每组内两两计算语义相似度
4. 将相似度>阈值的记忆聚为一组（聚类）
5. 每组保留priority最低（最优）的一条作为主记忆
6. 其余记忆的内容差异合并到主记忆的merged_from
7. 删除被合并的记忆
8. 返回压缩报告（删除数量、合并详情）
```

**DESIGN-012** 健康报告  
对应 REQ-016（自动治理报告）

```go
type HealthReport struct {
    TotalCount            int              `json:"total_count"`
    ByCategory            map[string]int   `json:"by_category"`
    ByStatus              map[string]int   `json:"by_status"`
    TopAccessed           []Memory         `json:"top_accessed"`
    ZeroAccess            []Memory         `json:"zero_access"`
    StaleMemories         []Memory         `json:"stale_memories"`
    ChangesSinceLastReport map[string]int  `json:"changes_since_last_report"`
}
```

---

### 3.4 API设计

**DESIGN-013** RESTful API接口规范  
对应 REQ-023（RESTful API）、REQ-025（多用户隔离）、REQ-020（Agent身份识别）

```
Base URL: http://localhost:8100/api/v1

认证: Header X-API-Key: {key}

框架根据API Key自动识别Agent身份（agent_id、user_id、team），
无需在请求中显式传递这些字段。

# 记忆操作
POST   /memories              创建记忆
GET    /memories/search       语义检索 (?query=&category=&page_size=&page_token=)
GET    /memories              列出记忆 (?category=&status=&visibility=&page_size=&page_token=)
GET    /memories/{id}         获取单条
PUT    /memories/{id}         更新记忆
DELETE /memories/{id}         删除记忆

# 批量操作
POST   /memories/batch        批量写入/更新/删除
POST   /memories/compress     批量压缩
GET    /memories/report       健康报告
POST   /memories/export       导出全量数据

# Agent管理
POST   /agents                注册Agent
GET    /agents                列出Agent
GET    /agents/{id}           获取Agent信息
DELETE /agents/{id}           注销Agent

# 系统
GET    /health                健康检查
GET    /metrics               监控指标 (Prometheus格式)
```

请求/响应示例：

```json
// POST /memories
// Request:
{
  "content": "可靠性优先于自动化",
  "category": "principle",     // 可选，未指定则自动推断
  "priority": 1,               // 可选，默认3
  "ttl": "permanent",          // 可选，默认month
  "source": "对话 2026-04-09", // 可选
  "tags": ["工程原则"]         // 可选
}

// Response:
{
  "id": "mem_abc123",
  "content": "可靠性优先于自动化",
  "category": "principle",
  "priority": 1,
  "ttl": "permanent",
  "confidence": 1.0,
  "tags": ["工程原则"],
  "status": "active",
  "suggestion": {              // 写入建议（DESIGN-005步骤5）
    "recommended_ttl": "permanent",
    "recommended_priority": 1,
    "dedup_hit": false
  },
  "created_at": "2026-04-09T17:30:00Z"
}
```

**DESIGN-014** OpenClaw插件接口  
对应 REQ-024（OpenClaw插件接口）、REQ-020（Agent身份识别）

实现OpenClaw的tool protocol，提供以下工具：

```yaml
tools:
  - name: memory_store
    description: "存储一条长期记忆，自动去重和分类"
    parameters:
      content: {type: string, required: true}
      category: {type: string, enum: [identity,principle,knowledge,working]}
      priority: {type: integer, minimum: 1, maximum: 5}
      tags: {type: array, items: {type: string}}

  - name: memory_search
    description: "语义检索记忆"
    parameters:
      query: {type: string, required: true}
      category: {type: string}
      limit: {type: integer, default: 10}

  - name: memory_list
    description: "列出记忆（支持分类过滤）"
    parameters:
      category: {type: string}
      status: {type: string, enum: [active,degraded,archived]}
      limit: {type: integer, default: 20}

  - name: memory_forget
    description: "删除一条记忆"
    parameters:
      memory_id: {type: string, required: true}

  - name: memory_report
    description: "获取记忆健康报告"
    parameters: {}
```

**DESIGN-022** Plugin 身份注入机制（v1.2 新增）

Plugin 的身份来源**唯一**是 openclaw.json 中的 `config.userId`，通过 `X-User-Id` Header 发送给后端。

```jsonc
// openclaw.json 中每个 OpenClaw 实例的配置
"plugins": {
  "entries": {
    "agent-memory-plugin": {
      "enabled": true,
      "config": {
        "host": "http://192.168.2.131:8101",
        "apiKey": "dev-api-key-001",     // 权限验证
        "userId": "gem12",                // 身份标识（每个实例必须不同！）
        "autoRecall": true,
        "autoCapture": true,
        "topK": 5
      }
    }
  }
}
```

**禁止项：**
- ❌ 禁止在 Plugin 代码中硬编码 `agentId → userId` 映射表（如 `{main: "m10s", dev: "devforge"}`），因为 OpenClaw 实例的 agentId 由运行时决定，无法预知，硬编码会导致跨实例身份混淆
- ❌ 禁止依赖 agentId 推断 userId，因为不同机器上的 agentId 可能相同（如都是 "main"）

**正确做法：**
- ✅ Plugin 每次请求都使用 `config.userId` 作为 `X-User-Id` Header
- ✅ 每个 OpenClaw 实例在部署时配置唯一的 `userId`

---

### 3.5 数据迁移

**DESIGN-015** Mem0数据迁移流程  
对应 REQ-027（Mem0数据导入）

```
迁移流程：
1. 通过Mem0 API导出目标用户的全量记忆
2. 逐条执行DESIGN-005写入流程：
   a. 分类推断（DESIGN-007）
   b. Visibility推断（DESIGN-007-A）
   c. 写入评估（DESIGN-005步骤5）
   d. 自动去重
3. 生成迁移报告：
   - 原始记忆数
   - 成功导入数
   - 因去重被合并数
   - 各category分布
   - 各visibility分布
   - 建议人工审查的记忆列表
```

迁移工具为独立CLI命令：`go run ./cmd/migrate --user-id hongyan`

**DESIGN-007-A** Visibility自动推断  
对应 REQ-027（Mem0数据导入时的visibility推断）

```go
func InferVisibility(content string, category string) string {
    // 基于分类和关键词推断可见性
    switch category {
    case "identity":
        // 用户身份信息 -> user级（所有Agent共享）
        return "user"
    case "principle":
        // 工作原则/沟通偏好 -> user级（所有Agent应遵循）
        return "user"
    case "knowledge":
        // 进一步细分：
        // 项目规范、架构决策 -> team级
        // 服务器地址、基础设施 -> team级
        // 默认 -> team级
        return "team"
    case "working":
        // 工作记忆默认 -> private（仅创建者可见）
        return "private"
    default:
        return "private"
    }
}
```

---

## 4. 技术选型

**DESIGN-016** 技术栈选型  
对应 NFR-014（技术栈）、NFR-015（Embedding模型可替换）、NFR-016（向量数据库可替换）

| 组件 | 选型 | 理由 |
|------|------|------|
| 后端框架 | Go (net/http + chi/gorilla mux) | 高性能、跨平台编译、单二进制部署 |
| 元数据存储 | SQLite (modernc.org/sqlite) | 纯Go实现无CGO依赖、零配置、单文件 |
| 向量数据库 | Qdrant | 轻量、Docker一行启动、原生Go SDK、支持过滤 |
| Embedding推理 | ONNX Runtime (sentence-transformers导出) | 本地运行、384维、跨平台、无Python依赖 |
| 向量相似度 | 余弦相似度 | 标准做法，Qdrant原生支持 |
| 容器化 | Docker + docker-compose | 与OpenClaw部署方式一致 |

```yaml
# docker-compose.yml
services:
  agent-memory:
    build: .
    ports:
      - "8100:8100"
    volumes:
      - ./data:/data                # SQLite数据持久化
      - ./config.yaml:/app/config.yaml
    environment:
      - API_KEYS=key1,key2        # 逗号分隔的API Key列表
      - EMBEDDING_MODEL_PATH=/models/all-MiniLM-L6-v2
    depends_on:
      - qdrant

  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
    volumes:
      - ./qdrant_storage:/qdrant/storage
```

**DESIGN-017** 可替换组件抽象  
对应 NFR-015（Embedding模型可替换）、NFR-016（向量数据库可替换）

```go
// 抽象层接口
type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dim() int
}

type VectorStore interface {
    Upsert(ctx context.Context, collection string, points []MemoryVector) error
    Search(ctx context.Context, collection string, vector []float32,
        topK int, filters map[string]string) ([]SearchResult, error)
    Delete(ctx context.Context, collection string, ids []string) error
}

// 实现
type ONNXEmbedding struct { ... }  // 本地ONNX Runtime
type OpenAIEmbedding struct { ... }  // OpenAI API

type QdrantStore struct { ... }  // Qdrant
type ChromaStore struct { ... }  // Chroma（预留）
```

---

## 5. 配置设计

**DESIGN-018** 配置文件结构  
对应 NFR-009（配置外置）

```yaml
# config.yaml
server:
  host: 0.0.0.0
  port: 8100

# Agent注册表（也可通过API动态管理）
# api_key 仅用于权限验证，不用于身份区分
# user_id 是身份隔离的唯一标识，每个 Agent 必须有独立值
agents:
  - id: m10s
    name: OpenClaw-M10S
    user_id: m10s              # 身份隔离标识（DESIGN-021）
    team: default
    api_key: ${M10S_API_KEY}
  - id: devforge
    name: OpenClaw-M10S-dev
    user_id: devforge
    team: default
    api_key: ${DEVFORGE_API_KEY}
  - id: qbot
    name: OpenClaw-M10S-QA
    user_id: qbot
    team: default
    api_key: ${QBOT_API_KEY}
  - id: sage
    name: Sage
    user_id: sage
    team: default
    api_key: ${SAGE_API_KEY}
  - id: clara
    name: Clara
    user_id: clara
    team: default
    api_key: ${CLARA_API_KEY}
  - id: gem12
    name: OpenClaw-GEM12-MAX-Master
    user_id: gem12
    team: default
    api_key: ${GEM12_API_KEY}    # 可以与其他 Agent 共享

storage:
  sqlite_path: ./data/memories.db
  vector:
    provider: qdrant        # qdrant / chroma
    host: qdrant
    port: 6333

embedding:
  provider: local           # local / openai
  model: all-MiniLM-L6-v2  # 本地模型名
  # openai:
  #   api_key: ${OPENAI_API_KEY}
  #   model: text-embedding-3-small
  dimensions: 384

dedup:
  thresholds:
    identity: 0.95
    principle: 0.90
    knowledge: 0.85
    working: 0.70

search:
  scoring:
    similarity_weight: 0.40
    priority_weight: 0.25
    access_count_weight: 0.15
    category_weight: 0.10
    urgency_weight: 0.10
  default_page_size: 10
  max_page_size: 50

ttl:
  scan_interval_hours: 6
  degrade_multiplier: 2
  archive_multiplier: 3

governance:
  compress_threshold: 0.85   # 压缩使用的去重阈值
  auto_delete_days: 0        # >0时自动物理删除归档超期记忆，0=不自动删除

logging:
  level: INFO
  file: ./data/agent-memory.log

monitoring:
  enabled: true
  metrics_path: /metrics
```

---

## 6. 项目结构

**DESIGN-019** 项目目录结构  
对应 NFR-007（独立部署）、NFR-017（通用化预留）

```
agent-memory/
├── docs/
│   ├── requirements.md          # 需求文档
│   └── design.md                # 设计文档（本文件）
├── cmd/
│   └── migrate/
│       └── main.go              # Mem0迁移CLI
├── internal/
│   ├── config/
│   │   └── config.go            # 配置加载
│   ├── model/
│   │   ├── memory.go            # 记忆数据模型
│   │   └── log.go               # 操作日志模型
│   ├── api/
│   │   ├── handler.go           # HTTP handler注册
│   │   ├── memories.go          # 记忆CRUD接口
│   │   ├── search.go            # 检索接口
│   │   ├── governance.go        # 治理接口（压缩/报告）
│   │   ├── health.go            # 健康检查
│   │   └── middleware.go        # API Key认证中间件
│   ├── core/
│   │   ├── writer.go            # 写入模块（去重+门控+元数据）
│   │   ├── retriever.go         # 检索模块（语义+多维排序）
│   │   ├── classifier.go        # 分类推断器
│   │   ├── ttl_manager.go       # TTL过期管理
│   │   └── compressor.go        # 批量压缩
│   ├── storage/
│   │   ├── dal.go               # 数据访问层接口
│   │   ├── sqlite.go            # SQLite操作
│   │   └── vector.go            # 向量存储抽象+实现
│   ├── embedding/
│   │   ├── provider.go          # Embedding抽象层接口
│   │   ├── onnx.go              # 本地ONNX Runtime
│   │   └── openai.go            # OpenAI API
│   └── plugin/
│       └── openclaw.go          # OpenClaw插件适配
├── pkg/
│   └── scoring/
│       └── scorer.go            # 多维度排序打分
├── go.mod
├── go.sum
├── config.yaml                  # 默认配置
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── README.md
```

---

## 7. 需求追溯矩阵

| 设计编号 | 对应需求 |
|----------|----------|
| DESIGN-001 | REQ-023, REQ-024, NFR-007 |
| DESIGN-002 | REQ-003, REQ-004, REQ-020 |
| DESIGN-003 | REQ-011, NFR-006 |
| DESIGN-003-A | REQ-020, REQ-022 |
| DESIGN-004 | NFR-012 |
| DESIGN-005 | REQ-005, REQ-007, REQ-018, REQ-019, REQ-021 |
| DESIGN-006 | REQ-006 |
| DESIGN-007 | REQ-002 |
| DESIGN-007-A | REQ-027 |
| DESIGN-008 | REQ-011, REQ-012, REQ-013, REQ-014, REQ-021 |
| DESIGN-009 | REQ-012 |
| DESIGN-010 | REQ-008, REQ-009, REQ-010 |
| DESIGN-011 | REQ-015 |
| DESIGN-012 | REQ-016 |
| DESIGN-013 | REQ-023, REQ-025, REQ-020 |
| DESIGN-014 | REQ-024, REQ-020 |
| DESIGN-015 | REQ-027 |
| DESIGN-016 | NFR-014, NFR-015, NFR-016 |
| DESIGN-017 | NFR-015, NFR-016 |
| DESIGN-018 | NFR-009 |
| DESIGN-019 | NFR-007, NFR-017 |
| DESIGN-020 | REQ-020, REQ-025（身份隔离架构） |
| DESIGN-021 | REQ-020（Agent 配置结构） |
| DESIGN-022 | REQ-024（Plugin 身份注入） |
| DESIGN-023 | REQ-020（X-User-Id 覆盖修复） |

---

## 8. 实现计划

| 阶段 | 内容 | 预计工作量 |
|------|------|-----------|
| Phase 1 | 项目骨架 + SQLite存储 + 基础CRUD API | 1天 |
| Phase 2 | ONNX Embedding集成 + Qdrant向量存储 + 语义检索 | 1.5天 |
| Phase 3 | 去重机制 + 分类推断 + 写入门控 | 1天 |
| Phase 4 | TTL过期 + 批量压缩 + 健康报告 | 1天 |
| Phase 5 | Mem0迁移工具 + OpenClaw插件 | 0.5天 |
| Phase 6 | Docker化 + 测试 + 文档完善 | 0.5天 |
| **Phase 7** | **身份隔离修复（DESIGN-020~023）** | **0.5天** |

## 9. 变更历史

### v1.2 (2026-04-12) — 身份隔离架构修复

**修复的 Bug：**

| # | 位置 | 问题 | 修复 | 设计编号 |
|---|------|------|------|----------|
| 1 | Plugin `index.ts` | 硬编码 `AGENT_USER_MAP`（`{main:"m10s", dev:"devforge", ...}`）覆盖 config 中的 `userId`，导致不同机器上 agentId 相同的实例（如都是 "main"）共享同一个身份 | 删除 `AGENT_USER_MAP`，`resolveUserId()` 直接返回 `config.userId` | DESIGN-022 |
| 2 | Server `config.go` + `main.go` | `AgentEntry` 没有 `user_id` 字段，`seedAgents()` 硬编码 `UserID: "default"`，导致 agents 表中所有 agent 的 user_id 相同 | `AgentEntry` 增加 `user_id` 字段，`seedAgents()` 读取使用 | DESIGN-021 |
| 3 | Server `middleware.go` | X-User-Id 覆盖时，若 `GetAgentByUserID` 返回 nil，只更新了 `userID`，`agentID` 和 `team` 仍为 API Key 对应 agent 的值 | fallback 时以 X-User-Id 同时设置 userID 和 agentID | DESIGN-023 |
| 4 | Server `sqlite.go` | `api_key_hash` 有 UNIQUE 约束，导致共享 API Key 的多个 Agent 只能注册第一个（`INSERT OR IGNORE` 静默跳过） | 去掉 UNIQUE 约束，改为普通索引 | DESIGN-020 |

**架构原则变更：**
- API Key 职责明确为**权限验证**，不再承担身份区分
- X-User-Id Header 是**唯一的身份标识来源**
- Plugin 身份来源**唯一**是 openclaw.json 中的 `config.userId`，禁止硬编码映射
