# AI Agent 通用记忆框架 - 需求文档

> 项目代号：agent-memory  
> 需求来源：洪岩  
> 创建日期：2026-04-09  
> 文档版本：v1.1  
> 更新日期：2026-04-10  
> 更新内容：新增 Agent 隔离(X-User-Id)、Web Dashboard、JWT 认证需求

---

## 1. 项目概述

### 1.1 背景

当前AI Agent使用的记忆系统（Mem0）存在严重缺陷：无分类、无过期机制、无自动去重、检索不准、无优先级、无来源追溯、无关联。经过对751条记忆的清理（保留133条，删除82%），暴露了记忆系统亟需结构化治理的需求。

### 1.2 目标

设计并实现一个通用的AI Agent记忆框架，现阶段服务于OpenClaw Agent团队，远期可扩展为适用于各种AI Agent的通用能力。

### 1.3 约束

- 现阶段优先服务自有Agent团队（OpenClaw生态）
- 须兼容现有Mem0存储后端（渐进式替换，非一次性重构）
- 框架须支持多用户（userId隔离）
- 框架须支持多Agent隔离（同一用户下的不同Agent各有独立记忆空间，同时支持共享记忆）
- 框架本身须为独立服务，Agent通过API调用

---

## 2. 功能性需求

### 2.1 记忆分类体系

**REQ-001** 记忆分类  
记忆须支持四个层级分类：
- L1 核心身份（identity）：名字、userId、时区等基本不变的信息
- L2 原则与偏好（principle）：工作原则、沟通偏好等极少变化的信息
- L3 上下文知识（knowledge）：团队结构、技术栈、基础设施等缓慢变化的信息
- L4 工作记忆（working）：当前任务状态、临时决策、操作记录等快速变化的信息

**REQ-002** 记忆分类标签  
每条记忆存储时须指定category字段，取值范围为：identity / principle / knowledge / working。若未指定，框架须根据内容自动推断分类。

### 2.2 记忆元数据

**REQ-003** 记忆元数据模型  
每条记忆除内容外，须携带以下元数据：
- agent_id：所属Agent标识（如"m10s"、"devforge"、"qbot"等），标识该记忆由哪个Agent创建
- visibility：可见性（private / team / user）
  - **private**：仅创建该记忆的Agent可读写（如Agent自身的任务状态、临时笔记）
  - **team**：同团队（同一user_id）下所有Agent可读，仅创建者可写（如项目规范、架构决策）
  - **user**：同用户下所有Agent可读写（如用户身份、沟通偏好、核心原则）
- priority：优先级（1-5，1为最高）
- source：来源追溯（对话ID、心跳触发、手动录入等）
- confidence：确信度（0.0-1.0，用户明确告知为1.0，推断得出按可信度衰减）
- ttl：生存时间（permanent / year / month / week / session）
- tags：标签列表（自由文本标签，如"安全"、"研发流程"）
- created_at：创建时间
- updated_at：最后更新时间
- last_accessed：最后被检索/使用的时间
- access_count：被检索使用次数（热度指标）
- version：版本号（每次更新自增）

**REQ-004** 元数据自动填充  
框架在存储记忆时，须自动填充以下元数据（无需调用方指定）：
- created_at、updated_at（自动取当前时间）
- access_count（初始为0）
- version（初始为1）
- agent_id（从API调用方的API Key或Header中自动识别）
- 若调用方未指定visibility、priority、ttl、confidence，须提供合理默认值（visibility=private、priority=3、ttl=month、confidence=0.8）

### 2.3 自动去重

**REQ-005** 语义去重  
新记忆存入前，须与该Agent可见范围内的已有记忆进行语义相似度匹配（即去重范围 = private（仅自己） + team（同团队） + user（同用户）中该Agent有权访问的记忆）。若最高相似度超过阈值（默认85%），则不新增，而是更新已有记忆（合并内容、提升confidence、刷新updated_at）。

**REQ-006** 去重阈值可配置  
语义去重阈值须支持按category配置不同值。建议默认值：
- identity：95%（身份信息高度精确）
- principle：90%（原则表述可以略有不同但语义一致）
- knowledge：85%（知识类信息允许一定冗余）
- working：70%（工作记忆允许较多相关但不同的条目）

**REQ-007** 去重结果通知  
当去重触发合并时，框架须返回合并结果，包含：被合并的原记忆ID、合并后的新内容、变更的元数据字段。

### 2.4 TTL过期与衰减

**REQ-008** TTL自动过期  
框架须支持记忆的TTL过期机制：
- session：会话结束时删除
- week：7天未被访问自动标记为过期
- month：30天未被访问自动标记为过期
- year：365天未被访问自动标记为过期
- permanent：永不过期

**REQ-009** 过期处理策略  
过期的记忆不立即物理删除，而是进入"降级"状态：
- 降级记忆在搜索结果中排到末尾
- 降级超过TTL的2倍后，进入"归档"状态
- 归档记忆仅在全量列表中可见，不出现在搜索结果中
- 归档超过TTL的3倍后，可被物理删除（需用户确认或配置自动清理）

**REQ-010** 热度重置TTL  
当记忆被成功检索并使用（access_count增加）时，其TTL计时器须重置。

### 2.5 检索增强

**REQ-011** 语义检索  
检索须基于向量嵌入（embedding）进行语义匹配，而非简单的文本关键词匹配。

**REQ-012** 多维度排序  
检索结果须综合以下因子排序：
- 语义相似度（主要因子，权重40%）
- priority优先级（权重25%）
- access_count热度（权重15%）
- category层级（identity > principle > knowledge > working，权重10%）
- ttl紧迫度（即将过期的working记忆适当提升，权重10%）

**REQ-013** 分类过滤  
检索须支持按category过滤，仅返回指定分类的记忆。

**REQ-014** 分页检索  
检索结果须支持分页（page_size和page_token），避免一次返回过多数据。

### 2.6 记忆治理

**REQ-015** 批量压缩  
框架须提供批量压缩能力：扫描指定用户的所有记忆，识别语义重复组，每组仅保留priority最高、内容最完整的一条，其余合并后删除。

**REQ-016** 自动治理报告  
框架须支持定期生成记忆健康报告，包含：
- 各category记忆数量及占比
- 过期/降级/归档记忆数量
- 高热度记忆Top 10
- 零访问记忆列表（建议清理）
- 与上次报告的变化量

**REQ-017** 手动治理操作  
框架须支持以下手动治理操作：
- 按category批量删除
- 按时间范围批量删除
- 按tag批量操作（删除/修改ttl/修改priority）
- 单条记忆的编辑、删除、归档
- 全量导出（JSON格式）

### 2.7 写入门控

**REQ-018** 写入评估  
每次写入前，框架须评估该内容是否值得长期记忆：
- 若内容为一次性操作记录（如"已执行cargo run"），建议ttl=week
- 若内容为重复信息（语义去重命中），执行合并而非新增
- 若内容为临时状态（如"当前正在调研XX"），建议ttl=month
- 若内容为核心原则或身份信息，建议ttl=permanent

**REQ-019** 写入建议返回  
写入评估结果须返回建议的category、priority、ttl，调用方可选择采纳或覆盖。

### 2.8 多Agent隔离

**REQ-020** Agent身份识别  
每个Agent通过API Key认证时，须携带其agent_id。框架根据agent_id自动确定记忆的读写范围。Agent身份信息可在配置中注册：

```yaml
agents:
 - id: m10s
   name: OpenClaw-M10S
   team: default
   api_key: ${M10S_API_KEY}
 - id: devforge
   name: OpenClaw-M10S-dev
   team: default
   api_key: ${DEVFORGE_API_KEY}
 - id: qbot
   name: OpenClaw-M10S-QA
   team: default
   api_key: ${QBOT_API_KEY}
 - id: sage
   name: Sage
   team: default
   api_key: ${SAGE_API_KEY}
 - id: clara
   name: Clara
   team: default
   api_key: ${CLARA_API_KEY}
```

**REQ-021** 记忆可见性规则  
记忆的读写权限须严格按visibility执行：

| visibility | 读权限 | 写权限 | 典型场景 |
|-----------|--------|--------|----------|
| private | 仅创建者Agent | 仅创建者Agent | Agent自身任务状态、临时笔记 |
| team | 同team下所有Agent | 仅创建者Agent | 项目规范、架构决策、共享知识 |
| user | 同user_id下所有Agent | 同user_id下所有Agent | 用户身份、沟通偏好、核心原则 |

- Agent A尝试读取Agent B的private记忆时，须返回403 Forbidden
- Agent A读取team记忆时，仅能读取同team内的，跨team不可见
- user级记忆任何Agent均可读写（但不可跨user）

**REQ-022** Agent注册与管理  
框架须支持Agent的注册、查询和删除：
- `POST /agents` - 注册新Agent（id、name、team、api_key）
- `GET /agents` - 列出已注册Agent
- `DELETE /agents/{id}` - 注销Agent（其private记忆须保留或按策略清理）

### 2.9 Agent集成接口

**REQ-023** RESTful API  
框架须提供RESTful API，支持以下操作：
- `POST /memories` - 写入记忆
- `GET /memories/search` - 语义检索（自动按visibility过滤）
- `GET /memories` - 列出记忆（支持过滤和分页，自动按visibility过滤）
- `GET /memories/{id}` - 获取单条（需有读权限）
- `PUT /memories/{id}` - 更新记忆（需有写权限）
- `DELETE /memories/{id}` - 删除记忆（需有写权限）
- `POST /memories/compress` - 批量压缩
- `GET /memories/report` - 健康报告
- `POST /memories/batch` - 批量操作

**REQ-024** OpenClaw插件接口  
框架须提供OpenClaw插件接口（兼容OpenClaw的tool/plugin协议），使Agent可直接通过工具调用访问记忆框架。插件须自动从Agent身份识别中获取agent_id，无需Agent手动指定。

**REQ-025** 多用户隔离  
所有API须支持userId隔离，不同用户的记忆互不可见（即使agent_id相同）。

**REQ-026** 团队（team）概念  
同一user_id下可划分多个team。默认所有Agent属于"default"团队。未来可支持自定义team（如"研发组"、"测试组"），team级记忆仅在同team内可见。

### 2.10 数据迁移

**REQ-027** Mem0数据导入  
框架须支持从现有Mem0存储导入已有记忆数据。导入时须：
- 自动分析内容推断category和visibility
- 用户身份/偏好类记忆导入为user级
- 项目规范/架构决策类记忆导入为team级
- 操作记录/任务状态类记忆导入为指定Agent的private级
- 自动评估priority和ttl
- 执行自动去重
- 生成迁移报告

### 2.9 数据迁移

**REQ-027** Mem0数据导入  
（见2.10节）

---

## 3. 非功能性需求

### 3.1 性能

**NFR-001** 检索延迟  
语义检索API（含embedding计算）的P95延迟须不超过500ms。

**NFR-002** 写入延迟  
单条记忆写入API的P95延迟须不超过200ms。

**NFR-003** 并发支持  
API须支持至少10个并发请求，无明显性能退化。

### 3.2 可靠性

**NFR-004** 数据持久化  
记忆数据须持久化存储，服务重启后数据不丢失。

**NFR-005** 写入原子性  
记忆的写入（含元数据和embedding）须为原子操作，不允许出现数据不一致。

**NFR-006** 向量一致性  
记忆内容更新后，对应的向量嵌入须同步更新，不允许内容与向量不一致。

### 3.3 可用性

**NFR-007** 独立部署  
框架须可独立部署（Docker容器），不依赖OpenClaw主进程运行。

**NFR-008** 健康检查  
须提供健康检查端点（GET /health），返回服务状态和存储连接状态。

**NFR-009** 配置外置  
所有可配置项（去重阈值、TTL策略、API端口等）须通过配置文件或环境变量管理，不支持硬编码。

### 3.4 安全

**NFR-010** 认证与授权  
API须支持API Key认证，不同用户使用不同Key访问各自数据。

**NFR-011** 数据隔离  
用户间记忆数据须严格隔离，API不可越权访问其他用户数据。同一用户内，Agent间按visibility规则隔离，private记忆不可被其他Agent读取。

### 3.5 可维护性

**NFR-012** 日志  
须记录关键操作日志（写入、删除、压缩、过期），日志级别可配置。

**NFR-013** 监控指标  
须暴露关键指标：总记忆数、各category数量、API调用量（QPS）、P95延迟。

**NFR-014** 技术栈  
后端使用Golang（高性能、跨平台、适合独立服务），向量数据库优先使用Qdrant（轻量、易部署、原生Go SDK），Embedding模型优先使用本地部署模型（通过ONNX Runtime加载sentence-transformers模型）以保护数据隐私。

### 3.6 可扩展性

**NFR-015** Embedding模型可替换  
Embedding模型须可配置替换，支持本地模型和远程API模型（如OpenAI embeddings）。

**NFR-016** 向量数据库可替换  
向量数据库须通过抽象层支持替换（Qdrant、Milvus、Chroma等）。

**NFR-017** 通用化预留  
数据模型和API设计须考虑通用化，为未来支持非OpenClaw的AI Agent预留扩展点（如不同Agent框架的适配层）。

---

## 4. 验收标准

| 编号 | 验收条件 |
|------|----------|
| REQ-001~002 | 存入记忆时可指定category，未指定时自动推断，四个层级均可正确存储和检索 |
| REQ-003~004 | 记忆包含完整元数据，未指定的字段有合理默认值 |
| REQ-005~007 | 相似度超阈值的记忆自动合并，阈值可按category配置，合并结果有明确返回 |
| REQ-008~010 | TTL过期机制正确运行，过期记忆降级→归档→删除，被访问时TTL重置 |
| REQ-011~014 | 语义检索准确，多维度排序合理，支持分类过滤和分页 |
| REQ-015~017 | 批量压缩可执行，健康报告可生成，手动治理操作完整 |
| REQ-018~019 | 写入时自动评估并返回建议 |
| REQ-020~022 | 多Agent身份注册、可见性规则（private/team/user）正确执行 |
| REQ-023~024 | RESTful API完整可用，支持OpenClaw插件 |
| REQ-025~026 | 多用户隔离正确，team概念支持 |
| REQ-027 | Mem0数据可导入，导入时自动推断visibility并执行自动去重 |
| NFR-001~002 | 检索P95<500ms，写入P95<200ms |
| NFR-004~006 | 数据持久化、原子写入、向量一致性 |
| NFR-007~009 | Docker独立部署、健康检查、配置外置 |
| NFR-010~011 | API Key认证、多用户+多Agent数据隔离 |
| NFR-012~014 | 日志、监控指标、Golang+Qdrant技术栈 |
| NFR-015~017 | Embedding模型可替换、向量数据库可替换、通用化预留 |
