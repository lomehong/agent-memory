# Agent-Memory Web Dashboard - 需求文档

> 项目代号：agent-memory-web  
> 需求来源：洪岩  
> 创建日期：2026-04-09  
> 文档版本：v1.1  
> 依赖：agent-memory v1.1 REST API (含 X-User-Id Agent 隔离)  
> 变更记录：v1.1 新增 WEB-REQ-013~015 登录认证需求，修订 DESIGN-WEB-003

---

## 1. 项目概述

### 1.1 背景

agent-memory后端服务已实现完整的记忆管理能力（CRUD、语义检索、自动分类、去重、TTL生命周期、健康报告等），但当前仅提供REST API接口，缺少可视化操作界面。管理和检查记忆只能通过curl或直接查询数据库，效率低下且不直观。

### 1.2 目标

设计并实现一个轻量级Web管理面板（Dashboard），提供：
- 记忆的浏览、搜索、创建、编辑、删除
- 记忆健康状态的直观展示
- Agent管理和API Key管理
- 系统配置的可视化查看

### 1.3 约束

- 前端为纯静态页面，由agent-memory后端服务直接托管（嵌入Go二进制，无额外部署）
- 不引入重型前端框架（React/Vue），使用轻量方案减少构建复杂度
- 支持中英文界面
- 响应式设计，支持桌面和移动端浏览器
- 所有数据操作通过后端REST API完成，前端不直接访问数据库

---

## 2. 功能性需求

### 2.0 用户认证

**WEB-REQ-013** 登录页面  
Web Dashboard 必须通过账号密码登录后才能访问，未登录时所有页面重定向到登录页：
- 登录页提供用户名和密码输入框
- 支持记住登录状态（7天有效期，使用 localStorage 存储会话token）
- 登录失败时显示错误提示（不暴露具体原因，统一提示"用户名或密码错误"）
- 登录成功后跳转到概览仪表盘
- 页面右上角显示当前登录用户名，提供登出按钮

**WEB-REQ-014** 会话管理  
登录后通过会话token进行API认证：
- 登录成功后服务端返回JWT token，前端存储在 localStorage
- 所有API请求自动附带 `Authorization: Bearer <token>` header
- token过期后自动跳转到登录页
- 登出时清除本地token并跳转到登录页

**WEB-REQ-015** 管理员账户  
Dashboard使用独立的管理员账户体系，与Agent API Key体系分离：
- 初始管理员账户在服务首次启动时创建（用户名和密码通过配置文件指定）
- 配置文件中支持多管理员账户
- 密码使用bcrypt哈希存储
- 管理员账户用于登录Dashboard，Agent API Key仍用于程序化API调用

### 2.1 记忆管理

**WEB-REQ-001** 记忆列表页  
提供记忆列表页面，支持：
- 按category过滤（identity / principle / knowledge / working）
- 按status过滤（active / degraded / archived）
- 按visibility过滤（private / team / user）
- 按关键词搜索（调用后端语义检索API）
- 分页浏览（每页20条，可调整）
- 每条记忆显示：内容摘要（前80字）、category标签、priority、visibility、status、access_count、created_at
- 点击记忆可展开查看完整内容

**WEB-REQ-002** 记忆详情与编辑  
提供记忆详情弹窗/页面，展示完整元数据，并支持：
- 编辑content、category、priority、visibility、ttl、tags、status
- 删除记忆（需二次确认）
- 查看记忆的version历史（merged_from字段）
- 查看记忆的最后访问时间和访问次数

**WEB-REQ-003** 新建记忆  
提供新建记忆表单，支持：
- 输入content（必填，textarea）
- 可选指定category、priority、visibility、ttl、tags
- 未指定时显示系统推荐的自动推断结果（DESIGN-007）
- 提交后显示写入建议（category、visibility、priority、ttl推荐值，以及去重检测结果）

### 2.2 记忆搜索

**WEB-REQ-004** 语义搜索  
提供语义搜索功能：
- 搜索框输入自然语言查询
- 实时调用 `GET /api/v1/memories/search` 接口
- 结果按综合评分降序排列
- 每条结果显示：内容、category、score（百分比+进度条）、priority
- 支持按category过滤搜索结果

### 2.3 健康仪表盘

**WEB-REQ-005** 概览仪表盘  
Dashboard首页展示记忆系统健康状态概览：
- 总记忆数（大数字卡片）
- 各category记忆数量（环形图或柱状图）
- 各status记忆数量（active / degraded / archived，颜色区分）
- 最近7天新增/更新趋势（折线图）

**WEB-REQ-006** 健康报告  
展示完整的记忆健康报告（调用 `GET /api/v1/memories/report`）：
- 高频访问记忆Top 10（含access_count和content摘要）
- 零访问记忆列表（从未被检索使用，建议清理）
- 陈旧记忆列表（30天以上未访问的非permanent记忆）
- 一键清理建议：零访问的working记忆批量删除

**WEB-REQ-007** 分类分布  
以可视化方式展示四类记忆的分布：
- 各category数量和占比
- 各category的平均priority
- 各category的平均access_count（热度）

### 2.4 Agent管理

**WEB-REQ-008** Agent列表  
展示已注册的Agent列表：
- Agent ID、Name、Team、创建时间
- API Key（脱敏显示，可切换显示/隐藏）
- 快速操作：删除Agent

**WEB-REQ-009** 新建Agent  
提供Agent注册表单：
- Name（必填）
- Team（默认default）
- API Key（自动生成，可手动修改）
- 创建成功后显示API Key（仅一次）

### 2.5 批量操作

**WEB-REQ-010** 批量压缩  
提供一键批量压缩按钮（调用 `POST /api/v1/memories/compress`）：
- 执行前显示预估影响（当前working记忆数量）
- 执行后显示结果（merged数量、archived数量、errors数量）

**WEB-REQ-011** 批量删除  
支持批量选择记忆后执行批量删除：
- 列表页每行增加checkbox
- 选中后显示批量操作栏（删除按钮）
- 删除前二次确认

### 2.6 系统信息

**WEB-REQ-012** 系统状态  
展示系统运行状态：
- 服务版本和启动时间
- 存储状态（SQLite文件大小、向量存储类型）
- Embedding提供商和维度
- 配置概览（去重阈值、TTL策略、治理参数等）

---

## 3. 非功能性需求

### 3.1 安全

**WEB-NFR-010** 密码安全  
- 密码使用bcrypt哈希存储，不可逆
- 登录接口限流（同一IP 5次/分钟，超过锁定1分钟）
- JWT token有效期默认24小时，可配置

**WEB-NFR-011** 会话安全  
- token使用HMAC-SHA256签名，密钥由服务端生成
- 登出时服务端提供token失效接口（可选，依赖token黑名单）
- 前端不存储密码，仅存储token

### 3.2 性能

**WEB-NFR-001** 页面加载  
Dashboard页面首次加载时间不超过2秒（含API调用）。

**WEB-NFR-002** 交互响应  
搜索、过滤、翻页等操作响应时间不超过1秒。

### 3.2 用户体验

**WEB-NFR-003** 视觉设计  
- 使用暗色主题（与OpenClaw风格一致）
- category使用颜色编码：identity=蓝色、principle=紫色、knowledge=绿色、working=橙色
- status使用图标标识：active=🟢、degraded=🟡、archived=🔴
- priority使用星级或数字显示

**WEB-NFR-004** 无刷新交互  
列表翻页、搜索、过滤等操作不刷新整个页面（使用fetch API局部更新DOM）。

**WEB-NFR-005** 错误处理  
API调用失败时显示友好的错误提示，不使用浏览器alert。

### 3.3 技术约束

**WEB-NFR-006** 零构建依赖  
前端为纯HTML/CSS/JS文件，不依赖npm/webpack/vite等构建工具。使用原生JavaScript和CSS，可直接嵌入Go二进制。

**WEB-NFR-007** 图表库  
使用轻量级图表库（Chart.js，CDN引入），用于概览仪表盘的可视化图表。

**WEB-NFR-008** 后端托管  
所有前端静态文件通过Go的 `embed.FS` 嵌入二进制，由 `http.FileServer` 托管在 `/` 路径下。API路径 `/api/v1/` 保持不变。

---

## 4. 页面结构

| 路径 | 页面 | 主要功能 |
|------|------|----------|
| `/` | 概览仪表盘 | 健康概览、分类分布、趋势图 |
| `/memories` | 记忆列表 | 浏览、搜索、过滤、批量操作 |
| `/agents` | Agent管理 | Agent列表、注册、API Key管理 |
| `/system` | 系统信息 | 服务状态、配置概览 |

导航栏固定在左侧或顶部，包含以上四个页面的入口，以及当前认证的Agent信息显示。

---

## 5. 验收标准

| 编号 | 验收条件 |
|------|----------|
| WEB-REQ-001 | 记忆列表可按category/status/visibility过滤，支持分页和关键词搜索 |
| WEB-REQ-002 | 记忆可查看详情、编辑字段、删除（二次确认） |
| WEB-REQ-003 | 新建记忆时可输入内容，提交后显示系统推荐值和去重结果 |
| WEB-REQ-004 | 语义搜索返回结果按score排序，显示score百分比 |
| WEB-REQ-005 | 仪表盘展示总记忆数、分类分布、状态分布 |
| WEB-REQ-006 | 健康报告展示高频/零访问/陈旧记忆列表 |
| WEB-REQ-007 | 分类分布以图表方式可视化展示 |
| WEB-REQ-008 | Agent列表展示ID、Name、Team，API Key可切换显隐 |
| WEB-REQ-009 | 可注册新Agent，自动生成API Key |
| WEB-REQ-010 | 一键批量压缩，显示执行结果 |
| WEB-REQ-011 | 支持checkbox多选+批量删除 |
| WEB-REQ-012 | 系统页面展示服务状态和配置信息 |
| WEB-NFR-006 | 纯HTML/CSS/JS，零构建依赖，嵌入Go二进制 |
| WEB-REQ-013 | 未登录时显示登录页，登录成功后跳转仪表盘 |
| WEB-REQ-014 | 登录后所有API请求自动附带Bearer token，token过期自动跳转登录页 |
| WEB-REQ-015 | 管理员账户通过配置文件管理，密码bcrypt哈希存储 |
| WEB-NFR-010 | 密码bcrypt哈希存储，登录接口限流 |
| WEB-NFR-011 | JWT token HMAC签名，前端仅存储token |
| WEB-NFR-008 | 访问 `http://host:port/` 直接打开Dashboard（登录后） |
