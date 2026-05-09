# qcloop 精简版 PRD

## 产品定位
qcloop 是一个**极简的批量测试编排工具**，用程序遍历替代"让 AI 自觉一个一个做"。

## 核心功能（MVP）

### 1. 创建批次
```bash
qcloop create \
  --name "test-lime" \
  --prompt "测试 Lime 功能: {{item}}" \
  --items "a,b,c"
```

**做什么**：
- 生成批次 ID
- 保存 prompt 模板
- 保存测试项列表（逗号分隔）

### 2. 运行批次
```bash
qcloop run --job-id <id>
```

**做什么**：
- 遍历所有测试项
- 对每个 item：
  1. 替换 prompt 模板中的 `{{item}}`
  2. 调用 `codex exec --prompt "..."`
  3. 保存 stdout/stderr
  4. 标记状态（success/failed）
- 串行执行，不并发

### 3. 查询状态
```bash
qcloop status --job-id <id>
```

**做什么**：
- 显示批次整体状态
- 显示成功/失败统计
- 显示每个 item 的状态

## 数据模型（极简）

### batch_jobs 表
```sql
CREATE TABLE batch_jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    prompt_template TEXT NOT NULL,
    status TEXT NOT NULL,  -- pending/running/completed/failed
    created_at TEXT NOT NULL,
    finished_at TEXT
);
```

### batch_items 表
```sql
CREATE TABLE batch_items (
    id TEXT PRIMARY KEY,
    batch_job_id TEXT NOT NULL,
    item_value TEXT NOT NULL,  -- 测试项的值（如 "a", "b", "c"）
    status TEXT NOT NULL,      -- pending/running/success/failed
    stdout TEXT,
    stderr TEXT,
    exit_code INTEGER,
    created_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (batch_job_id) REFERENCES batch_jobs(id)
);
```

## 技术实现

### 核心流程
```
1. 用户创建批次 -> 写入 batch_jobs 和 batch_items
2. 用户运行批次 -> for item in items:
   - 替换 prompt 模板
   - 调用 codex exec
   - 保存结果
3. 用户查询状态 -> 读取数据库
```

### 目录结构
```
qcloop/
├── cmd/qcloop/main.go      # CLI 入口
├── internal/
│   ├── db/
│   │   ├── db.go           # 数据库连接
│   │   ├── models.go       # 数据模型
│   │   └── dao.go          # CRUD 操作
│   └── executor/
│       └── codex.go        # codex exec 调用
├── go.mod
└── README.md
```

## 去掉的功能（后续可选）
- ❌ 多轮质检（verifier/repair）
- ❌ lease 机制
- ❌ 崩溃恢复
- ❌ 并发执行
- ❌ GUI
- ❌ 复杂状态机

## 实施计划
1. 实现数据库层（1 小时）
2. 实现 codex executor（1 小时）
3. 实现 CLI 命令（1 小时）
4. 测试验证（30 分钟）

**总计：3.5 小时完成 MVP**
