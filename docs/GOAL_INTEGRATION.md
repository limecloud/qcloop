# Codex Goal 功能集成方案

> **状态**(更新于 2026-05-10):**阶段 1 已实装** ✅
>
> 基于用户实际使用反馈,qcloop 现已交付 Goal 集成的**阶段 1:Goal-flavored
> prompt 包装**——批次可设置 `execution_mode=goal_assisted`,Runner 会把
> 每次发给 codex 的 prompt 包装成 GOAL / TASK / STOP WHEN 结构,让 Codex
> 在单次 `codex exec` 内部就尽可能朝目标收敛,同时 qcloop 外层的
> `max_qc_rounds` + `token_budget_per_item` 继续做硬停止兜底。
>
> **为什么选 prompt 包装而不是直接接 `thread/goal/*` RPC**:
> - `/goal` slash command 是 TUI 内部的,不走 `codex exec`,无法从
>   qcloop 外部直接复用
> - 接 app-server 需要维持长 thread / WebSocket client / session 恢复,
>   是独立大工程,留给阶段 2
> - 阶段 1 的 prompt 工程手法已经在多数场景下够用,零外部依赖
>
> **使用方式**:
> - CLI: `qcloop create --execution-mode goal_assisted ...`
> - API: `POST /api/jobs` 传 `"execution_mode": "goal_assisted"`
> - Web: 创建批次表单的"执行模式"下拉选 goal_assisted
>
> **下一步(阶段 2,尚未启动)**:
> - 起 codex app-server,走 JSON-RPC `thread/goal/*`
> - 维持跨 exec 的 session / thread 连续性
> - 从 goal state 拿真实的 `tokensUsed`(当前 Goal 风格 prompt 下 tokens=0)
> - 触发条件:用户提出明确需求,或 app-server API 从 experimental 转正

## 概述

Codex 的 goal 功能允许设置一个持久化的目标，Codex 会自主循环执行直到达成目标。这与 qcloop 的多轮质检理念完美契合。

## Codex Goal 核心特性

### 1. 设置 Goal
```json
{
  "method": "thread/goal/set",
  "params": {
    "threadId": "thr_123",
    "objective": "完成测试项 X，确保通过所有质检",
    "tokenBudget": 50000
  }
}
```

### 2. Goal 状态
- `active` - 正在执行
- `paused` - 已暂停
- `completed` - 已完成
- `failed` - 失败

### 3. Token 预算
- 设置 `tokenBudget` 限制最大消耗
- 实时追踪 `tokensUsed`
- 超出预算自动停止

## 集成方案

### 方案 A：Goal 驱动的完整自主执行

**优点**：
- Codex 完全自主，无需人工干预
- 自动处理 worker -> verifier -> repair 循环
- 智能决策何时停止

**缺点**：
- 失去对执行流程的精确控制
- 难以追踪每一轮的详细结果
- 可能消耗大量 token

**实现**：
```go
// 为每个 item 创建一个 goal
objective := fmt.Sprintf(`
完成测试项: %s

要求：
1. 执行测试任务
2. 运行质检验证
3. 如果质检失败，根据反馈修复
4. 重复直到质检通过或达到 %d 轮

质检标准：
%s
`, item.ItemValue, job.MaxQCRounds, job.VerifierPromptTemplate)

// 设置 goal
goal := codex.SetGoal(threadID, objective, tokenBudget)

// 等待 goal 完成
result := codex.WaitForGoal(threadID)
```

### 方案 B：Goal 辅助的半自主执行（推荐）

**优点**：
- 保留 qcloop 的流程控制
- 每一轮都有明确的记录
- 可以精确追踪 attempts 和 qc_rounds
- 更容易调试和监控

**缺点**：
- 需要更多的编排代码
- 不是完全自主

**实现**：
```go
// 每一轮使用 goal 来执行单次任务
for qcNo := 1; qcNo <= job.MaxQCRounds; qcNo++ {
    // Worker: 使用 goal 执行任务
    workerGoal := fmt.Sprintf(`
    执行测试项: %s
    
    任务：%s
    
    要求：完成任务并输出结果
    `, item.ItemValue, job.PromptTemplate)
    
    workerResult := codex.ExecuteGoal(workerGoal, workerTokenBudget)
    
    // 保存 attempt
    saveAttempt(workerResult)
    
    // Verifier: 使用 goal 执行质检
    verifierGoal := fmt.Sprintf(`
    质检测试结果
    
    测试项: %s
    输出: %s
    
    质检标准：%s
    
    要求：输出 JSON 格式的判定结果
    {"pass": bool, "feedback": string}
    `, item.ItemValue, workerResult.Output, job.VerifierPromptTemplate)
    
    verifierResult := codex.ExecuteGoal(verifierGoal, verifierTokenBudget)
    
    // 保存 qc_round
    saveQCRound(verifierResult)
    
    // 判断是否通过
    if verifierResult.Pass {
        return success
    }
    
    // 如果未通过，继续下一轮（repair）
}
```

### 方案 C：混合模式

**优点**：
- 结合两者优势
- 灵活性最高

**实现**：
```go
// 用户可以选择执行模式
if job.ExecutionMode == "autonomous" {
    // 使用方案 A：完全自主
    return executeWithFullGoal(item, job)
} else {
    // 使用方案 B：半自主
    return executeWithStepGoal(item, job)
}
```

## 技术实现

### 1. 创建 GoalExecutor

```go
package executor

import (
    "context"
    "encoding/json"
    "fmt"
)

type GoalExecutor struct {
    appServerURL string
    threadID     string
}

func NewGoalExecutor() *GoalExecutor {
    return &GoalExecutor{
        appServerURL: "ws://localhost:8765", // Codex app-server
    }
}

// SetGoal 设置持久化目标
func (e *GoalExecutor) SetGoal(ctx context.Context, objective string, tokenBudget int) (*Goal, error) {
    request := map[string]interface{}{
        "method": "thread/goal/set",
        "id":     1,
        "params": map[string]interface{}{
            "threadId":    e.threadID,
            "objective":   objective,
            "tokenBudget": tokenBudget,
        },
    }
    
    // 发送 JSON-RPC 请求到 app-server
    response := e.sendRequest(request)
    
    return parseGoal(response)
}

// GetGoal 获取当前目标状态
func (e *GoalExecutor) GetGoal(ctx context.Context) (*Goal, error) {
    request := map[string]interface{}{
        "method": "thread/goal/get",
        "id":     2,
        "params": map[string]interface{}{
            "threadId": e.threadID,
        },
    }
    
    response := e.sendRequest(request)
    return parseGoal(response)
}

// WaitForGoal 等待目标完成
func (e *GoalExecutor) WaitForGoal(ctx context.Context) (*GoalResult, error) {
    for {
        goal, err := e.GetGoal(ctx)
        if err != nil {
            return nil, err
        }
        
        if goal.Status == "completed" || goal.Status == "failed" {
            return &GoalResult{
                Status:      goal.Status,
                TokensUsed:  goal.TokensUsed,
                TimeUsed:    goal.TimeUsedSeconds,
            }, nil
        }
        
        // 等待一段时间后再次检查
        time.Sleep(5 * time.Second)
    }
}

type Goal struct {
    ThreadID         string `json:"threadId"`
    Objective        string `json:"objective"`
    Status           string `json:"status"`
    TokenBudget      int    `json:"tokenBudget"`
    TokensUsed       int    `json:"tokensUsed"`
    TimeUsedSeconds  int    `json:"timeUsedSeconds"`
    CreatedAt        int64  `json:"createdAt"`
    UpdatedAt        int64  `json:"updatedAt"`
}

type GoalResult struct {
    Status     string
    TokensUsed int
    TimeUsed   int
}
```

### 2. 修改 Runner 使用 GoalExecutor

```go
// 在 runner.go 中
func (r *Runner) processItemWithGoal(ctx context.Context, job *BatchJob, item *BatchItem) error {
    goalExecutor := executor.NewGoalExecutor()
    
    // 构造目标
    objective := fmt.Sprintf(`
完成测试项: %s

任务描述：
%s

质检标准：
%s

要求：
1. 执行测试任务
2. 运行质检验证（输出 JSON: {"pass": bool, "feedback": string}）
3. 如果质检失败，根据反馈修复
4. 重复直到质检通过或达到 %d 轮
5. 记录每一轮的执行结果

最终输出格式：
{
  "success": bool,
  "rounds": int,
  "final_output": string,
  "qc_results": [...]
}
`, item.ItemValue, job.PromptTemplate, job.VerifierPromptTemplate, job.MaxQCRounds)
    
    // 设置 goal
    tokenBudget := 50000 // 每个 item 的 token 预算
    goal, err := goalExecutor.SetGoal(ctx, objective, tokenBudget)
    if err != nil {
        return err
    }
    
    // 等待 goal 完成
    result, err := goalExecutor.WaitForGoal(ctx)
    if err != nil {
        return err
    }
    
    // 解析结果并保存到数据库
    return r.saveGoalResult(item, result)
}
```

## 配置选项

在 `batch_jobs` 表中添加新字段：

```sql
ALTER TABLE batch_jobs ADD COLUMN execution_mode TEXT DEFAULT 'standard';
-- 'standard': 当前的 worker->verifier->repair 流程
-- 'goal_autonomous': 完全自主的 goal 模式
-- 'goal_assisted': goal 辅助的半自主模式

ALTER TABLE batch_jobs ADD COLUMN token_budget_per_item INTEGER DEFAULT 50000;
-- 每个 item 的 token 预算
```

## CLI 使用示例

```bash
# 使用标准模式（当前）
qcloop create \
  --name "test" \
  --prompt "..." \
  --verifier-prompt "..." \
  --items "a,b,c"

# 使用 goal 自主模式
qcloop create \
  --name "test" \
  --prompt "..." \
  --verifier-prompt "..." \
  --items "a,b,c" \
  --execution-mode goal-autonomous \
  --token-budget 50000

# 使用 goal 辅助模式
qcloop create \
  --name "test" \
  --prompt "..." \
  --verifier-prompt "..." \
  --items "a,b,c" \
  --execution-mode goal-assisted \
  --token-budget 30000
```

## 优势分析

### 使用 Codex Goal 的优势

1. **真正的自主执行**
   - Codex 会自己决定何时停止
   - 不需要预设固定的轮次
   - 更智能的决策

2. **更好的上下文保持**
   - Goal 在整个执行过程中保持上下文
   - 不会因为多次调用而丢失信息

3. **Token 预算控制**
   - 精确控制每个 item 的最大消耗
   - 避免无限循环

4. **更自然的交互**
   - Codex 可以自己判断是否需要更多信息
   - 可以主动提出改进建议

### 与当前方案的对比

| 特性 | 当前方案 | Goal 方案 |
|------|---------|-----------|
| 控制精度 | 高 | 中 |
| 自主程度 | 低 | 高 |
| Token 效率 | 中 | 高 |
| 调试难度 | 低 | 中 |
| 灵活性 | 中 | 高 |
| 实现复杂度 | 低 | 中 |

## 实施建议

### 阶段 1：调研验证（1-2 天）
- [ ] 搭建 Codex app-server
- [ ] 测试 goal API 的基本功能
- [ ] 验证 goal 的执行效果

### 阶段 2：原型实现（3-5 天）
- [ ] 实现 GoalExecutor
- [ ] 实现方案 B（半自主模式）
- [ ] 集成到 qcloop

### 阶段 3：完整实现（5-7 天）
- [ ] 实现方案 A（完全自主模式）
- [ ] 添加配置选项
- [ ] 完善错误处理
- [ ] 编写测试用例

### 阶段 4：优化改进（持续）
- [ ] 性能优化
- [ ] Token 消耗优化
- [ ] 用户体验改进

## 参考资料

- [Codex Goal Feature Review](https://www.jdhodges.com/blog/codex-goal-feature-review/)
- [Codex /goal: What It Does, How to Use It](https://blog.laozhang.ai/en/posts/codex-goal)
- [Codex Goal Mode Autonomous Task Guide](https://help.apiyi.com/en/codex-goal-mode-autonomous-task-guide-en.html)
- [Codex /goal Practical Guide](https://smartscope.blog/en/generative-ai/chatgpt/codex-goal-practical-guide/)
- [Simon Willison: Codex CLI 0.128.0 adds /goal](https://simonwillison.net/2026/Apr/30/codex-goals/)

## 总结

Codex Goal 功能为 qcloop 提供了一个强大的升级路径。通过结合 goal 的自主执行能力和 qcloop 的编排能力，我们可以实现：

1. **更智能的多轮质检** - Codex 自己决定何时停止
2. **更高的 token 效率** - 保持上下文，减少重复
3. **更好的用户体验** - 设置目标后自动完成
4. **更灵活的控制** - 支持多种执行模式

建议优先实现**方案 B（半自主模式）**，在保留当前流程控制的同时，引入 goal 的优势。
