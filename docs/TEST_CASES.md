# qcloop 测试用例文档

本文档提供完整的测试用例，用于验证 qcloop 批量测试编排工具的功能。

## 测试环境准备

### 前置条件
1. Go 1.21+ 已安装
2. Node.js 18+ 已安装（用于前端测试）
3. 至少一个本机 AI CLI 执行器已安装并可用；默认测试使用 codex CLI
4. SQLite 3 已安装

### 构建项目
```bash
cd /Users/coso/Documents/dev/ai/limecloud/qcloop
go build -o qcloop ./cmd/qcloop
```

## 测试用例 1: CLI 基础功能测试

### 1.1 测试 CLI 帮助信息
**目的**：验证 CLI 命令是否正常工作

**执行命令**：
```bash
./qcloop --help
```

**预期结果**：
- 显示 qcloop 的使用说明
- 列出所有可用命令：create, run, status, serve
- 显示全局参数：--db

**验证点**：
- [ ] 帮助信息完整显示
- [ ] 包含所有子命令
- [ ] 格式清晰易读

### 1.2 测试创建批次（无 verifier）
**目的**：验证基础批次创建功能

**执行命令**：
```bash
./qcloop create \
  --name "test-basic" \
  --prompt "echo 'Testing item: {{item}}'" \
  --items "item1,item2,item3"
```

**预期结果**：
- 成功创建批次
- 返回批次 ID
- 显示批次名称和测试项数量

**验证点**：
- [ ] 批次创建成功
- [ ] 批次 ID 格式正确（UUID）
- [ ] 测试项数量正确（3 个）
- [ ] 数据库文件创建在 ~/.qcloop/qcloop.db

**保存批次 ID**：
```bash
# 记录输出的批次 ID，用于后续测试
BATCH_ID="<从输出中复制>"
```

### 1.3 测试查询批次状态
**目的**：验证批次状态查询功能

**执行命令**：
```bash
./qcloop status --job-id $BATCH_ID
```

**预期结果**：
- 显示批次详细信息
- 状态为 "pending"
- 测试项统计正确

**验证点**：
- [ ] 批次信息完整
- [ ] 状态为 pending
- [ ] 总数为 3
- [ ] 待处理为 3

### 1.4 测试运行批次（无 verifier）
**目的**：验证批次执行功能

**执行命令**：
```bash
./qcloop run --job-id $BATCH_ID
```

**预期结果**：
- 批次开始执行
- 逐个处理测试项
- 显示执行进度
- 批次完成

**验证点**：
- [ ] 批次状态变为 running
- [ ] 每个 item 都被执行
- [ ] 执行完成后状态变为 completed
- [ ] 所有 item 状态为 success

### 1.5 测试查询完成后的状态
**目的**：验证批次完成后的状态

**执行命令**：
```bash
./qcloop status --job-id $BATCH_ID
```

**预期结果**：
- 状态为 "completed"
- 成功数量为 3
- 显示完成时间和总耗时

**验证点**：
- [ ] 状态为 completed
- [ ] 成功 3 个
- [ ] 失败 0 个
- [ ] 显示耗时

## 测试用例 2: 多轮质检功能测试

### 2.1 创建带 verifier 的批次
**目的**：验证多轮质检功能

**执行命令**：
```bash
./qcloop create \
  --name "test-qc" \
  --prompt "echo 'Result for {{item}}'" \
  --verifier-prompt "echo '{\"pass\": true, \"feedback\": \"OK\"}'" \
  --items "qc1,qc2,qc3" \
  --max-qc-rounds 3
```

**预期结果**：
- 成功创建批次
- 显示最大质检轮次为 3

**验证点**：
- [ ] 批次创建成功
- [ ] verifier_prompt_template 已保存
- [ ] max_qc_rounds 为 3

**保存批次 ID**：
```bash
QC_BATCH_ID="<从输出中复制>"
```

### 2.2 运行带质检的批次
**目的**：验证 worker -> verifier 流程

**执行命令**：
```bash
./qcloop run --job-id $QC_BATCH_ID
```

**预期结果**：
- 每个 item 先执行 worker
- 然后执行 verifier
- verifier 返回 pass: true
- item 标记为 success

**验证点**：
- [ ] worker 执行成功
- [ ] verifier 执行成功
- [ ] verdict 解析正确
- [ ] item 状态为 success

### 2.3 测试质检失败和返修
**目的**：验证 verifier fail -> repair 流程

**执行命令**：
```bash
./qcloop create \
  --name "test-repair" \
  --prompt "echo 'Attempt {{item}}'" \
  --verifier-prompt "echo '{\"pass\": false, \"feedback\": \"需要修复\"}'" \
  --items "repair1" \
  --max-qc-rounds 2
```

**预期结果**：
- worker 执行
- verifier 返回 pass: false
- 触发 repair
- repair 执行（注入 feedback）
- 再次 verifier
- 如果仍失败，达到最大轮次后标记为 exhausted

**验证点**：
- [ ] 第一次 verifier 失败
- [ ] 触发 repair
- [ ] repair 包含 feedback
- [ ] 达到 max_qc_rounds 后标记为 exhausted

## 测试用例 3: HTTP API 测试

### 3.1 启动 API 服务器
**目的**：验证 HTTP API 服务器

**执行命令**：
```bash
./qcloop serve --addr :8080 &
SERVER_PID=$!
sleep 2
```

**预期结果**：
- 服务器启动成功
- 监听 8080 端口

**验证点**：
- [ ] 服务器启动无错误
- [ ] 端口 8080 可访问

### 3.2 测试创建批次 API
**目的**：验证 POST /api/jobs

**执行命令**：
```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-test",
    "prompt_template": "echo {{item}}",
    "verifier_prompt_template": "",
    "max_qc_rounds": 3,
    "execution_mode": "standard",
    "executor_provider": "codex",
    "items": ["api1", "api2"]
  }'
```

**预期结果**：
- 返回 200 状态码
- 返回批次 JSON 对象
- 包含 id, name, status, executor_provider 等字段

**验证点**：
- [ ] HTTP 200
- [ ] 返回 JSON 格式
- [ ] 包含批次 ID
- [ ] status 为 pending
- [ ] executor_provider 为 codex

**保存批次 ID**：
```bash
API_BATCH_ID="<从响应中提取>"
```

### 3.3 测试获取批次 API
**目的**：验证 GET /api/jobs/:id

**执行命令**：
```bash
curl http://localhost:8080/api/jobs/$API_BATCH_ID
```

**预期结果**：
- 返回 200 状态码
- 返回批次详细信息

**验证点**：
- [ ] HTTP 200
- [ ] 批次信息完整
- [ ] ID 匹配

### 3.4 测试运行批次 API
**目的**：验证 POST /api/jobs/run

**执行命令**：
```bash
curl -X POST http://localhost:8080/api/jobs/run \
  -H "Content-Type: application/json" \
  -d "{\"job_id\": \"$API_BATCH_ID\", \"mode\": \"auto\"}"
```

**预期结果**：
- 返回 200 状态码
- 返回 {"status": "started"}
- 批次进入全局 worker pool 后台运行

**验证点**：
- [ ] HTTP 200
- [ ] 返回 started 状态
- [ ] 批次开始执行,未成功项重试时可使用 mode=retry_unfinished

### 3.5 测试获取批次项 API
**目的**：验证 GET /api/items?job_id=

**执行命令**：
```bash
curl "http://localhost:8080/api/items/?job_id=$API_BATCH_ID"
```

**预期结果**：
- 返回 200 状态码
- 返回批次项数组
- 每个 item 包含 attempts 和 qc_rounds

**验证点**：
- [ ] HTTP 200
- [ ] 返回数组
- [ ] 包含 attempts 字段
- [ ] 包含 qc_rounds 字段

### 3.6 停止 API 服务器
**执行命令**：
```bash
kill $SERVER_PID
```

## 测试用例 4: 前端界面测试

### 4.1 安装前端依赖
**执行命令**：
```bash
cd web
npm install
```

**预期结果**：
- 依赖安装成功
- 无错误

### 4.2 构建前端
**执行命令**：
```bash
npm run build
```

**预期结果**：
- 构建成功
- 生成 dist 目录
- 构建产物大小合理（< 200KB gzip）

**验证点**：
- [ ] 构建成功
- [ ] dist/index.html 存在
- [ ] dist/assets/ 包含 JS 和 CSS

### 4.3 启动前端开发服务器
**执行命令**：
```bash
# 先启动后端 API
cd ..
./qcloop serve --addr :8080 &
API_PID=$!

# 启动前端
cd web
npm run dev &
WEB_PID=$!
sleep 3
```

**预期结果**：
- 前端服务器启动在 3000 端口
- 后端 API 运行在 8080 端口

### 4.4 手动测试前端功能
**访问**：http://localhost:3000

**测试步骤**：
1. 填写创建批次表单
   - 批次名称：frontend-test
   - Worker Prompt：echo "Testing {{item}}"
   - Verifier Prompt：echo '{"pass": true, "feedback": "OK"}'
   - 测试项：test1,test2,test3
   - 最大质检轮次：3

2. 点击"创建批次"按钮

3. 点击"运行批次"按钮；如果批次已结束且有失败/耗尽项，按钮会显示"重试未成功项"

4. 观察实时更新
   - 统计卡片更新
   - 表格行状态变化
   - 执行摘要标签出现

**验证点**：
- [ ] 表单提交成功
- [ ] 批次创建成功
- [ ] 批次开始运行
- [ ] 实时状态更新（2s 轮询）
- [ ] 表格显示正确
- [ ] 状态标签正确
- [ ] 执行摘要标签正确
- [ ] 统计卡片正确

### 4.5 停止服务
**执行命令**：
```bash
kill $API_PID
kill $WEB_PID
```

## 测试用例 5: 数据库验证测试

### 5.1 检查数据库文件
**执行命令**：
```bash
ls -lh ~/.qcloop/qcloop.db
```

**预期结果**：
- 数据库文件存在
- 文件大小合理

### 5.2 查询数据库表结构
**执行命令**：
```bash
sqlite3 ~/.qcloop/qcloop.db ".schema"
```

**预期结果**：
- 显示 5 张表的结构
- batch_jobs
- batch_items
- attempts
- qc_rounds
- batch_templates

**验证点**：
- [ ] 5 张表都存在
- [ ] 外键约束正确
- [ ] 索引存在

### 5.3 查询批次数据
**执行命令**：
```bash
sqlite3 ~/.qcloop/qcloop.db "SELECT id, name, status FROM batch_jobs LIMIT 5;"
```

**预期结果**：
- 显示创建的批次记录
- 数据格式正确

### 5.4 查询批次项数据
**执行命令**：
```bash
sqlite3 ~/.qcloop/qcloop.db "SELECT id, item_value, status FROM batch_items LIMIT 5;"
```

**预期结果**：
- 显示批次项记录
- 状态正确

### 5.5 查询执行尝试数据
**执行命令**：
```bash
sqlite3 ~/.qcloop/qcloop.db "SELECT id, attempt_no, attempt_type, status FROM attempts LIMIT 5;"
```

**预期结果**：
- 显示执行尝试记录
- attempt_type 为 worker 或 repair

### 5.6 查询质检轮次数据
**执行命令**：
```bash
sqlite3 ~/.qcloop/qcloop.db "SELECT id, qc_no, status FROM qc_rounds LIMIT 5;"
```

**预期结果**：
- 显示质检轮次记录
- status 为 pass 或 fail

## 测试用例 6: 边界条件测试

### 6.1 测试空测试项列表
**执行命令**：
```bash
./qcloop create \
  --name "test-empty" \
  --prompt "echo {{item}}" \
  --items ""
```

**预期结果**：
- 创建失败或创建成功但 total_items 为 0

**验证点**：
- [ ] 处理空列表

### 6.2 测试大量测试项
**执行命令**：
```bash
./qcloop create \
  --name "test-large" \
  --prompt "echo {{item}}" \
  --items "$(seq -s, 1 100)"
```

**预期结果**：
- 成功创建 100 个测试项
- 数据库性能正常

**验证点**：
- [ ] 创建成功
- [ ] total_items 为 100

### 6.3 测试最大质检轮次
**执行命令**：
```bash
./qcloop create \
  --name "test-max-rounds" \
  --prompt "echo {{item}}" \
  --verifier-prompt "echo '{\"pass\": false, \"feedback\": \"fail\"}'" \
  --items "max1" \
  --max-qc-rounds 10
```

**预期结果**：
- 执行 10 轮质检
- 最终标记为 exhausted

**验证点**：
- [ ] 执行 10 轮
- [ ] 状态为 exhausted

### 6.4 测试无效的 verifier JSON
**执行命令**：
```bash
./qcloop create \
  --name "test-invalid-json" \
  --prompt "echo {{item}}" \
  --verifier-prompt "echo 'invalid json'" \
  --items "invalid1"
```

**预期结果**：
- verifier 解析失败
- 标记为 fail

**验证点**：
- [ ] 处理 JSON 解析错误
- [ ] 不崩溃

## 测试用例 7: 错误处理测试

### 7.1 测试不存在的批次 ID
**执行命令**：
```bash
./qcloop status --job-id "non-existent-id"
```

**预期结果**：
- 返回错误信息
- 提示批次不存在

**验证点**：
- [ ] 错误处理正确
- [ ] 错误信息清晰

### 7.2 测试数据库文件权限
**执行命令**：
```bash
chmod 000 ~/.qcloop/qcloop.db
./qcloop status --job-id $BATCH_ID
chmod 644 ~/.qcloop/qcloop.db
```

**预期结果**：
- 返回权限错误
- 不崩溃

**验证点**：
- [ ] 错误处理正确

### 7.3 测试 API 服务器端口占用
**执行命令**：
```bash
./qcloop serve --addr :8080 &
PID1=$!
sleep 1
./qcloop serve --addr :8080
kill $PID1
```

**预期结果**：
- 第二个服务器启动失败
- 返回端口占用错误

**验证点**：
- [ ] 错误处理正确

## 测试报告模板

### 测试执行摘要
- 测试日期：____
- 测试人员：____
- 测试环境：____
- qcloop 版本：____

### 测试结果统计
| 测试用例 | 总数 | 通过 | 失败 | 跳过 |
|---------|------|------|------|------|
| CLI 基础功能 | 5 | _ | _ | _ |
| 多轮质检功能 | 3 | _ | _ | _ |
| HTTP API | 6 | _ | _ | _ |
| 前端界面 | 5 | _ | _ | _ |
| 数据库验证 | 6 | _ | _ | _ |
| 边界条件 | 4 | _ | _ | _ |
| 错误处理 | 3 | _ | _ | _ |
| **总计** | **32** | _ | _ | _ |

### 发现的问题
1. 问题描述：____
   - 严重程度：____
   - 复现步骤：____
   - 预期结果：____
   - 实际结果：____

### 测试结论
- [ ] 所有核心功能正常
- [ ] 所有 API 正常
- [ ] 前端界面正常
- [ ] 数据库操作正常
- [ ] 错误处理正常
- [ ] 建议发布

### 备注
____
