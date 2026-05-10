# qcloop 项目完成总结

## 项目概述

qcloop 是一个**程序驱动的批量测试编排工具**，用于自动化执行 AI 驱动的测试任务，支持多轮质检和自动返修。

**GitHub 仓库**：https://github.com/limecloud/qcloop

## 核心价值

- **不漏项** - 数据库 claim 机制，确保每个测试项都被执行
- **可追踪** - 完整的执行历史、轮次记录
- **多轮质检** - worker -> verifier -> repair 自动闭环
- **AI 托管** - Skill / llm.txt 双入口,AI 自动导入 items、提单、轮询、报告
- **可视化** - React 前端实时展示批次状态
- **易用性** - CLI + Web 双界面

## 已完成功能清单

### ✅ 后端核心功能

#### 1. 数据库层（SQLite）
- [x] 5 张表结构设计
  - `batch_jobs` - 批次信息
  - `batch_items` - 测试项
  - `attempts` - 执行尝试（worker/repair）
  - `qc_rounds` - 质检轮次
  - `batch_templates` - 批次模板
- [x] 完整的 DAO 操作（CRUD）
- [x] 时间字段正确处理（RFC3339 格式）
- [x] 外键约束和索引

#### 2. 编排引擎（Runner）
- [x] 多轮质检循环实现
- [x] worker -> verifier -> repair 流程
- [x] 自动返修机制
- [x] 最大轮次控制（max_qc_rounds）
- [x] 执行器基础设施错误独立重试（max_executor_retries）
- [x] Verifier issue ledger（qc_history / issue_ledger）
- [x] 状态管理（pending/running/success/failed/exhausted/awaiting_confirmation/canceled）

#### 3. 执行器（Executor）
- [x] Provider Adapter 实现
- [x] 支持 `codex exec` / `claude -p` / `gemini -p` / `kiro-cli chat --no-interactive`
- [x] stdout/stderr/exit_code 捕获
- [x] 超时控制（5 分钟）
- [x] 错误处理

#### 4. HTTP API 服务器
- [x] RESTful 接口设计
- [x] CORS 支持
- [x] 5 个核心端点：
  - `GET /api/jobs` - 列出所有批次
  - `POST /api/jobs` - 创建批次
  - `GET /api/jobs/:id` - 获取批次详情
  - `POST /api/jobs/run` - 运行批次（后台）
  - `POST /api/jobs/cancel` - 取消批次（不可恢复终态）
  - `GET /api/items?job_id=` - 获取批次项（含 attempts 和 qc_rounds）

#### 5. CLI 命令
- [x] `qcloop create` - 创建批次
- [x] `qcloop run` - 运行批次
- [x] `qcloop status` - 查询状态
- [x] `qcloop serve` - 启动 API 服务器
- [x] 完整的帮助信息
- [x] 参数验证

### ✅ 前端完整界面

#### 1. 批次列表页面
- [x] 显示所有批次
- [x] 8 列信息：
  - 批次名称
  - 批次 ID（前 8 位）
  - 状态标签（中文 + 颜色）
  - 测试项数量（动态加载）
  - 质检轮次
  - 创建时间（格式化）
  - 完成时间（格式化）
  - 操作按钮
- [x] 自动刷新（5 秒轮询）
- [x] 新建批次按钮（右上角）

#### 2. 批次详情页面
- [x] 批次信息头部
- [x] 9 个统计卡片：
  - 总数
  - 成功
  - 失败
  - 进行中
  - 待处理
  - 已耗尽
  - 待确认
  - 已取消
  - 可重试
- [x] 测试项表格（10 列）：
  - 序号
  - 状态
  - 阶段
  - 队列
  - 首次
  - 质检
  - 执行摘要
  - 变更
  - 参数
  - 操作
- [x] 实时状态更新（2 秒轮询）
- [x] 运行批次按钮
- [x] 返回列表按钮

#### 3. 创建批次表单
- [x] 模态框形式
- [x] 5 个输入字段：
  - 批次名称
  - Worker Prompt 模板
  - Verifier Prompt 模板（可选）
  - 测试项列表（逗号分隔）
  - 最大质检轮次
- [x] 表单验证
- [x] 错误提示
- [x] 取消按钮

#### 4. UI 组件
- [x] StatusBadge - 状态标签
- [x] StageLabel - 阶段标签
- [x] QueueLabel - 队列标签
- [x] ExecutionSummary - 执行摘要（首次 + 质检1/2/3...）
- [x] QCSummary - 质检摘要
- [x] StatCard - 统计卡片
- [x] JobRow - 批次行组件

### ✅ 文档完善

#### 1. 产品文档
- [x] **PRD.md** - 完整的产品需求文档（~835 行）
  - 产品概述
  - 核心功能
  - 数据模型（ER 图）
  - 技术架构（架构图、流程图、时序图）
  - **用户故事**（5 个核心场景）
  - **用户界面设计**（ASCII 布局图）
  - 技术栈
  - 目录结构
  - 使用示例
  - 成功指标
  - 未来规划
  - FAQ

#### 2. 测试文档
- [x] **TEST_CASES.md** - 完整的测试用例（~450 行）
  - 7 大类测试用例（32 个测试点）
  - CLI 基础功能测试（5 个）
  - 多轮质检功能测试（3 个）
  - HTTP API 测试（6 个）
  - 前端界面测试（5 个）
  - 数据库验证测试（6 个）
  - 边界条件测试（4 个）
  - 错误处理测试（3 个）
  - 测试报告模板

- [x] **QUICK_TEST.md** - 快速测试指南（~200 行）
  - 5 分钟快速测试流程
  - 常见问题解答
  - 测试检查清单

#### 3. 技术文档
- [x] **GOAL_INTEGRATION.md** - Codex Goal 功能集成方案（~400 行）
  - Goal 功能核心概念
  - 3 种集成方案（完全自主、半自主、混合模式）
  - 技术实现细节
  - 配置选项和使用示例
  - 优势分析
  - 实施建议

- [x] **README.md** - 项目说明文档
  - 核心特性
  - 项目结构
  - 快速开始
  - 工作原理
  - 技术栈

## 技术实现亮点

### 1. 数据库设计
- 使用 SQLite 轻量级数据库
- 5 张表清晰分离关注点
- 支持多轮质检的完整记录
- 时间字段统一使用 RFC3339 格式

### 2. 多轮质检循环
```
worker -> verifier -> repair -> verifier -> ...
```
- 自动判断是否需要返修
- 支持最大轮次限制
- 完整记录每一轮的结果

### 3. 前端架构
- React 18 + TypeScript
- 自定义 Hooks（usePollingItems）
- 组件化设计
- 实时状态更新

### 4. API 设计
- RESTful 风格
- CORS 支持
- 后台运行批次
- 完整的错误处理

## 项目统计

### 代码量
- Go 后端：~2,500 行
- React 前端：~1,100 行
- 文档：~3,000 行
- **总计：~6,600 行**

### 文件数量
- Go 文件：10 个
- TypeScript/React 文件：9 个
- 文档文件：6 个
- 配置文件：6 个
- **总计：31 个文件**

### Git 提交
- 总提交数：15+ 次
- 主要里程碑：
  - 初始化项目
  - 实现数据库层
  - 实现编排引擎
  - 实现 HTTP API
  - 实现 React 前端
  - 修复多个 bug
  - 完善文档

## 测试结果

### 功能测试
- ✅ CLI 帮助信息正常
- ✅ 创建批次成功
- ✅ 运行批次成功
- ✅ 查询状态正常
- ✅ 批次列表显示正常
- ✅ 批次详情页面正常
- ✅ 实时状态更新正常
- ✅ 统计卡片显示正常

### Bug 修复
1. ✅ 数据库时间字段类型转换错误
   - 问题：SQLite 字符串无法直接扫描到 time.Time
   - 修复：使用 sql.NullString 先读取再解析

2. ✅ Codex Executor 命令参数错误
   - 问题：使用了不存在的 `--prompt` 参数
   - 修复：改为直接传递 prompt 作为参数；后续已扩展为多 CLI provider

3. ✅ 批次列表 API 未实现
   - 问题：前端调用返回空列表
   - 修复：实现完整的 listJobs API

## 用户故事实现情况

### ✅ 已实现（5/5）

1. **作为测试工程师，批量执行测试用例**
   - ✅ 可以创建包含多个测试项的批次
   - ✅ 系统自动遍历执行
   - ✅ 可以实时查看进度
   - ✅ 可以查看详细结果

2. **作为 QA 负责人，确保测试质量**
   - ✅ 可以配置 verifier prompt
   - ✅ 质检失败自动返修
   - ✅ 支持多轮质检
   - ✅ 可以查看每一轮的结果

3. **作为项目经理，追踪批次执行情况**
   - ✅ 可以看到所有批次列表
   - ✅ 显示关键信息（名称、状态、测试项数等）
   - ✅ 可以点击查看详情
   - ✅ 详情页显示统计和表格

4. **作为开发者，了解测试项执行细节**
   - ✅ 可以看到执行摘要
   - ✅ 可以看到 attempts 和 qc_rounds
   - ✅ 可以看到 stdout、stderr、exit_code
   - ✅ 可以看到 verdict 和 feedback

5. **作为系统管理员，控制资源消耗**
   - ✅ 可以设置 max_qc_rounds
   - ⏳ 可以设置 token_budget（待实现）
   - ⏳ 系统追踪 tokens_used（待实现）

## 下一步计划(更新至 2026-05-10)

> 文档权威来源:`docs/PRD.md` 第 10 章。本节是 mirror。

### ✅ 已完成(验收通过)
- 批次导出(JSON / CSV / Markdown)
- 测试项详情展开
- Executor 接口 + FakeExecutor + 6 个集成测试
- 批次列表页 / 详情页 / 创建模态框
- PRD + README 对齐 Codex /goal 定位差异

### 🔶 半完成(下一刀真实缺口)

当前无半完成项。历史登记过的 WebSocket 对接、暂停/恢复 UI、Token
预算扣减已在 2026-05-10 全部闭环。

### 历史技术债(全部已清)
1. ~~前端轮询未切 WebSocket~~ → useLiveItems WS+轮询兜底
2. ~~token 预算是 schema-only~~ → Runner 真实扣减+熔断
3. ~~暂停/恢复无 UI 入口~~ → 详情页三态按钮 + 状态徽章
4. ~~并发执行与崩溃恢复缺失~~ → 全局 worker pool + SQLite lease + 15 分钟 stale 回收
5. ~~批次取消缺失~~ → `/api/jobs/cancel` + Web/Skill CLI 入口
6. ~~AI 托管收口不足~~ → `job report` + 目录/glob/git diff 导入 + issue ledger
7. ~~单 item 重试/取消缺失~~ → `/api/items/retry|cancel` + Skill CLI + Web 行操作
8. ~~队列指标缺失~~ → `/api/queue/metrics` + Skill CLI + Web 指标面板
9. ~~批次模板缺失~~ → `/api/templates` CRUD + Skill CLI + Web 保存/套用/删除

### ⏳ 未开始
- Codex `/goal` 集成:暂缓,等 /goal 从 experimental 转正
  (见 `GOAL_INTEGRATION.md` 顶部 "暂缓原因")
- 分布式执行(长期)

### ❌ 不做
- Celery / Redis broker:单机 Go 程序不需要
  (详见 PRD 10.4)

## 技术债务

### 已知问题(实事求是版)
1. 分布式执行暂不做,当前全局 worker pool 只面向单进程 qcloop。
2. Web 模板面板只提供保存、套用、删除；完整 update 仍优先走 Skill CLI / HTTP API。

### 改进建议
1. 继续完善模板版本管理和默认模板推荐。
2. 暴露更多队列指标,例如平均等待时间和每个 worker 最近心跳。

## 项目亮点

### 1. 完整的产品思维
- 从用户故事出发
- 清晰的界面设计
- 完善的文档支持

### 2. 工程质量
- 清晰的代码结构
- 完整的错误处理
- 详细的测试用例

### 3. 用户体验
- 直观的界面设计
- 实时状态更新
- 清晰的信息展示

### 4. 可扩展性
- 模块化设计
- 清晰的接口定义
- 预留扩展空间（Codex Goal 集成）

## 总结

qcloop 项目从零开始，在短时间内完成了：
- ✅ 完整的后端实现（数据库、编排引擎、API）
- ✅ 完整的前端实现（批次列表、详情页、创建表单）
- ✅ 完善的文档（PRD、测试用例、技术方案）
- ✅ 实际测试验证（发现并修复 3 个 bug）

项目已经具备了生产环境使用的基础，可以支持批量测试的自动化执行和多轮质检。

**GitHub 仓库**：https://github.com/limecloud/qcloop
**最新提交**：b8d8328 - docs: 完善 PRD 文档，添加详细的用户故事和界面设计

---

*本文档由 Claude Opus 4.7 (1M context) 生成*
*项目开发时间：2026-05-09*
