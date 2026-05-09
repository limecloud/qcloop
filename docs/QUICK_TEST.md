# qcloop 快速测试指南

本文档提供快速测试 qcloop 的步骤，适合 Codex 快速验证功能。

## 快速测试流程（5 分钟）

### 1. 构建项目
```bash
cd /Users/coso/Documents/dev/ai/limecloud/qcloop
go build -o qcloop ./cmd/qcloop
```

### 2. 测试基础 CLI
```bash
# 测试帮助
./qcloop --help

# 创建简单批次
./qcloop create \
  --name "quick-test" \
  --prompt "echo 'Testing: {{item}}'" \
  --items "test1,test2,test3"

# 记录批次 ID（从输出中复制）
BATCH_ID="<复制这里>"

# 查询状态
./qcloop status --job-id $BATCH_ID

# 运行批次
./qcloop run --job-id $BATCH_ID

# 再次查询状态（应该显示 completed）
./qcloop status --job-id $BATCH_ID
```

**预期结果**：
- ✅ 批次创建成功
- ✅ 3 个测试项都执行成功
- ✅ 状态变为 completed

### 3. 测试多轮质检
```bash
# 创建带 verifier 的批次
./qcloop create \
  --name "qc-test" \
  --prompt "echo 'Result: {{item}}'" \
  --verifier-prompt "echo '{\"pass\": true, \"feedback\": \"OK\"}'" \
  --items "qc1,qc2" \
  --max-qc-rounds 3

QC_BATCH_ID="<复制这里>"

# 运行批次
./qcloop run --job-id $QC_BATCH_ID

# 查询状态
./qcloop status --job-id $QC_BATCH_ID
```

**预期结果**：
- ✅ worker 执行成功
- ✅ verifier 执行成功
- ✅ 所有 item 通过质检

### 4. 测试 HTTP API
```bash
# 启动 API 服务器（后台）
./qcloop serve --addr :8080 &
API_PID=$!
sleep 2

# 测试创建批次 API
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-test",
    "prompt_template": "echo {{item}}",
    "items": ["api1", "api2"]
  }'

# 记录返回的批次 ID
API_BATCH_ID="<从响应中提取>"

# 测试获取批次
curl http://localhost:8080/api/jobs/$API_BATCH_ID

# 测试运行批次
curl -X POST http://localhost:8080/api/jobs/run \
  -H "Content-Type: application/json" \
  -d "{\"job_id\": \"$API_BATCH_ID\"}"

# 等待几秒
sleep 3

# 测试获取批次项
curl "http://localhost:8080/api/items/?job_id=$API_BATCH_ID"

# 停止 API 服务器
kill $API_PID
```

**预期结果**：
- ✅ API 创建批次成功
- ✅ API 运行批次成功
- ✅ API 返回批次项数据

### 5. 测试前端（可选）
```bash
# 启动后端 API
./qcloop serve --addr :8080 &
API_PID=$!

# 启动前端
cd web
npm install
npm run dev &
WEB_PID=$!

# 访问 http://localhost:3000
# 手动测试创建批次和运行批次

# 停止服务
kill $API_PID
kill $WEB_PID
```

**预期结果**：
- ✅ 前端页面加载成功
- ✅ 可以创建批次
- ✅ 可以运行批次
- ✅ 实时状态更新

## 验证数据库

```bash
# 查看数据库文件
ls -lh ~/.qcloop/qcloop.db

# 查询批次
sqlite3 ~/.qcloop/qcloop.db "SELECT id, name, status FROM batch_jobs;"

# 查询批次项
sqlite3 ~/.qcloop/qcloop.db "SELECT id, item_value, status FROM batch_items;"

# 查询执行尝试
sqlite3 ~/.qcloop/qcloop.db "SELECT id, attempt_no, attempt_type, status FROM attempts;"

# 查询质检轮次
sqlite3 ~/.qcloop/qcloop.db "SELECT id, qc_no, status FROM qc_rounds;"
```

## 清理测试数据

```bash
# 删除数据库文件
rm ~/.qcloop/qcloop.db

# 或者备份后删除
mv ~/.qcloop/qcloop.db ~/.qcloop/qcloop.db.backup
```

## 常见问题

### Q: codex exec 命令不存在
A: 确保 codex CLI 已安装并在 PATH 中

### Q: 数据库权限错误
A: 检查 ~/.qcloop/ 目录权限

### Q: API 端口占用
A: 使用 `lsof -i :8080` 查看占用进程，或更换端口

### Q: 前端无法连接后端
A: 确保后端 API 运行在 8080 端口，前端代理配置正确

## 测试检查清单

- [ ] CLI 帮助信息正常
- [ ] 创建批次成功
- [ ] 运行批次成功
- [ ] 查询状态正常
- [ ] 多轮质检功能正常
- [ ] HTTP API 正常
- [ ] 前端界面正常
- [ ] 数据库记录正确
- [ ] 错误处理正常

## 报告问题

如果发现问题，请记录：
1. 执行的命令
2. 预期结果
3. 实际结果
4. 错误信息
5. 环境信息（Go 版本、OS 版本等）
