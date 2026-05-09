# qcloop

Lime 批量测试编排工具 - 程序驱动的 AI 测试执行器

## 核心特性

- **不漏项** - 数据库 claim 机制，确保每个测试项都被执行
- **可追踪** - 完整的执行历史、轮次记录、产物存储
- **多轮质检** - worker -> verifier -> repair 自动闭环
- **Web 界面** - React 前端可视化批次状态

## 项目结构

```
qcloop/
├── cmd/qcloop/         # CLI 入口
├── internal/
│   ├── db/             # SQLite 数据库层
│   ├── core/           # 编排引擎（多轮质检）
│   ├── executor/       # codex exec 执行器
│   └── api/            # HTTP API 服务器
├── web/                # React + TypeScript 前端
│   ├── src/
│   │   ├── components/ # UI 组件
│   │   ├── hooks/      # React Hooks
│   │   ├── api/        # API 客户端
│   │   └── types/      # TypeScript 类型
│   └── package.json
├── docs/               # 产品文档
│   ├── PRD.md          # 完整 PRD
│   └── PRD_SIMPLE.md   # 精简版 PRD
└── go.mod
```

## 快速开始

### 1. 构建后端

```bash
go build -o qcloop ./cmd/qcloop
```

### 2. 启动 API 服务器

```bash
./qcloop serve --addr :8080
```

### 3. 启动前端（开发模式）

```bash
cd web
npm install
npm run dev
```

然后访问 http://localhost:3000

### 4. 或使用 CLI

```bash
# 创建批次
./qcloop create \
  --name "test-lime" \
  --prompt "测试 Lime 功能: {{item}}" \
  --verifier-prompt "检查结果，输出 JSON: {\"pass\": bool, \"feedback\": string}" \
  --items "a,b,c" \
  --max-qc-rounds 3

# 运行批次
./qcloop run --job-id <id>

# 查询状态
./qcloop status --job-id <id>
```

## 工作原理

### 1. 批次创建
用户提供 prompt 模板和测试项列表，qcloop 创建批次并写入数据库。

### 2. 多轮质检循环
对每个测试项：
1. **Worker**: 调用 `codex exec` 执行任务
2. **Verifier**: 独立审查结果（输出 JSON verdict）
3. **Repair**: 如果质检失败，根据 feedback 自动返修
4. 重复直到通过或达到 max_qc_rounds

### 3. 界面展示
- 批次表格视图
- 实时状态更新（2s 轮询）
- 执行摘要（首次 + 质检1/2/3...）
- 统计卡片（总数/成功/失败/进行中）

## 技术栈

**后端**：Go + SQLite + HTTP Server
**前端**：React + TypeScript + Vite
**执行器**：codex exec（子进程）

## License

MIT
