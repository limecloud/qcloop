# qcloop 产品需求文档 (PRD)

## 1. 产品概述

### 1.1 产品定位
qcloop 是一个**程序驱动的 AI 批量测试编排工具**，专门用于 Lime 项目的系统化测试。它解决了"让 AI 自觉一个一个处理"的不可靠问题，通过数据库事务、串行调度、多轮质检确保批量任务不漏项、可恢复、可追踪。

### 1.2 核心价值
- **可靠性** - 程序驱动替代 AI 自觉，确保每个测试项都被执行
- **可恢复性** - 崩溃后自动恢复未完成项，支持断点续跑
- **可追踪性** - 完整的执行历史、轮次记录、产物存储
- **质量保证** - worker -> verifier -> repair 自动闭环，多轮质检

### 1.3 目标用户
- Lime 项目开发者
- QA 测试工程师
- 需要批量执行 AI 驱动测试的团队

## 2. 核心功能

### 2.1 批次管理

#### 2.1.1 创建批次
**功能描述**：用户提供 prompt 模板和测试项列表，创建一个批次任务。

**输入参数**：
- `name` - 批次名称（必填）
- `description` - 批次描述（可选）
- `worker_prompt_template` - worker 执行模板（必填）
  - 支持变量替换：`{{item}}`, `{{file_path}}`, `{{test_case}}` 等
- `verifier_prompt_template` - verifier 质检模板（可选）
  - 要求输出结构化 JSON：`{"pass": bool, "feedback": string}`
- `repair_prompt_template` - repair 返修模板（可选）
- `items` - 测试项列表（必填）
  - 支持逗号分隔：`item1,item2,item3`
  - 支持文件路径：`file1.md,file2.md`
- `max_qc_rounds` - 最大质检轮次（默认 3）
- `concurrency` - 并发度（MVP 固定为 1）

**输出**：
- 批次 ID
- 批次状态
- 总测试项数

**CLI 示例**：
```bash
qcloop create \
  --name "test-lime-workspace" \
  --description "批量测试 Lime workspace 功能" \
  --worker-prompt "测试 Lime workspace 功能: {{test_case}}" \
  --verifier-prompt "检查测试结果是否符合预期，输出 JSON: {\"pass\": bool, \"feedback\": string}" \
  --items "create_workspace,read_config,update_settings,delete_workspace" \
  --max-qc-rounds 3
```

#### 2.1.2 运行批次
**功能描述**：启动批次执行，逐个处理测试项。

**执行流程**：
1. dispatcher 从数据库 claim 下一条 `pending` 或 `retry_pending` 的 item
2. 在事务内写入 `lease_owner` 和 `lease_expires_at`
3. 事务提交成功后，启动 executor 执行
4. 执行完成后，更新 item 状态
5. 重复直到所有 item 完成

**CLI 示例**：
```bash
qcloop run --job-id <job-id>
```

#### 2.1.3 查询状态
**功能描述**：查询批次整体状态和进度。

**输出信息**：
- 批次 ID、名称、描述
- 状态：`pending/running/completed/failed/canceled/paused`
- 总测试项数
- 已通过项数
- 已耗尽项数（达到最大轮次仍未通过）
- 已取消项数
- 开始时间、结束时间

**CLI 示例**：
```bash
qcloop status --job-id <job-id>
```

#### 2.1.4 查询详情
**功能描述**：查询单个测试项的详细执行历史。

**输出信息**：
- item ID、参数、状态
- 执行尝试列表（attempt）
  - 尝试编号、状态、开始/结束时间、耗时
  - executor 类型、thread ID、session ID
  - stdout、stderr、exit code
- 质检轮次列表（qc_round）
  - 轮次编号、状态、开始/结束时间、耗时
  - verifier thread ID
  - verdict（结构化判定）、feedback（返修建议）
- 产物列表（artifact）
  - 产物类型、内容、创建时间

**CLI 示例**：
```bash
qcloop detail --job-id <job-id> --item-id <item-id>
```

#### 2.1.5 恢复中断批次
**功能描述**：崩溃后自动恢复未完成的批次。

**恢复逻辑**：
1. 扫描所有 `lease_expires_at` 已过期的 item
2. 检查对应的 attempt 和 qc_round 实际终态
3. 根据终态回滚 item 状态：
   - attempt 成功 + qc 未开始 -> `pending`（进入质检）
   - attempt 成功 + qc 失败 -> `retry_pending`（返修）
   - attempt 失败 -> `retry_pending`（重试）
4. 清空 `lease_owner` 和 `lease_expires_at`

**CLI 示例**：
```bash
qcloop recover --job-id <job-id>
```

### 2.2 执行器（Executor）

#### 2.2.1 Executor 接口
```go
type Executor interface {
    Execute(ctx context.Context, params ExecuteParams) (*ExecuteResult, error)
}

type ExecuteParams struct {
    Prompt      string
    ThreadID    *string  // 续跑时提供
    Timeout     time.Duration
}

type ExecuteResult struct {
    Status       AttemptStatus
    ThreadID     *string
    SessionID    *string
    ExitCode     *int
    Stdout       string
    Stderr       string
    ErrorMessage string
    DurationMs   int64
}
```

#### 2.2.2 Codex Executor
**功能描述**：通过子进程调用 `codex exec` 执行测试。

**实现要点**：
- 使用 `os/exec` 启动子进程
- 捕获 stdout 和 stderr
- 支持超时控制（context.WithTimeout）
- 提取 thread_id（从 stdout 解析）
- 支持续跑（传入 thread_id）

**调用示例**：
```bash
codex exec --prompt "测试 Lime workspace 功能: create_workspace"
```

#### 2.2.3 Fake Executor
**功能描述**：测试用 fake 实现，模拟成功/失败。

**用途**：
- 单元测试
- 集成测试
- 验证状态机转换

### 2.3 质检器（QC）

#### 2.3.1 Verifier
**功能描述**：独立审查 worker 执行结果，判断是否通过。

**输入**：
- verifier_prompt_template
- worker 执行产物（stdout、diff 等）

**输出**：
- verdict（结构化 JSON）
  ```json
  {
    "pass": true,
    "feedback": "测试通过，workspace 创建成功"
  }
  ```

**实现要点**：
- 使用独立的 verifier thread（不复用 worker thread）
- 解析 JSON verdict
- 如果 `pass: false`，提取 feedback 用于 repair

#### 2.3.2 Repair
**功能描述**：根据 verifier feedback 返修。

**输入**：
- repair_prompt_template
- verifier feedback

**输出**：
- 新的执行结果

**实现要点**：
- 在原 worker thread 上续跑（保留上下文）
- 注入 feedback 到 repair prompt
- 重复 verifier -> repair 直到通过或达到 `max_qc_rounds`

### 2.4 状态机

#### 2.4.1 BatchJob 状态
```
pending -> running -> completed
                   -> failed
                   -> canceled
                   -> paused
```

#### 2.4.2 BatchItem 状态
```
pending -> claimed -> running -> qc_running -> passed (终态)
                              -> qc_failed -> retry_pending -> running
                                           -> exhausted (终态)
                                           -> canceled (终态)
```

#### 2.4.3 Attempt 状态
```
running -> success
        -> error
        -> timeout
        -> canceled
```

#### 2.4.4 QCRound 状态
```
running -> pass
        -> fail
        -> error
```

## 3. 数据模型

### 3.1 数据库表

#### 3.1.1 batch_jobs
批次表，存储批次定义和汇总统计。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | 主键 |
| name | TEXT | 批次名称 |
| description | TEXT | 批次描述 |
| status | TEXT | 状态 |
| worker_prompt_template | TEXT | worker 模板 |
| verifier_prompt_template | TEXT | verifier 模板 |
| repair_prompt_template | TEXT | repair 模板 |
| max_qc_rounds | INTEGER | 最大质检轮次 |
| concurrency | INTEGER | 并发度 |
| created_at | TEXT | 创建时间 |
| started_at | TEXT | 开始时间 |
| finished_at | TEXT | 结束时间 |
| total_items | INTEGER | 总测试项数 |
| passed_items | INTEGER | 已通过项数 |
| exhausted_items | INTEGER | 已耗尽项数 |
| canceled_items | INTEGER | 已取消项数 |

#### 3.1.2 batch_items
批次项表，存储单个测试项的状态和 lease 信息。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | 主键 |
| batch_job_id | TEXT | 外键 -> batch_jobs.id |
| item_key | TEXT | 参数 hash（去重） |
| params | TEXT | JSON 格式参数 |
| status | TEXT | 状态 |
| lease_owner | TEXT | lease 持有者 |
| lease_expires_at | TEXT | lease 过期时间 |
| current_attempt_no | INTEGER | 当前尝试编号 |
| current_qc_no | INTEGER | 当前质检编号 |
| created_at | TEXT | 创建时间 |
| claimed_at | TEXT | claim 时间 |
| finished_at | TEXT | 完成时间 |

**唯一约束**：`(batch_job_id, item_key)`

#### 3.1.3 attempts
执行尝试表，存储每次 worker 执行的详细信息。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | 主键 |
| batch_item_id | TEXT | 外键 -> batch_items.id |
| attempt_no | INTEGER | 尝试编号 |
| status | TEXT | 状态 |
| executor_type | TEXT | 执行器类型 |
| thread_id | TEXT | codex thread ID |
| session_id | TEXT | codex session ID |
| started_at | TEXT | 开始时间 |
| finished_at | TEXT | 结束时间 |
| duration_ms | INTEGER | 耗时（毫秒） |
| exit_code | INTEGER | 退出码 |
| stdout | TEXT | 标准输出 |
| stderr | TEXT | 标准错误 |
| error_message | TEXT | 错误信息 |

**唯一约束**：`(batch_item_id, attempt_no)`

#### 3.1.4 qc_rounds
质检轮次表，存储每次 verifier 执行的详细信息。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | 主键 |
| batch_item_id | TEXT | 外键 -> batch_items.id |
| qc_no | INTEGER | 质检编号 |
| status | TEXT | 状态 |
| verifier_thread_id | TEXT | verifier thread ID |
| started_at | TEXT | 开始时间 |
| finished_at | TEXT | 结束时间 |
| duration_ms | INTEGER | 耗时（毫秒） |
| verdict | TEXT | JSON 格式判定 |
| feedback | TEXT | 返修建议 |

**唯一约束**：`(batch_item_id, qc_no)`

#### 3.1.5 artifacts
产物表，存储执行过程中的各类产物。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | 主键 |
| batch_item_id | TEXT | 外键 -> batch_items.id |
| artifact_type | TEXT | 产物类型 |
| content | TEXT | 产物内容 |
| created_at | TEXT | 创建时间 |

**产物类型**：
- `prompt_snapshot` - prompt 快照
- `diff` - 代码变更
- `log` - 执行日志
- `verdict` - 质检判定

## 4. 技术架构

### 4.1 系统架构图

```mermaid
graph TB
    subgraph "用户交互层"
        CLI[CLI 命令]
        GUI[Web GUI<br/>批次表格视图]
    end
    
    subgraph "编排层"
        BatchManager[批次管理器<br/>创建/查询/恢复]
        Dispatcher[调度器<br/>逐个 claim item]
        Recovery[恢复器<br/>扫描过期 lease]
    end
    
    subgraph "执行层"
        Worker[Worker 执行器<br/>首次执行]
        Verifier[Verifier 质检器<br/>质检1/2/3...]
        Repair[Repair 返修器<br/>在原 thread 续跑]
    end
    
    subgraph "存储层"
        DAO[DAO 层]
        DB[(SQLite<br/>batch_jobs<br/>batch_items<br/>attempts<br/>qc_rounds<br/>artifacts)]
    end
    
    CLI --> BatchManager
    GUI --> BatchManager
    
    BatchManager --> Dispatcher
    BatchManager --> Recovery
    
    Dispatcher --> Worker
    Worker --> Verifier
    Verifier -->|pass| DAO
    Verifier -->|fail| Repair
    Repair --> Verifier
    
    Recovery --> DAO
    Dispatcher --> DAO
    DAO --> DB
    
    style CLI fill:#e1f5ff
    style GUI fill:#e1f5ff
    style Dispatcher fill:#fff4e1
    style Worker fill:#f0e1ff
    style Verifier fill:#e1ffe1
    style Repair fill:#ffe1f0
    style DB fill:#ffe1e1
```

### 4.2 数据模型关系图

```mermaid
erDiagram
    BATCH_JOBS ||--o{ BATCH_ITEMS : contains
    BATCH_ITEMS ||--o{ ATTEMPTS : has
    BATCH_ITEMS ||--o{ QC_ROUNDS : has
    BATCH_ITEMS ||--o{ ARTIFACTS : produces
    
    BATCH_JOBS {
        string id PK
        string name
        string status
        string worker_prompt_template
        string verifier_prompt_template
        int max_qc_rounds
        int total_items
        int passed_items
    }
    
    BATCH_ITEMS {
        string id PK
        string batch_job_id FK
        string item_key UK
        string params
        string status
        string lease_owner
        datetime lease_expires_at
        int current_attempt_no
        int current_qc_no
    }
    
    ATTEMPTS {
        string id PK
        string batch_item_id FK
        int attempt_no UK
        string status
        string executor_type
        string thread_id
        string stdout
        string stderr
    }
    
    QC_ROUNDS {
        string id PK
        string batch_item_id FK
        int qc_no UK
        string status
        string verdict
        string feedback
    }
    
    ARTIFACTS {
        string id PK
        string batch_item_id FK
        string artifact_type
        string content
    }
```

### 4.3 技术栈
- **后端**：Go 1.21+
- **数据库**：SQLite 3
- **CLI 框架**：cobra
- **前端**（可选）：React + TypeScript + Tailwind CSS

### 4.2 目录结构
```
qcloop/
├── cmd/qcloop/          # CLI 入口
├── internal/
│   ├── db/              # 数据库层
│   │   ├── models.go    # 数据模型
│   │   ├── schema.go    # 表定义
│   │   ├── db.go        # 连接管理
│   │   └── dao.go       # DAO 层
│   ├── core/            # 核心编排
│   │   ├── dispatcher.go    # 调度器
│   │   ├── state_machine.go # 状态机
│   │   └── recovery.go      # 恢复逻辑
│   ├── executor/        # 执行器
│   │   ├── executor.go  # 接口定义
│   │   ├── codex.go     # codex 实现
│   │   └── fake.go      # fake 实现
│   ├── qc/              # 质检器
│   │   ├── verifier.go  # verifier
│   │   └── verdict.go   # verdict 解析
│   └── api/             # HTTP API（可选）
├── web/                 # React 前端（可选）
├── docs/                # 文档
│   ├── PRD.md           # 产品需求文档
│   ├── ARCHITECTURE.md  # 架构设计
│   └── API.md           # API 文档
├── go.mod
├── go.sum
├── README.md
└── .gitignore
```

### 4.5 核心流程

#### 4.5.1 创建批次流程图

```mermaid
flowchart TD
    Start([用户输入]) --> Parse[解析参数]
    Parse --> ValidateParams{参数校验}
    ValidateParams -->|失败| Error1[返回错误]
    ValidateParams -->|成功| GenJob[生成 batch_job]
    GenJob --> GenItems[生成 batch_items]
    GenItems --> CalcKey[计算 item_key<br/>防重复]
    CalcKey --> WriteTx[事务写入数据库]
    WriteTx --> Success[返回 job_id]
    
    style Start fill:#e1f5ff
    style Success fill:#e1ffe1
    style Error1 fill:#ffe1e1
```

#### 4.5.2 执行批次时序图

```mermaid
sequenceDiagram
    participant CLI
    participant Dispatcher
    participant DB
    participant Executor
    participant Verifier
    
    CLI->>Dispatcher: run(job_id)
    
    loop 直到所有 item 完成
        Dispatcher->>DB: BEGIN TRANSACTION
        Dispatcher->>DB: SELECT ... FOR UPDATE
        DB-->>Dispatcher: next_item
        Dispatcher->>DB: UPDATE lease_owner, lease_expires_at
        Dispatcher->>DB: COMMIT
        
        Dispatcher->>Executor: execute(item)
        Executor-->>Dispatcher: attempt_result
        Dispatcher->>DB: save attempt
        
        alt 需要质检
            Dispatcher->>Verifier: verify(attempt_result)
            Verifier-->>Dispatcher: verdict
            Dispatcher->>DB: save qc_round
            
            alt verdict.pass == false
                alt 未达最大轮次
                    Dispatcher->>Executor: repair(feedback)
                    Note over Dispatcher,Executor: 在原 thread 续跑
                else 达到最大轮次
                    Dispatcher->>DB: mark as exhausted
                end
            else verdict.pass == true
                Dispatcher->>DB: mark as passed
            end
        else 无需质检
            Dispatcher->>DB: mark as passed
        end
    end
    
    Dispatcher-->>CLI: 批次完成
```

#### 4.5.3 质检闭环流程图

```mermaid
flowchart TD
    Start([Worker 执行完成]) --> SaveAttempt[保存 attempt]
    SaveAttempt --> CheckVerifier{是否配置<br/>verifier?}
    
    CheckVerifier -->|否| MarkPassed[标记 item 为 passed]
    CheckVerifier -->|是| RunVerifier[运行 verifier<br/>独立 thread]
    
    RunVerifier --> ParseVerdict[解析 verdict JSON]
    ParseVerdict --> CheckPass{verdict.pass?}
    
    CheckPass -->|true| MarkPassed
    CheckPass -->|false| CheckRounds{是否达到<br/>max_qc_rounds?}
    
    CheckRounds -->|是| MarkExhausted[标记 item 为 exhausted]
    CheckRounds -->|否| IncrementQC[qc_no++]
    
    IncrementQC --> RunRepair[运行 repair<br/>原 worker thread 续跑]
    RunRepair --> InjectFeedback[注入 feedback]
    InjectFeedback --> SaveAttempt
    
    MarkPassed --> End([完成])
    MarkExhausted --> End
    
    style Start fill:#e1f5ff
    style End fill:#e1ffe1
    style MarkExhausted fill:#fff4e1
```

#### 4.5.4 崩溃恢复流程图

```mermaid
flowchart TD
    Start([启动 qcloop]) --> ScanExpired[扫描过期 lease]
    ScanExpired --> Query[SELECT ... WHERE<br/>lease_expires_at < NOW()]
    Query --> HasExpired{有过期 item?}
    
    HasExpired -->|否| Normal[正常启动]
    HasExpired -->|是| LoopItems[遍历过期 item]
    
    LoopItems --> GetAttempt[查询最新 attempt]
    GetAttempt --> GetQC[查询最新 qc_round]
    
    GetQC --> CheckState{判断终态}
    
    CheckState -->|attempt 成功<br/>qc 未开始| Rollback1[回滚到 pending<br/>进入质检]
    CheckState -->|attempt 成功<br/>qc 失败| Rollback2[回滚到 retry_pending<br/>返修]
    CheckState -->|attempt 失败| Rollback3[回滚到 retry_pending<br/>重试]
    
    Rollback1 --> ClearLease[清空 lease_owner<br/>和 lease_expires_at]
    Rollback2 --> ClearLease
    Rollback3 --> ClearLease
    
    ClearLease --> NextItem{还有下一个?}
    NextItem -->|是| LoopItems
    NextItem -->|否| Normal
    
    Normal --> End([恢复完成])
    
    style Start fill:#e1f5ff
    style End fill:#e1ffe1
```

## 5. 实施计划

### 5.1 Stage 1: MVP 核心（1-2 周）
**目标**：最小可用编排引擎 + fake 执行器

**交付物**：
- SQLite schema + DAO
- 串行 dispatcher（claim + 事务）
- fake executor（模拟成功/失败）
- 崩溃恢复逻辑
- CLI: `create` / `run` / `status` / `recover`

**验证**：
```bash
go test ./...
qcloop create --name test --items a,b,c --worker-prompt "process {{item}}"
qcloop run --job-id <id>
# Ctrl+C 中断
qcloop recover --job-id <id>
```

### 5.2 Stage 2: Codex 执行器（1 周）
**目标**：真正调用 `codex exec` 测试 Lime

**交付物**：
- `internal/executor/codex.go`
- 子进程管理 + 输出捕获
- 超时控制
- thread_id 提取

**验证**：
```bash
qcloop create --name test-lime \
  --items "test1,test2,test3" \
  --worker-prompt "测试 Lime workspace 功能: {{test_case}}"
qcloop run --job-id <id>
```

### 5.3 Stage 3: 多轮质检（1 周）
**目标**：verifier + repair 闭环

**交付物**：
- `internal/qc/verifier.go`
- 结构化 verdict 解析
- repair prompt 注入
- 轮次统计

**验证**：
```bash
qcloop create --name qc-test \
  --worker-prompt "修改 {{file}}" \
  --verifier-prompt "检查是否符合规范，输出 JSON" \
  --max-qc-rounds 3
qcloop run --job-id <id>
qcloop detail --job-id <id> --item-id <item-id>
```

### 5.4 Stage 4: React GUI（可选，1-2 周）
**目标**：可视化批次面板

**技术栈**：
- React + TypeScript
- TanStack Query（数据获取）
- Tailwind CSS（样式）
- Go HTTP API（提供数据）

**交付物**：
- 批次列表页
- item 详情页（显示轮次历史）
- 实时状态更新

## 6. 成功指标

### 6.1 功能指标
- ✅ 批次创建成功率 100%
- ✅ item 执行不漏项率 100%
- ✅ 崩溃恢复成功率 100%
- ✅ 质检闭环成功率 > 95%

### 6.2 性能指标
- 单个 item 执行延迟 < 5s（不含 codex exec 时间）
- 批次恢复时间 < 10s
- 数据库查询延迟 < 100ms

### 6.3 可用性指标
- CLI 命令响应时间 < 1s
- 错误信息清晰度 > 90%（用户反馈）
- 文档完整度 100%

## 7. 风险与挑战

### 7.1 技术风险
- **codex exec 稳定性**：依赖外部工具，可能不稳定
  - 缓解：增加重试机制、超时控制
- **SQLite 并发限制**：单写入限制
  - 缓解：MVP 固定并发度为 1
- **thread_id 提取失败**：codex exec 输出格式变化
  - 缓解：正则表达式 + fallback 逻辑

### 7.2 产品风险
- **用户学习成本**：CLI 命令较多
  - 缓解：提供详细文档和示例
- **prompt 模板复杂度**：用户可能不会写
  - 缓解：提供常用模板库

### 7.3 运维风险
- **数据库损坏**：SQLite 文件损坏
  - 缓解：定期备份、WAL 模式
- **磁盘空间不足**：产物存储占用大
  - 缓解：定期清理、压缩存储

## 8. 未来规划

### 8.1 短期（3 个月）
- 支持并发执行（concurrency > 1）
- 支持更多执行器（Claude Code SDK、OpenAI API）
- 支持批次暂停/恢复
- 支持批次克隆

### 8.2 中期（6 个月）
- React GUI 完整实现
- 支持批次模板
- 支持批次调度（定时执行）
- 支持批次依赖（批次 A 完成后自动启动批次 B）

### 8.3 长期（1 年）
- 支持分布式执行（多机并发）
- 支持批次优先级
- 支持批次成本统计（token 消耗）
- 支持批次报告生成（PDF、HTML）

## 9. 附录

### 9.1 术语表
- **batch_job**：批次，一组测试项的集合
- **batch_item**：批次项，单个测试项
- **attempt**：执行尝试，一次 worker 执行
- **qc_round**：质检轮次，一次 verifier 执行
- **artifact**：产物，执行过程中的各类输出
- **lease**：租约，用于防止重复执行的锁机制
- **claim**：领取，dispatcher 从数据库领取下一个待执行项
- **dispatcher**：调度器，负责串行调度 item 执行
- **executor**：执行器，负责真正执行测试（如 codex exec）
- **verifier**：质检器，负责审查执行结果
- **repair**：返修，根据 verifier feedback 重新执行

### 9.2 参考资料
- [Lime 项目](https://github.com/limecloud/lime)
- [Codex CLI](https://docs.anthropic.com/claude/docs/codex)
- [SQLite 文档](https://www.sqlite.org/docs.html)
- [Go 标准库](https://pkg.go.dev/std)
