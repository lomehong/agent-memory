# Agent-Memory 进化系统 — 设计文档

> 版本：v1.0  
> 日期：2026-04-13  
> 状态：待评审  
> 前置文档：[REQUIREMENTS.md](./REQUIREMENTS.md)

---

## 1. 架构概览

### 1.1 新增模块

在现有 agent-memory 服务内部新增三个模块，不引入新进程或新依赖：

```
internal/
├── core/
│   ├── classifier.go      # 已有
│   ├── ttl_manager.go     # 已有，需增强
│   ├── compressor.go      # 已有
│   ├── retriever.go       # 已有
│   ├── writer.go          # 已有
│   ├── dreamer.go         # 新增：Dream 回顾提炼
│   ├── reviewer.go        # 新增：任务复盘分析
│   └── heat_scorer.go     # 新增：热度评分
├── llm/
│   └── glm.go             # 新增：智谱 GLM 调用封装
internal/api/
├── handler.go             # 已有，需新增路由
```

### 1.2 调用链路

```
┌─────────────┐     cron 03:00      ┌──────────────┐
│   OpenClaw   │ ──────────────────→ │ POST /dream  │
│   cron job   │                     │   Dreamer    │──→ GLM (总结增强)
└─────────────┘                     └──────┬───────┘
                                            │
┌─────────────┐     任务完成时       ┌──────▼───────┐
│   OpenClaw   │ ──────────────────→ │ POST /review │
│   插件调用    │                     │   Reviewer   │──→ GLM (可选增强)
└─────────────┘                     └──────────────┘

┌─────────────┐     TTL 扫描时      ┌──────────────┐
│   TTL        │ ──────────────────→ │ HeatScorer   │
│   Manager    │      (6h周期)       │  (内嵌调用)   │
└─────────────┘                     └──────────────┘
```

---

## 2. Dream 回顾提炼 — 详细设计

### 2.1 Dreamer 结构体

```go
type Dreamer struct {
    dal          storage.DAL
    vector       storage.VectorStore
    scorer       *scoring.Scorer
    llm          *llm.GLMClient      // GLM 智能总结
    logger       *zerolog.Logger
    config       DreamerConfig
}

type DreamerConfig struct {
    DefaultLookbackDays int     // 默认回溯天数，默认 7
    PatternThreshold    float64 // 重复模式相似度阈值，默认 0.7
    InsightSimilarity   float64 // 洞察去重阈值，默认 0.85
    MaxSourceMemories   int     // 单条洞察最大关联源记忆数，默认 20
}
```

### 2.2 Dream 执行流程

```
POST /api/v1/dream
    │
    ▼
1. 参数解析 & 校验
    │
    ▼
2. 加载时间窗口内的记忆（按 agent_id 过滤）
    │  GET /memories?created_after=<date>&agent_id=<id>
    ▼
3. 模式识别（向量+统计）
    │
    ├─ 3a. 重复模式检测
    │      对所有记忆做两两向量相似度比较（批量）
    │      聚类：相似度 > threshold 的归为一组
    │      组内 >= 3 条 → 候选重复模式
    │
    ├─ 3b. 趋势模式检测
    │      按 category 分组统计
    │      对比当前窗口 vs 上一窗口的增长率
    │      增长率 > 200% → 候选趋势模式
    │
    ├─ 3c. 孤立记忆检测
    │      筛选 priority <= 2 且 access_count == 0
    │      候选孤立记忆（列表形式）
    │
    └─ 3d. 冲突模式检测
           按 tags/topic 聚类
           组内记忆向量距离 > 1.5 但 category 相同 → 候选冲突
    │
    ▼
4. GLM 智能总结（已决策：接入智谱 GLM）
    │  将步骤 3 的候选模式 + 原始记忆内容发送给 GLM
    │  prompt 要求 GLM：
    │  - 判断候选模式是否为真阳性（过滤误报）
    │  - 生成简洁的模式描述
    │  - 给出可操作的行动建议
    │  配置：model=glm-5-turbo, max_tokens=2000
    │  失败回退：GLM 不可用时使用规则生成的描述（降级）
    ▼
5. 洞察去重
    │  新洞察 vs 已有 dream 洞察做相似度比较
    │  相似度 > 0.85 → 更新已有洞察，追加 source_memories
    ▼
6. 写入洞察记忆（dry_run 时跳过）
    │  category = "knowledge", priority = 2
    │  tags = ["dream", "<pattern_type>", "<date>"]
    ▼
7. 写入 Dream 日志记忆
    │  category = "working", tags = ["dream-log"]
    ▼
8. 返回 Dream 报告 JSON
```

### 2.3 API 设计

```
POST /api/v1/dream
Request:
{
    "agent_id": "all" | "m10s" | "devforge" | ...,  // 默认 "all"
    "lookback_days": 7,                              // 默认 7
    "dry_run": false                                 // 默认 false
}

Response 200:
{
    "status": "ok",
    "dream_id": "uuid",
    "summary": {
        "agents_scanned": 6,
        "memories_analyzed": 234,
        "patterns_found": {
            "repetition": 2,
            "trend": 1,
            "orphan": 5,
            "conflict": 0
        },
        "insights_created": 3,
        "insights_updated": 1,
        "duration_ms": 1234,
        "llm_used": true,
        "llm_model": "glm-5-turbo"
    },
    "insights": [
        {
            "pattern_type": "repetition",
            "description": "M10S 在过去 7 天内 5 次遇到 SSH 连接超时问题",
            "source_memory_ids": ["id1", "id2", ...],
            "action_suggestion": "建议检查 131 服务器的 SSH 配置稳定性"
        }
    ]
}
```

### 2.4 GLM 集成设计

```go
// internal/llm/glm.go
type GLMClient struct {
    baseURL    string  // https://open.bigmodel.cn/api/coding/paas/v4
    apiKey     string  // 从环境变量 GLM_API_KEY 或配置读取
    model      string  // glm-5-turbo
    httpClient *http.Client
    timeout    time.Duration  // 默认 10s
}

func (c *GLMClient) SummarizePatterns(ctx context.Context, patterns []CandidatePattern, memories []MemorySummary) ([]Insight, error)
```

Dream 调用 GLM 的 prompt 模板：

```
你是 AI Agent 记忆系统的分析模块。以下是从 Agent 记忆中识别到的候选模式，
请判断每个模式是否为真实有价值的模式，并生成简洁描述和行动建议。

## 候选模式
{patterns_json}

## 原始记忆摘要
{memory_summaries}

## 输出格式（JSON）
[
  {
    "valid": true/false,
    "pattern_type": "repetition|trend|orphan|conflict",
    "description": "简洁描述",
    "action_suggestion": "可操作建议（可选）"
  }
]
```

**降级策略**：
- GLM 调用超时（10s）或失败时，使用规则引擎生成的默认描述
- 规则降级描述格式：`"[规则生成] <pattern_type>: 发现 <N> 条相似记忆"` 
- Dream 日志中记录 `llm_used: false`，便于追踪

### 2.5 性能优化

- **向量批量查询**：Qdrant 支持 batch search，避免 N² 单次查询
- **聚类算法**：使用简单的贪心聚类（O(N²) 但 N 通常 < 500），不引入复杂算法
- **幂等性**：Dream 可安全重复执行，不会产生重复洞察（靠去重机制）
- **GLM 并发控制**：单次 Dream 最多调用 GLM 1 次（批量发送所有候选模式），避免 API 频率限制

---

## 3. 任务复盘分析 — 详细设计

### 3.1 Reviewer 结构体

```go
type Reviewer struct {
    dal          storage.DAL
    vector       storage.VectorStore
    classifier   *Classifier
    llm          *llm.GLMClient      // 可选 GLM 增强
    logger       *zerolog.Logger
}

type ReviewReport struct {
    AgentID    string              `json:"agent_id"`
    TimeRange  TimeRange           `json:"time_range"`
    Summary    ReviewSummary       `json:"summary"`
    Findings   ReviewFindings      `json:"findings"`
    ActionItems []string           `json:"action_items"`
    RelatedIDs []string            `json:"related_memories"`
}

type ReviewFindings struct {
    Experience []FindingItem `json:"experience"`  // 踩坑经验
    Skills     []FindingItem `json:"skills"`      // 可复用流程
    Principles []FindingItem `json:"principles"`  // 抽象准则
    Insights   []FindingItem `json:"insights"`    // 跨场景泛化
    Queries    []FindingItem `json:"queries"`     // 值得留存的问答
}

type FindingItem struct {
    Description string   `json:"description"`
    Evidence    []string `json:"evidence"`  // 相关记忆ID
    Confidence  float64  `json:"confidence"`
}
```

### 3.2 五问分析逻辑

```
POST /api/v1/review
    │
    ▼
1. 加载时间窗口内指定 Agent 的记忆
    │
    ▼
2. 五问分析（规则 + 向量，GLM 可选增强）
    │
    ├─ Q1 踩坑了吗？
    │  筛选条件：category=working + 内容包含错误/失败/问题信号词
    │  信号词：["失败", "错误", "bug", "问题", "异常", "error", "fail", "超时", "timeout"]
    │  输出 → findings.experience
    │
    ├─ Q2 可复用流程？
    │  对 working 类记忆做向量聚类
    │  同一聚类 >= 2 条 → 说明有重复操作
    │  输出 → findings.skills
    │
    ├─ Q3 抽象准则？
    │  检查 principle 类记忆
    │  对比现有 principle，找是否有新行为模式未被规则化
    │  判断依据：某类 working 记忆反复出现且已有规则未覆盖
    │  输出 → findings.principles
    │
    ├─ Q4 跨场景泛化？
    │  对 knowledge 类记忆做跨 Agent 相似度搜索
    │  如果 A Agent 的某条记忆与 B Agent 的记忆高度相似
    │  但 visibility=private → 建议提升为 team
    │  输出 → findings.insights
    │
    └─ Q5 值得留存的问答？
       筛选包含问号/问号模式 + 后续有回答的记忆
       判断依据：一条记忆是问题，时间窗口内有后续记忆是其解答
       输出 → findings.queries
    │
    ▼
3. 生成 action_items
    │  基于 findings 生成建议：
    │  - experience 有发现 → "建议将踩坑经验写入 knowledge"
    │  - skills 有发现 → "建议将复用流程文档化"
    │  - insights 有发现 → "建议将跨 Agent 知识设为 team 可见"
    │  可选：调用 GLM 优化 action_items 表述
    ▼
4. 返回 ReviewReport JSON
```

### 3.3 API 设计

```
POST /api/v1/review
Request:
{
    "agent_id": "m10s",
    "since": "2026-04-06T00:00:00Z",  // 必填
    "use_llm": false                    // 可选，默认 false
}

Response 200:
{
    "agent_id": "m10s",
    "time_range": { "from": "2026-04-06T00:00:00Z", "to": "2026-04-13T00:00:00Z" },
    "summary": {
        "total_memories": 47,
        "categories": { "identity": 2, "principle": 3, "knowledge": 15, "working": 27 }
    },
    "findings": {
        "experience": [
            {
                "description": "131 服务器 agent-memory 服务反复宕机（3次）",
                "evidence": ["id1", "id2", "id3"],
                "confidence": 0.92
            }
        ],
        "skills": [
            {
                "description": "SSH 到 131 执行 sudo 命令的标准流程",
                "evidence": ["id4", "id5"],
                "confidence": 0.85
            }
        ],
        "principles": [],
        "insights": [],
        "queries": []
    },
    "action_items": [
        "建议将 131 宕机排查经验写入 knowledge",
        "建议将 SSH sudo 流程文档化为 skill"
    ],
    "related_memories": ["id1", "id2", "id3", "id4", "id5"]
}
```

---

## 4. 热度感知归档 — 详细设计

### 4.1 HeatScorer 结构体

```go
type HeatScorer struct {
    config HeatConfig
}

type HeatConfig struct {
    RecencyWeight      float64 // 时间衰减权重，默认 0.4
    FrequencyWeight    float64 // 访问频率权重，默认 0.6
    HeatThreshold      float64 // 降级热度阈值，默认 30
    ExtensionMultiplier float64 // 高热度 TTL 延长倍数，默认 1.5
}
```

### 4.2 热度评分算法

```go
func (hs *HeatScorer) Score(memory model.Memory) float64 {
    // 频率分：对数缩放，避免极端值
    freqScore := math.Log(float64(memory.AccessCount) + 1) * 10
    // 时间衰减分：距上次访问越久分越低
    daysSinceAccess := time.Since(memory.LastAccessed).Hours() / 24
    recencyScore := math.Max(0, 100 - daysSinceAccess*5)
    
    return freqScore*hs.config.FrequencyWeight + 
           recencyScore*hs.config.RecencyWeight
}
```

示例计算：

| access_count | days_since_access | freqScore | recencyScore | heat_score |
|-------------|-------------------|-----------|-------------|------------|
| 0 | 30 | 0 | 25 | 10 |
| 1 | 7 | 6.9 | 65 | 42.8 |
| 5 | 3 | 16.1 | 85 | 56.9 |
| 35 | 1 | 35.6 | 95 | 70.3 |

### 4.3 增强 TTL 策略

修改现有 `ttl_manager.go` 中的降级逻辑：

```
原有逻辑：
  if ttl_expired(now) → degrade(memory)

新逻辑：
  if ttl_expired(now) {
      heat = heatScorer.Score(memory)
      if heat >= config.HeatThreshold {
          // 高热度：延长 TTL
          memory.TTL *= config.ExtensionMultiplier
          update(memory)
      } else {
          degrade(memory)
      }
  }
```

### 4.4 增强健康报告

在 `/api/v1/memories/report` 响应中新增字段：

```json
{
    "heat": {
        "top_hot": [
            { "id": "...", "content_preview": "...", "heat_score": 85.2, "access_count": 35 }
        ],
        "top_cold": [
            { "id": "...", "content_preview": "...", "heat_score": 5.1, "access_count": 0, "priority": 2 }
        ],
        "distribution": {
            "hot_70_100": 12,
            "warm_30_70": 45,
            "cold_0_30": 89
        }
    }
}
```

---

## 5. GLM 客户端 — 公共模块

Dream 和 Review 共用同一个 GLM 客户端：

```go
// internal/llm/glm.go
type GLMClient struct {
    baseURL    string
    apiKey     string
    model      string
    httpClient *http.Client
    timeout    time.Duration
}

// NewGLMClient 从配置创建
func NewGLMClient(cfg config.LLMConfig) *GLMClient

// ChatCompletion 通用对话接口
func (c *GLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error)

// SummarizeDreamPatterns Dream 专用：分析候选模式
func (c *GLMClient) SummarizeDreamPatterns(ctx context.Context, req DreamSummarizeRequest) ([]Insight, error)

// EnhanceReviewActionItems Review 专用：优化行动建议
func (c *GLMClient) EnhanceReviewActionItems(ctx context.Context, findings ReviewFindings) ([]string, error)
```

配置项：

```yaml
llm:
  enabled: true
  provider: zhipu
  base_url: "https://open.bigmodel.cn/api/coding/paas/v4"
  api_key: "${GLM_API_KEY}"     # 环境变量注入
  model: "glm-5-turbo"
  timeout_seconds: 10
  max_tokens: 2000
```

---

## 6. 数据库变更

### 6.1 SQLite Schema 变更

```sql
-- memories 表新增字段（ALTER TABLE，兼容现有数据）
ALTER TABLE memories ADD COLUMN heat_score REAL DEFAULT 0;
ALTER TABLE memories ADD COLUMN heat_updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

-- dream 执行记录表（新建）
CREATE TABLE IF NOT EXISTS dream_runs (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    memories_analyzed INTEGER DEFAULT 0,
    patterns_found INTEGER DEFAULT 0,
    insights_created INTEGER DEFAULT 0,
    insights_updated INTEGER DEFAULT 0,
    llm_used BOOLEAN DEFAULT FALSE,
    status TEXT DEFAULT 'running',  -- running | completed | failed
    error TEXT,
    report_json TEXT
);
```

### 6.2 Qdrant Payload 变更

无需变更。洞察记忆复用现有 payload 结构，通过 tags 区分。

---

## 7. 配置变更

```yaml
# config.yaml 新增项
dream:
  enabled: true
  default_lookback_days: 7
  pattern_threshold: 0.7
  insight_dedup_threshold: 0.85
  max_source_memories: 20

review:
  enabled: true
  # 五问分析的关键词配置
  error_keywords: ["失败", "错误", "bug", "问题", "异常", "error", "fail", "超时", "timeout"]

llm:
  enabled: true
  provider: zhipu
  base_url: "https://open.bigmodel.cn/api/coding/paas/v4"
  api_key: "${GLM_API_KEY}"
  model: "glm-5-turbo"
  timeout_seconds: 10
  max_tokens: 2000

ttl:
  scan_interval_hours: 6
  degrade_multiplier: 2
  archive_multiplier: 3
  # 新增热度相关
  heat_threshold: 30
  heat_extension_multiplier: 1.5
  heat_recency_weight: 0.4
  heat_frequency_weight: 0.6
```

---

## 8. OpenClaw 插件变更

### 8.1 新增工具 Schema

```typescript
// memory_review 工具
{
    name: "memory_review",
    description: "回顾指定时间范围内的记忆，生成结构化复盘报告，包含踩坑经验、可复用流程、抽象准则、跨场景泛化和值得留存的问答",
    inputSchema: {
        type: "object",
        properties: {
            agent_id: { type: "string", description: "要复盘的 Agent ID，默认当前 Agent" },
            since: { type: "string", description: "复盘起始时间（ISO 8601），默认 7 天前" }
        },
        required: []
    }
}
```

### 8.2 autoCapture 增强

在 autoCapture 流程末尾，新增"任务完成检测"：

```
检测规则（已决策：暂时用信号词方案）：
1. 用户消息包含完成信号词：["完成了", "搞定了", "做好了", "done", "finished"]
2. 且当前会话中 autoCapture 已提取 >= 3 条新记忆
3. 满足条件 → 自动调用 POST /review，生成复盘报告
4. 复盘报告注入到下一轮上下文（但不自动写入记忆）
```

---

## 9. Cron 集成

### 9.1 方案一：OpenClaw cron（推荐）

```json
{
    "jobs": [
        {
            "name": "agent-memory-dream",
            "schedule": "0 3 * * *",
            "command": "curl -s -X POST http://192.168.2.131:8101/api/v1/dream -H 'Content-Type: application/json' -H 'X-API-Key: dev-api-key-001' -d '{\"agent_id\":\"all\",\"lookback_days\":7}'",
            "enabled": true
        }
    ]
}
```

### 9.2 方案二：服务内建定时器（备选）

如果 OpenClaw cron 不支持 HTTP 调用，在 agent-memory 服务内建定时器：

```go
// cmd/server/main.go
if config.Dream.Enabled {
    go dreamScheduler.Start(dal, vector, config.Dream)
}

func (ds *DreamScheduler) Start() {
    ticker := time.NewTicker(24 * time.Hour)
    for range ticker.C {
        now := time.Now()
        if now.Hour() == 3 {
            ds.dreamer.Run("all", ds.config.DefaultLookbackDays, false)
        }
    }
}
```

---

## 10. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Dream 向量聚类在记忆量大时性能下降 | 执行超时 | 限制 lookback_days，使用 Qdrant batch search，必要时分页 |
| GLM API 不可用或超时 | Dream/Review 质量降级 | 降级到纯规则模式，确保流程不中断 |
| GLM 总结质量不稳定 | 洞察不准确 | 保留向量+统计的候选模式作为兜底，GLM 只做增强不替代 |
| 五问分析信号词覆盖不全 | 复盘报告遗漏 | 信号词可配置，后续可扩展 |
| 热度算法参数不合适 | 记忆被误归档 | 提供配置项，初始保守值，运行一周后根据 report 数据调优 |
| Dream 生成垃圾洞察 | 记忆库质量下降 | 设置 minimum_cluster_size=3，只有充分证据才生成洞察 |

---

## 11. 测试策略

| 测试类型 | 覆盖范围 | 方法 |
|----------|----------|------|
| 单元测试 | HeatScorer、聚类算法、五问分析、GLM prompt 构造 | 表驱动测试，mock storage |
| 集成测试 | Dream/Review API 端到端 | Docker Compose + Qdrant + 真实 SQLite |
| GLM 集成测试 | GLM 调用 + 降级逻辑 | mock HTTP server 模拟超时/错误 |
| 回归测试 | 现有 API 不受影响 | 现有测试套件全部通过 |
| 性能测试 | Dream 在 500 条记忆下的执行时间 | benchmark，目标 < 30s |
