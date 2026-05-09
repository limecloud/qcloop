# qcloop

Lime 批量测试编排工具 - 程序驱动的 AI 测试执行器

## 核心特性

- **不漏项** - 数据库事务 claim 机制，确保每个测试项都被执行
- **可恢复** - 崩溃后自动恢复未完成的测试项
- **可追踪** - 完整的执行历史、轮次记录、产物存储
- **多轮质检** - worker -> verifier -> repair 自动闭环

## 快速开始

### 安装

```bash
go install github.com/coso/qcloop/cmd/qcloop@latest
```

### 创建批次

```bash
qcloop create \
  --name "test-lime-workspace" \
  --worker-prompt "测试 Lime workspace 功能: {{test_case}}" \
  --verifier-prompt "检查测试结果是否符合预期，输出 JSON: {\"pass\": bool, \"feedback\": string}" \
  --items "create_workspace,read_config,update_settings,delete_workspace" \
  --max-qc-rounds 3
```

### 运行批次

```bash
qcloop run --job-id <job-id>
```

### 查询状态

```bash
qcloop status --job-id <job-id>
qcloop detail --job-id <job-id> --item-id <item-id>
```

### 恢复中断批次

```bash
qcloop recover --job-id <job-id>
```

## 架构

```
qcloop/
├── cmd/qcloop/          # CLI 入口
├── internal/
│   ├── db/              # SQLite schema + DAO
│   ├── core/            # 编排引擎（dispatcher, state machine）
│   ├── executor/        # 执行器适配（codex, fake）
│   ├── qc/              # 质检器
│   └── api/             # HTTP API (可选)
└── web/                 # React 前端 (可选)
```

## 开发

### 运行测试

```bash
go test ./...
```

### 本地构建

```bash
go build -o qcloop ./cmd/qcloop
```

## 工作原理

### 1. 批次创建
用户提供 prompt 模板和测试项列表，qcloop 创建批次并写入数据库。

### 2. 串行调度
dispatcher 逐个 claim 测试项（事务内写 lease），确保不重复、不遗漏。

### 3. 执行与质检
- **worker**: 调用 `codex exec` 执行测试
- **verifier**: 独立审查结果
- **repair**: 如果 verifier fail，自动返修

### 4. 崩溃恢复
启动时扫描过期 lease，自动恢复未完成项。

## License

MIT
