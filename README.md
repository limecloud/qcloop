# qcloop

<div align="center">

**程序驱动的 AI 批量测试编排工具**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=flat&logo=react)](https://reactjs.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

[功能特性](#功能特性) • [快速开始](#快速开始) • [使用指南](#使用指南) • [架构设计](#架构设计) • [文档](#文档)

> **AI agent 使用者请读 [llms-full.txt](./llms-full.txt)**(导航索引见 [llms.txt](./llms.txt))。
> qcloop 的设计意图之一就是让 AI agent 通过 HTTP API 自动提单、人类只在面板监管,不要手动在 UI 里填表。

</div>

---

## 📖 项目简介

qcloop 是一个**批量测试编排工具**，专为 AI 驱动的测试场景设计。它通过程序化的方式遍历执行测试项，支持多轮质检和自动返修，确保测试不漏项、可追踪、高质量。

### 为什么需要 qcloop？

**问题**：手动让 AI 执行批量测试时，容易出现：
- ❌ 遗漏测试项
- ❌ 重复执行
- ❌ 中断后难以恢复
- ❌ 缺少质检机制
- ❌ 结果难以追踪

**解决方案**：qcloop 提供：
- ✅ 数据库驱动，确保不漏项
- ✅ 自动遍历，无需人工干预
- ✅ 多轮质检，自动返修
- ✅ 完整记录，可追溯
- ✅ Web 界面，实时监控

### 不只是“agent 检查”

qcloop 底层确实可以驱动 Codex、Claude、Gemini 等 agent 完成检查；价值不在“让某个 agent 检查一次”，而在把 agent 检查工程化成可复用的 QA loop。

普通脚本 prompt 适合一次性自检，qcloop 解决的是批量、可观察、可复现、可审计的质检编排：

- **批量化**：把一组同质任务拆成结构化 item，逐项执行、逐项质检，不靠人工记忆维护进度。
- **状态机**：每个 item 有 pending/running/success/failed/exhausted 等明确状态，重跑时会重置本轮可见状态并保留历史。
- **证据留痕**：完整保存参数、worker 输出、stderr、verifier verdict、feedback、tokens 和轮次记录。
- **失败返修**：verifier 不通过时自动进入 repair，再继续质检，直到通过或达到 `max_qc_rounds`。
- **可视化监管**：Web 面板展示队列、质检轮次、执行摘要、统计数字和展开明细，人类只需要监管异常。
- **模型可插拔**：qcloop 是编排层，不绑定某个模型；Codex/Claude 只是 worker/verifier 的实现选择。

类比来说，`agent 检查` 就像“能跑测试”；qcloop 更像给 AI agent 任务加上 CI、测试报告、失败重跑和质量台账。任务粒度小不是缺点，而是为了方便并行、定位失败和统计质量。

#### 架构对比

```mermaid
flowchart LR
  subgraph Script["脚本 prompt 自检"]
    S1["一次性 prompt"]
    S2["单个 agent 执行/自评"]
    S3["终端输出"]
    S4["人工判断是否可信"]
    S1 --> S2 --> S3 --> S4
  end

  subgraph QCLoop["qcloop QA loop"]
    Q1["结构化测试项<br/>items"]
    Q2["Runner 状态机<br/>pending/running/success/failed/exhausted"]
    Q3["Worker agent<br/>执行任务"]
    Q4["Verifier agent<br/>独立质检"]
    Q5{"是否通过?"}
    Q6["Repair agent<br/>按 feedback 返修"]
    Q7["Evidence Store<br/>attempts/qc_rounds/tokens/stderr"]
    Q8["Web 面板<br/>统计/轮次/重跑/展开明细"]

    Q1 --> Q2 --> Q3 --> Q4 --> Q5
    Q5 -- "通过" --> Q7
    Q5 -- "不通过且未耗尽" --> Q6 --> Q3
    Q5 -- "达到 max_qc_rounds" --> Q7
    Q7 --> Q8
    Q2 --> Q8
  end
```

图里的关键差异是：脚本 prompt 把“检查”留在一次终端输出里；qcloop 把检查拆成可编排的状态机、独立 verifier、失败返修、证据存储和可视化监管。

#### 产品界面预览

下面是 Playwright 从本地 qcloop 面板截取的真实详情页：可以直接看到批次状态、统计数字、分页、每个 item 的状态机、质检轮次标签和参数入口。

![qcloop dashboard showing batch status, QC rounds and execution evidence](docs/images/qcloop-dashboard.png)

### 与 Codex `/goal` 的区别

Codex CLI 0.128.0 新增的 `/goal` 是官方版 Ralph loop,和 qcloop 解决的是类似问题,但设计哲学不同:

| | Codex `/goal` | qcloop |
|---|---|---|
| 停止条件 | AI 自评 + token budget(软) | `max_qc_rounds`(硬) |
| 判定主体 | 同一 thread 内 AI 自审 | 独立 verifier(外部判断) |
| 收敛保证 | 概率性 | 确定性 |
| 轮次可审计 | 黑盒 | 每轮落库 |
| API 稳定性 | experimental(文档未收录) | 稳定可用 |
| 最适场景 | 单目标探索 | 批量同质任务 |

**一句话**:单目标探索用 Goal,批量任务用 qcloop。qcloop 也支持 `--execution-mode goal_assisted` 把每条 prompt 包装成 Goal 风格,兼得两者优势。详见 [PRD 1.4 节](docs/PRD.md#14-设计哲学为什么选择-程序兜底-而不是-ai-自主)。

## 🎯 功能特性

### 核心功能

- **批量执行** - 一次创建，自动遍历所有测试项
- **多轮质检** - worker → verifier → repair 自动闭环
- **不漏项** - 数据库 claim 机制，确保每个测试项都被执行
- **可追踪** - 完整记录执行历史、质检结果、返修过程
- **实时监控** - Web 界面实时展示批次状态和执行进度
- **双界面** - CLI 命令 + Web 界面，灵活选择

### 工作流程

```
1. 创建批次
   ├─ 定义 Worker Prompt 模板
   ├─ 定义 Verifier Prompt 模板（可选）
   ├─ 提供测试项列表
   └─ 设置最大质检轮次

2. 执行批次
   ├─ Worker: 执行测试任务
   ├─ Verifier: 审查结果（输出 JSON verdict）
   ├─ Repair: 根据 feedback 自动返修
   └─ 循环直到通过或达到最大轮次

3. 查看结果
   ├─ 批次列表：所有批次概览
   ├─ 批次详情：统计卡片 + 测试项表格
   └─ 执行历史：每一轮的详细记录
```

## 🚀 快速开始

### 前置要求

- Go 1.21+
- Node.js 18+（仅前端需要）
- Codex CLI（执行器）

### 安装

#### 方式 1：从源码构建

```bash
# 克隆仓库
git clone https://github.com/limecloud/qcloop.git
cd qcloop

# 构建后端
go build -o qcloop ./cmd/qcloop

# 安装到系统路径（可选）
sudo mv qcloop /usr/local/bin/
```

#### 方式 2：使用 Go Install

```bash
go install github.com/limecloud/qcloop/cmd/qcloop@latest
```

### 验证安装

```bash
qcloop --help
```

## 📚 使用指南

### CLI 使用

#### 1. 创建批次

```bash
qcloop create \
  --name "test-lime-workspace" \
  --prompt "测试 Lime workspace 功能: {{item}}" \
  --verifier-prompt "检查结果，输出 JSON: {\"pass\": bool, \"feedback\": \"OK\"}" \
  --items "create,read,update,delete" \
  --max-qc-rounds 3
```

**参数说明**：
- `--name`: 批次名称
- `--prompt`: Worker Prompt 模板（`{{item}}` 会被替换为实际测试项）
- `--verifier-prompt`: Verifier Prompt 模板（可选，用于质检）
- `--items`: 测试项列表（逗号分隔）
- `--max-qc-rounds`: 最大质检轮次（默认 3）

**输出示例**：
```
✅ 批次创建成功
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
批次 ID: abc-123-def
批次名称: test-lime-workspace
测试项数: 4
最大质检轮次: 3
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

#### 2. 运行批次

```bash
qcloop run --job-id abc-123-def
```

**输出示例**：
```
🚀 开始执行批次: test-lime-workspace
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/4] 执行测试项: create
  ✅ Worker 执行成功
  ✅ Verifier 通过

[2/4] 执行测试项: read
  ✅ Worker 执行成功
  ❌ Verifier 失败: 输出格式不正确
  🔧 Repair 执行中...
  ✅ Verifier 通过

...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ 批次执行完成
```

#### 3. 查询状态

```bash
qcloop status --job-id abc-123-def
```

**输出示例**：
```
批次状态: test-lime-workspace
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
批次 ID: abc-123-def
状态: completed
创建时间: 2026-05-09 23:00:00
完成时间: 2026-05-09 23:05:30
总耗时: 5m30s

测试项统计:
  总数: 4
  ✅ 成功: 3 (75.0%)
  ❌ 失败: 1 (25.0%)
  ⏳ 待处理: 0 (0.0%)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Web 界面使用

#### 1. 启动服务

```bash
# 启动后端 API
qcloop serve --addr :8080

# 启动前端（新终端）
cd web
npm install
npm run dev
```

#### 2. 访问界面

打开浏览器访问：http://localhost:3000

#### 3. 界面功能

**批次列表页面**：
- 查看所有批次
- 显示批次名称、状态、测试项数、创建时间等
- 点击"查看详情"进入批次详情页

**批次详情页面**：
- 统计卡片：总数、成功、失败、进行中、待处理、已耗尽
- 测试项表格：9 列详细信息
- 实时状态更新（2 秒轮询）
- 运行批次按钮

**创建批次表单**：
- 填写批次名称
- 输入 Worker Prompt 模板
- 输入 Verifier Prompt 模板（可选）
- 输入测试项列表（逗号分隔）
- 设置最大质检轮次

## 🏗️ 架构设计

### 系统架构

```
┌─────────────────────────────────────────────────────────┐
│                    用户交互层                            │
│  ┌──────────────┐              ┌──────────────┐        │
│  │  CLI 命令    │              │  Web 界面    │        │
│  └──────────────┘              └──────────────┘        │
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                    API 层                                │
│              HTTP API Server (RESTful + CORS)           │
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                    编排层                                │
│         Runner 编排引擎（多轮质检循环）                  │
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                    执行层                                │
│            Codex Executor（子进程调用）                 │
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                    存储层                                │
│              SQLite（4 张表）                           │
└─────────────────────────────────────────────────────────┘
```

### 数据模型

```
batch_jobs (批次)
  ├─ id, name, prompt_template
  ├─ verifier_prompt_template
  ├─ max_qc_rounds, status
  └─ created_at, finished_at

batch_items (测试项)
  ├─ id, batch_job_id, item_value
  ├─ status, current_attempt_no, current_qc_no
  └─ created_at, finished_at

attempts (执行尝试)
  ├─ id, batch_item_id, attempt_no
  ├─ attempt_type (worker/repair)
  ├─ status, stdout, stderr, exit_code
  └─ started_at, finished_at

qc_rounds (质检轮次)
  ├─ id, batch_item_id, qc_no
  ├─ status, verdict, feedback
  └─ started_at, finished_at
```

### 多轮质检流程

```
开始
  │
  ▼
Worker 执行
  │
  ▼
保存 attempt
  │
  ▼
是否配置 verifier? ──否──▶ 标记为 success ──▶ 完成
  │
  是
  ▼
运行 Verifier
  │
  ▼
解析 verdict JSON
  │
  ▼
verdict.pass? ──是──▶ 标记为 success ──▶ 完成
  │
  否
  ▼
是否达到 max_qc_rounds? ──是──▶ 标记为 exhausted ──▶ 完成
  │
  否
  ▼
qc_no++
  │
  ▼
运行 Repair（注入 feedback）
  │
  ▼
返回 Worker 执行
```

## 📂 项目结构

```
qcloop/
├── cmd/qcloop/              # CLI 入口
│   └── main.go
├── internal/
│   ├── db/                  # 数据库层
│   │   ├── db.go            # 数据库连接
│   │   ├── models.go        # 数据模型
│   │   ├── schema.go        # 表结构
│   │   └── dao.go           # CRUD 操作
│   ├── core/                # 编排引擎
│   │   └── runner.go        # 多轮质检逻辑
│   ├── executor/            # 执行器
│   │   ├── codex.go         # Codex Executor
│   │   └── fake.go          # Fake Executor（测试用）
│   └── api/                 # HTTP API
│       └── server.go        # RESTful 服务器
├── web/                     # React 前端
│   ├── src/
│   │   ├── components/      # UI 组件
│   │   │   ├── BatchTable.tsx
│   │   │   ├── CreateJobForm.tsx
│   │   │   └── StatusBadges.tsx
│   │   ├── hooks/           # React Hooks
│   │   │   └── usePollingItems.ts
│   │   ├── api/             # API 客户端
│   │   │   └── index.ts
│   │   ├── types/           # TypeScript 类型
│   │   │   └── index.ts
│   │   ├── App.tsx
│   │   ├── main.tsx
│   │   └── styles.css
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── docs/                    # 文档
│   ├── PRD.md               # 产品需求文档（含设计哲学、用户故事）
│   ├── TEST_CASES.md        # 测试用例（32 个测试点）
│   ├── QUICK_TEST.md        # 快速测试指南
│   ├── GOAL_INTEGRATION.md  # Codex Goal 集成方案
│   └── PROJECT_SUMMARY.md   # 项目完成总结
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

## 👥 用户故事

### 故事 1：批量执行测试用例

**角色**：测试工程师

**场景**：我有 100 个 Lime 功能需要测试，手动一个一个测试太慢了。

**解决方案**：
1. 创建一个批次，包含所有测试项
2. qcloop 自动遍历执行每个测试项
3. 实时查看执行进度
4. 执行完成后查看详细结果

### 故事 2：确保测试质量

**角色**：QA 负责人

**场景**：AI 执行的测试可能不准确，我需要质检机制来验证结果。

**解决方案**：
1. 配置 verifier prompt 来审查测试结果
2. 如果质检失败，系统自动返修
3. 支持多轮质检，直到通过或达到上限
4. 查看每一轮的质检结果和反馈

### 故事 3：追踪批次执行情况

**角色**：项目经理

**场景**：我需要了解批次的整体进度和每个测试项的详细状态。

**解决方案**：
1. 查看所有批次的列表
2. 每个批次显示关键信息（名称、状态、测试项数、创建时间）
3. 点击批次查看详细信息
4. 详细页面显示每个测试项的执行情况

## 📖 文档

- [产品需求文档 (PRD)](docs/PRD.md) - 含设计哲学、用户故事、界面设计
- [测试用例文档](docs/TEST_CASES.md) - 32 个详细测试用例
- [快速测试指南](docs/QUICK_TEST.md) - 5 分钟快速测试
- [Codex Goal 集成方案](docs/GOAL_INTEGRATION.md) - 未来功能规划
- [项目完成总结](docs/PROJECT_SUMMARY.md) - 项目成果总结

## 🛠️ 技术栈

**后端**：
- Go 1.21+
- SQLite 3
- cobra（CLI 框架）
- net/http（HTTP 服务器）

**前端**：
- React 18
- TypeScript
- Vite
- 自定义 Hooks

**执行器**：
- Codex CLI（子进程调用）

## 🔧 开发

### 运行测试

```bash
go test ./...
```

### 本地开发

```bash
# 后端
go run ./cmd/qcloop serve --addr :8080

# 前端
cd web
npm run dev
```

### 构建生产版本

```bash
# 后端
go build -o qcloop ./cmd/qcloop

# 前端
cd web
npm run build
```

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📝 License

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- [Codex CLI](https://github.com/openai/codex) - AI 执行引擎
- [Cobra](https://github.com/spf13/cobra) - CLI 框架
- [React](https://reactjs.org/) - 前端框架
- [Vite](https://vitejs.dev/) - 前端构建工具

---

<div align="center">

**[⬆ 回到顶部](#qcloop)**

Made with ❤️ by the qcloop team

</div>
