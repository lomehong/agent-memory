# Agent-Memory Web Dashboard - 设计文档

> 项目代号：agent-memory-web  
> 需求文档：docs/web-requirements.md  
> 创建日期：2026-04-09  
> 文档版本：v1.1  
> 更新日期：2026-04-10  
> 更新内容：前端代码独立为 frontend/ 目录，Go embed 嵌入

---

## 1. 架构概览

```
┌─────────────────────────────────────────────────┐
│                   浏览器                          │
│  ┌───────────────────────────────────────────┐  │
│  │         agent-memory-web (SPA)            │  │
│  │   纯HTML/CSS/JS + Chart.js (CDN)         │  │
│  │                                           │  │
│  │  /           → 概览仪表盘                  │  │
│  │  /memories   → 记忆管理                    │  │
│  │  /agents     → Agent管理                   │  │
│  │  /system     → 系统信息                    │  │
│  └──────────────────┬────────────────────────┘  │
└─────────────────────┼───────────────────────────┘
                      │ fetch API
┌─────────────────────┼───────────────────────────┐
│       agent-memory server (Go binary)           │
│  ┌──────────────────┴────────────────────────┐  │
│  │  http.FileServer (embed.FS)  → /          │  │
│  │  API Router (chi)           → /api/v1/*   │  │
│  └───────────────────────────────────────────┘  │
│  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │  Writer  │  │ Retriever│  │   TTL/Misc   │  │
│  └────┬─────┘  └────┬─────┘  └──────┬────────┘  │
│  ┌────┴──────────────┴───────────────┴────────┐  │
│  │              SQLite + Vector Store         │  │
│  └─────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

**DESIGN-WEB-001** 静态文件嵌入  
对应 WEB-NFR-008。前端文件放在 `web/` 目录下，通过 Go 的 `//go:embed` 指令嵌入二进制。API 路由注册优先于静态文件服务，避免路由冲突。

**DESIGN-WEB-002** SPA路由  
前端采用简单的 hash-based 路由（`#/memories`、`#/agents`、`#/system`），无需服务器端路由支持。所有页面在一个 HTML 文件中通过 JS 切换显示。

**DESIGN-WEB-003** 账号密码认证（取代v1.0的API Key方式）  
Dashboard采用账号密码登录，取代原有的API Key输入方式：
- 新增 `POST /api/v1/auth/login` 接口，验证用户名密码，返回JWT token
- 新增 `POST /api/v1/auth/logout` 接口，使token失效（可选）
- 新增 `GET /api/v1/auth/me` 接口，验证当前token有效性并返回用户信息
- JWT payload包含 `sub`（用户名）、`iat`（签发时间）、`exp`（过期时间）
- 前端所有API请求附带 `Authorization: Bearer <token>` header
- 管理员账户在 `config.yaml` 的 `web.admins` 节点配置，密码为bcrypt哈希
- 未登录或token过期时，前端重定向到登录页（`#/login`）
- 后端API中间件同时支持两种认证方式：JWT token（Dashboard用）和 X-API-Key（程序化调用用）

---

## 2. 目录结构

**DESIGN-WEB-004** 前端目录结构

```
web/
├── index.html          # 单页应用入口
├── css/
│   └── style.css       # 全局样式
├── js/
│   ├── app.js          # 应用入口、路由、全局状态
│   ├── api.js          # API调用封装
│   ├── dashboard.js    # 概览仪表盘页面
│   ├── memories.js     # 记忆管理页面
│   ├── agents.js       # Agent管理页面
│   └── system.js       # 系统信息页面
└── assets/
    └── logo.svg        # 项目Logo
```

后端集成：
```
cmd/server/
└── main.go             # 新增: embed.FS 注册 + FileServer
```

---

## 3. API封装层

**DESIGN-WEB-005** 前端API客户端  
统一封装所有API调用，对应 WEB-NFR-004。

```javascript
class AgentMemoryAPI {
  constructor(baseUrl, apiKey) { ... }

  // 记忆操作
  async search(query, { category, topK })        // GET /memories/search
  async listMemories({ category, status, limit, offset })  // GET /memories
  async getMemory(id)                             // GET /memories/{id}
  async createMemory({ content, category, priority, tags }) // POST /memories
  async updateMemory(id, updates)                 // PUT /memories/{id}
  async deleteMemory(id)                          // DELETE /memories/{id}
  async batchCreate(items)                        // POST /memories/batch
  async compress()                                // POST /memories/compress
  async getReport()                               // GET /memories/report

  // Agent操作
  async listAgents()                              // GET /agents
  async createAgent({ name, team })               // POST /agents
  async deleteAgent(id)                           // DELETE /agents/{id}

  // 系统
  async health()                                  // GET /health
  async metrics()                                 // GET /metrics
}
```

---

## 4. 页面设计

### 4.1 全局布局

**DESIGN-WEB-006** 页面骨架

```
┌──────────────────────────────────────────────────────────┐
│  🧠 Agent Memory                    [API Key: ****001] [⚙] │
├──────────┬───────────────────────────────────────────────┤
│          │                                               │
│  📊 概览  │              页面内容区                        │
│  📝 记忆  │                                               │
│  🤖 Agent│                                               │
│  ℹ️ 系统  │                                               │
│          │                                               │
│          │                                               │
│          │                                               │
│          │                                               │
│          │                                               │
└──────────┴───────────────────────────────────────────────┘
```

- 顶部栏：Logo + 系统名称 + API Key状态 + 设置按钮
- 左侧导航：固定宽度，4个页面入口
- 内容区：根据路由切换不同页面内容

### 4.2 概览仪表盘

**DESIGN-WEB-007** 仪表盘布局

```
┌──────────────────────────────────────────────────────────┐
│  📊 概览仪表盘                                            │
├───────────┬───────────┬───────────┬─────────────────────┤
│ 总记忆数   │ Active    │ Degraded  │ Archived             │
│   128     │   115 🟢  │   8 🟡    │   5 🔴              │
├───────────┴───────────┴───────────┴─────────────────────┤
│                                                          │
│  ┌─── 分类分布 ───────┐  ┌─── 状态分布 ───────┐          │
│  │  🍩 Doughnut Chart │  │  📊 Bar Chart      │          │
│  │  identity: 15      │  │  active:  115      │          │
│  │  principle: 32     │  │  degraded: 8       │          │
│  │  knowledge: 45     │  │  archived: 5       │          │
│  │  working: 36       │  │                    │          │
│  └────────────────────┘  └────────────────────┘          │
│                                                          │
│  ┌─── 热度 Top 5 ──────────────────────────────┐         │
│  │  1. [identity] 洪岩是我的主人...    42次    │         │
│  │  2. [principle] 可靠性优先...        28次    │         │
│  │  3. [knowledge] agent-memory使用Go... 15次  │         │
│  │  4. ...                                     │         │
│  │  5. ...                                     │         │
│  └─────────────────────────────────────────────┘         │
│                                                          │
│  ┌─── 一键操作 ─────────────────────────────────┐        │
│  │  [🔄 批量压缩]  [📊 查看完整报告]             │        │
│  └─────────────────────────────────────────────┘        │
└──────────────────────────────────────────────────────────┘
```

数据来源：
- 总数/状态：`GET /api/v1/memories/report` → `total_count`, `by_status`
- 分类分布：`report.by_category`
- 热度Top 5：`report.top_accessed`
- 批量压缩：`POST /api/v1/memories/compress`

### 4.3 记忆管理页

**DESIGN-WEB-008** 记忆列表页布局

```
┌──────────────────────────────────────────────────────────┐
│  📝 记忆管理                           [+ 新建记忆]       │
├──────────────────────────────────────────────────────────┤
│  🔍 [语义搜索...                    ] [搜索]             │
│                                                          │
│  过滤: [全部Category▾] [全部Status▾] [全部Visibility▾]  │
│                                                          │
│  ☐ │ID(截断)    │内容          │分类  │优先│状态│访问│时间│
│  ──┼────────────┼─────────────┼─────┼───┼───┼───┼────│
│  ☐ │b33b...     │洪岩是我的主人 │🪪id │ 1 │🟢  │ 0 │20:51│
│  ☐ │b959...     │可靠性优先... │📐pr │ 2 │🟢  │ 0 │20:51│
│  ☐ │5c9b...     │agent-memory │📚kn │ 3 │🟢  │ 2 │20:48│
│  ☐ │...         │...          │...  │...│...│...│... │
│                                                          │
│  [批量删除(0)]                    « 1  2  3 »  共28条    │
└──────────────────────────────────────────────────────────┘
```

**DESIGN-WEB-009** 记忆详情弹窗

```
┌─── 记忆详情 ──────────────────────────────────────┐
│  ID: b33bbc79-a3ee-4ba8-ab3e-a9c9c42deae8         │
│  状态: 🟢 Active  │ 版本: 1                        │
│                                                    │
│  内容:                                              │
│  ┌──────────────────────────────────────────────┐  │
│  │ 洪岩是我的主人，时区Asia/Shanghai             │  │
│  └──────────────────────────────────────────────┘  │
│                                                    │
│  分类: [identity ▾]  优先级: [1 ▾]                 │
│  可见性: [user ▾]     TTL: [permanent ▾]           │
│  标签: [                                          │
│                                                    │
│  创建: 2026-04-09 20:51  更新: 2026-04-09 20:51   │
│  最后访问: 2026-04-09 20:51  访问次数: 0           │
│                                                    │
│  [💾 保存]  [🗑️ 删除]              [✕ 关闭]       │
└────────────────────────────────────────────────────┘
```

**DESIGN-WEB-010** 新建记忆弹窗

```
┌─── 新建记忆 ──────────────────────────────────────┐
│  内容 *:                                            │
│  ┌──────────────────────────────────────────────┐  │
│  │                                              │  │
│  │                                              │  │
│  └──────────────────────────────────────────────┘  │
│                                                    │
│  分类: [自动推断 ▾]   优先级: [自动 ▾]              │
│  可见性: [自动推断 ▾]  TTL: [自动 ▾]                │
│  标签: [用逗号分隔]                                  │
│                                                    │
│  ── 系统推荐（提交后显示）──                         │
│  📋 推荐分类: identity  推荐可见性: user             │
│  📋 推荐优先级: 1  推荐TTL: permanent              │
│  🔗 去重检测: 未命中 / 已合并到 xxx (score: 0.95)   │
│                                                    │
│  [💾 创建]                        [✕ 取消]         │
└────────────────────────────────────────────────────┘
```

### 4.4 Agent管理页

**DESIGN-WEB-011** Agent列表页

```
┌──────────────────────────────────────────────────────────┐
│  🤖 Agent管理                          [+ 注册Agent]     │
├───────────┬────────────┬──────┬────────────┬────────────┤
│ Agent ID  │ Name       │ Team │ API Key    │ 操作       │
├───────────┼────────────┼──────┼────────────┼────────────┤
│ m10s      │ OpenClaw.. │ def  │ ****001 [👁]│ [🗑️]      │
│ devforge  │ OpenClaw.. │ def  │ ****001 [👁]│ [🗑️]      │
│ qbot      │ OpenClaw.. │ def  │ ****001 [👁]│ [🗑️]      │
├───────────┴────────────┴──────┴────────────┴────────────┤
│  创建时间: 2026-04-09 20:13                          │
└──────────────────────────────────────────────────────────┘
```

**DESIGN-WEB-012** 注册Agent弹窗

```
┌─── 注册Agent ──────────────────────────────────────┐
│  Name *: [                                          │
│  Team:   [default                                   │
│  API Key: [自动生成中...]  [🔄 重新生成]             │
│                                                    │
│  ⚠️ API Key 仅显示一次，请妥善保存                    │
│                                                    │
│  [💾 注册]                        [✕ 取消]         │
└────────────────────────────────────────────────────┘
```

### 4.5 系统信息页

**DESIGN-WEB-013** 系统页面布局

```
┌──────────────────────────────────────────────────────────┐
│  ℹ️ 系统信息                                              │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─── 服务状态 ──────────────────────────────────┐       │
│  │  状态: ✅ Running                              │       │
│  │  版本: v1.0.0                                  │       │
│  │  启动时间: 2026-04-09 20:48                    │       │
│  │  运行时长: 2h 15m                              │       │
│  └────────────────────────────────────────────────┘       │
│                                                          │
│  ┌─── 存储配置 ──────────────────────────────────┐       │
│  │  数据库: SQLite (./data/memories.db)          │       │
│  │  向量存储: In-Memory (dev mode)               │       │
│  │  Embedding: Mock (384 dims)                   │       │
│  └────────────────────────────────────────────────┘       │
│                                                          │
│  ┌─── 去重阈值 ──────────────────────────────────┐       │
│  │  identity:  0.95  ████████████████████░░ 95%  │       │
│  │  principle: 0.90  ██████████████████░░░░ 90%  │       │
│  │  knowledge: 0.85  ████████████████░░░░░░ 85%  │       │
│  │  working:   0.70  ██████████████░░░░░░░░ 70%  │       │
│  └────────────────────────────────────────────────┘       │
│                                                          │
│  ┌─── TTL策略 ───────────────────────────────────┐       │
│  │  扫描间隔: 6h  降级倍数: 2x  归档倍数: 3x     │       │
│  └────────────────────────────────────────────────┘       │
│                                                          │
│  ┌─── 评分权重 ──────────────────────────────────┐       │
│  │  相似度: 40%  优先级: 25%  热度: 15%          │       │
│  │  分类: 10%    紧急度: 10%                      │       │
│  └────────────────────────────────────────────────┘       │
└──────────────────────────────────────────────────────────┘
```

---

## 5. 认证API

**DESIGN-WEB-016** 认证接口设计  
对应 WEB-REQ-013~015。

```
POST /api/v1/auth/login
Request:  { "username": "admin", "password": "xxx" }
Response: { "token": "eyJhbG...", "expires_at": "2026-04-10T21:00:00Z" }
Error:    401 { "error": "用户名或密码错误" }

POST /api/v1/auth/logout
Header:   Authorization: Bearer <token>
Response: 200 { "message": "已登出" }

GET /api/v1/auth/me
Header:   Authorization: Bearer <token>
Response: { "username": "admin", "role": "admin" }
Error:    401 { "error": "token无效或已过期" }
```

**DESIGN-WEB-017** JWT实现  
- 签名算法：HS256
- 签名密钥：服务启动时从 `config.yaml` 读取 `web.jwt_secret`，未配置时自动生成随机密钥（重启后旧token失效）
- token有效期：默认24小时，可通过 `web.token_ttl_hours` 配置
- JWT Claims：`sub`（用户名）、`role`（固定"admin"）、`iat`、`exp`

**DESIGN-WEB-018** 管理员账户配置  
对应 WEB-REQ-015。

```yaml
# config.yaml 新增配置节
web:
  jwt_secret: "${WEB_JWT_SECRET:-am-dashboard-2026-secret-key}"  # 生产环境必须修改
  token_ttl_hours: 24
  admins:
    - username: "admin"
      password_hash: "$2a$10$..."  # bcrypt哈希，支持明文（启动时自动哈希化）
  login_rate_limit: 5  # 每IP每分钟最多登录尝试次数
```

启动时行为：
1. 读取 `web.admins` 列表
2. 对每个管理员，如果 `password_hash` 不以 `$2` 开头（即明文密码），自动用bcrypt哈希后覆盖到配置
3. 至少需要1个管理员，否则服务启动失败
4. 首次启动时如果没有配置文件中的web节，使用默认管理员 `admin/admin123` 并在日志中打印警告

---

## 6. API扩展

**DESIGN-WEB-014** 新增系统信息API  
前端系统页面需要获取配置信息，新增接口：

```
GET /api/v1/system/config
Response:
{
  "version": "v1.0.0",
  "uptime_seconds": 8100,
  "storage": {
    "sqlite_path": "./data/memories.db",
    "vector_provider": "memory",
    "db_size_bytes": 1048576
  },
  "embedding": {
    "provider": "mock",
    "model": "all-MiniLM-L6-v2",
    "dimensions": 384
  },
  "dedup_thresholds": {
    "identity": 0.95,
    "principle": 0.90,
    "knowledge": 0.85,
    "working": 0.70
  },
  "ttl": {
    "scan_interval_hours": 6,
    "degrade_multiplier": 2,
    "archive_multiplier": 3
  },
  "scoring": {
    "similarity_weight": 0.40,
    "priority_weight": 0.25,
    "access_count_weight": 0.15,
    "category_weight": 0.10,
    "urgency_weight": 0.10
  },
  "governance": {
    "compress_threshold": 0.85,
    "max_memories_per_agent": 10000,
    "max_content_length": 10000
  }
}
```

---

## 7. 颜色方案

**DESIGN-WEB-015** 暗色主题配色  
对应 WEB-NFR-003。

| 元素 | 颜色 | 用途 |
|------|------|------|
| 背景 | `#0d1117` | 页面主背景 |
| 卡片背景 | `#161b22` | 卡片/弹窗背景 |
| 边框 | `#30363d` | 分隔线、边框 |
| 主文字 | `#e6edf3` | 标题、正文 |
| 次文字 | `#8b949e` | 辅助说明文字 |
| 强调色 | `#58a6ff` | 链接、选中状态 |
| identity | `#58a6ff` (蓝) | identity标签 |
| principle | `#bc8cff` (紫) | principle标签 |
| knowledge | `#3fb950` (绿) | knowledge标签 |
| working | `#d29922` (橙) | working标签 |
| active | `#3fb950` (绿) | active状态 |
| degraded | `#d29922` (黄) | degraded状态 |
| archived | `#f85149` (红) | archived状态 |
| 危险按钮 | `#f85149` (红) | 删除按钮 |

---

## 8. 需求追溯矩阵

| 设计编号 | 对应需求 |
|----------|----------|
| DESIGN-WEB-001 | WEB-NFR-008, WEB-NFR-006 |
| DESIGN-WEB-002 | WEB-NFR-004 |
| DESIGN-WEB-003 | WEB-REQ-001~003, WEB-REQ-008~009 |
| DESIGN-WEB-004 | WEB-NFR-006 |
| DESIGN-WEB-005 | WEB-REQ-001~004, WEB-REQ-008~010, WEB-REQ-012 |
| DESIGN-WEB-006 | WEB-NFR-003 |
| DESIGN-WEB-007 | WEB-REQ-005, WEB-REQ-006, WEB-REQ-007, WEB-REQ-010 |
| DESIGN-WEB-008 | WEB-REQ-001, WEB-REQ-011 |
| DESIGN-WEB-009 | WEB-REQ-002 |
| DESIGN-WEB-010 | WEB-REQ-003 |
| DESIGN-WEB-011 | WEB-REQ-008 |
| DESIGN-WEB-012 | WEB-REQ-009 |
| DESIGN-WEB-013 | WEB-REQ-012 |
| DESIGN-WEB-014 | WEB-REQ-012 |
| DESIGN-WEB-015 | WEB-NFR-003 |
| DESIGN-WEB-016 | WEB-REQ-013, WEB-REQ-014 |
| DESIGN-WEB-017 | WEB-NFR-011, WEB-REQ-014 |
| DESIGN-WEB-018 | WEB-REQ-015, WEB-NFR-010 |

---

## 9. 实现计划

| 阶段 | 内容 | 预计工作量 |
|------|------|-----------|
| Phase 1 | 项目骨架 + 全局布局 + 路由 + API封装 + 后端embed集成 | 0.5天 |
| Phase 2 | 概览仪表盘（统计卡片 + 图表 + 热度列表） | 0.5天 |
| Phase 3 | 记忆管理（列表 + 搜索 + 过滤 + 详情编辑 + 新建） | 1天 |
| Phase 4 | Agent管理 + 系统信息 + 批量操作 | 0.5天 |
| Phase 5 | 新增系统配置API + 集成测试 | 0.5天 |
| Phase 6 | **登录认证**（JWT + bcrypt + 登录页 + 会话管理 + API中间件改造） | **0.5天** |
