package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

// EventBroadcaster 是 Runner 用来推实时事件的 seam。
// internal/api.WSHub 实现了这个接口;测试可传 nil。
//
// 为什么定义在 core 而不是 api:
//   - 避免 core → api 反向依赖(api 已经 import core.Runner)
//   - 测试时注入 nil 或 fake,核心流程不依赖 WebSocket 实现
type EventBroadcaster interface {
	BroadcastItemUpdate(jobID, itemID string, data interface{})
	BroadcastJobUpdate(jobID string, data interface{})
}

const (
	RunModeAuto            = "auto"
	RunModeContinue        = "continue"
	RunModeRetryUnfinished = "retry_unfinished"
	RunModeRerunAll        = "rerun_all"
)

// Runner 批次执行器
type Runner struct {
	database        *db.DB
	executor        executor.Executor // 测试/嵌入场景的固定 executor;nil 时按 job provider 创建
	executorFactory func(provider string) (executor.Executor, error)
	// broadcaster 可为 nil;非 nil 时 Runner 会在关键事件推 WebSocket
	broadcaster EventBroadcaster
}

// NewRunner 创建执行器。实际底层 CLI 按 batch_job.executor_provider 选择。
func NewRunner(database *db.DB) *Runner {
	return &Runner{
		database:        database,
		executorFactory: executor.NewExecutorForProvider,
	}
}

// NewRunnerWithExecutor 用指定 executor 创建 Runner(测试用)
func NewRunnerWithExecutor(database *db.DB, exec executor.Executor) *Runner {
	return &Runner{
		database: database,
		executor: exec,
	}
}

// SetBroadcaster 注入事件广播器。传 nil 可关闭广播(默认行为)。
// 调用时机:api.Server 构造时注入 WSHub。
func (r *Runner) SetBroadcaster(b EventBroadcaster) {
	r.broadcaster = b
}

// emitItemUpdate 非阻塞广播单个 item 的最新快照。
// broadcaster 为 nil 时无操作,测试/CLI 路径不受影响。
// payload 从 DB 重读,避免广播陈旧数据。
func (r *Runner) emitItemUpdate(jobID, itemID string) {
	if r.broadcaster == nil {
		return
	}
	item, err := r.database.GetItem(itemID)
	if err != nil || item == nil {
		return
	}
	attempts, _ := r.listAttempts(itemID)
	qcRounds, _ := r.listQCRounds(itemID)
	r.broadcaster.BroadcastItemUpdate(jobID, itemID, map[string]interface{}{
		"item":      item,
		"attempts":  attempts,
		"qc_rounds": qcRounds,
	})
}

// emitJobUpdate 非阻塞广播 job 状态
func (r *Runner) emitJobUpdate(jobID string) {
	if r.broadcaster == nil {
		return
	}
	job, err := r.database.GetJob(jobID)
	if err != nil || job == nil {
		return
	}
	r.broadcaster.BroadcastJobUpdate(jobID, job)
}

// EmitItemUpdate 对 API 层暴露一次 item 快照广播,用于 answer/resume 等非 Runner 内部动作。
func (r *Runner) EmitItemUpdate(jobID, itemID string) {
	r.emitItemUpdate(jobID, itemID)
}

// EmitJobUpdate 对 API 层暴露一次 job 快照广播。
func (r *Runner) EmitJobUpdate(jobID string) {
	r.emitJobUpdate(jobID)
}

// setItemStatus 更新 item 状态并广播。集中一处避免忘记广播。
func (r *Runner) setItemStatus(jobID, itemID, status string) error {
	var err error
	switch status {
	case "success", "failed", "exhausted":
		err = r.database.FinishItem(itemID, status)
	default:
		err = r.database.UpdateItemStatus(itemID, status)
	}
	if err != nil {
		return err
	}
	r.emitItemUpdate(jobID, itemID)
	return nil
}

// PrepareRun 同步把批次切到"本次运行"起点,兼容旧调用。
//
// API 会在返回前调用它,保证用户点击"重新运行"后立刻看到:
// - job.status=running,finished_at 清空
// - terminal 批次的 item 回到 pending/running 队列
// - 历史 attempts/qc_rounds 保留,但当前轮计数归零
func (r *Runner) PrepareRun(jobID string) (*db.BatchJob, []*db.BatchItem, error) {
	return r.PrepareRunMode(jobID, RunModeAuto)
}

// PrepareRunMode 按运行模式准备队列状态。
//
// mode:
//   - auto:新批次继续 pending;有未成功项时重试未成功项;全成功终态批次重跑全部
//   - continue:只恢复 pending/stale running,不递增 run_no
//   - retry_unfinished:递增 run_no,仅非 success item 入队
//   - rerun_all:递增 run_no,全部 item 入队
func (r *Runner) PrepareRunMode(jobID, mode string) (*db.BatchJob, []*db.BatchItem, error) {
	job, err := r.database.GetJob(jobID)
	if err != nil {
		return nil, nil, err
	}
	if job == nil {
		return nil, nil, fmt.Errorf("批次不存在")
	}

	items, err := r.database.ListItems(jobID)
	if err != nil {
		return nil, nil, err
	}
	mode = normalizeRunMode(mode)
	if mode == RunModeAuto {
		mode = chooseAutoRunMode(job, items)
	}
	if job.Status == "running" && mode != RunModeContinue {
		return nil, nil, fmt.Errorf("running job can only continue")
	}
	if job.Status == "canceled" {
		return nil, nil, fmt.Errorf("canceled job cannot be run")
	}
	emitPreparedItems := mode != RunModeContinue

	switch mode {
	case RunModeRetryUnfinished:
		if err := r.database.IncrementJobRunNo(jobID); err != nil {
			return nil, nil, err
		}
		if err := r.database.ResetItemsForRerunMode(jobID, "unfinished"); err != nil {
			return nil, nil, err
		}
	case RunModeRerunAll:
		if err := r.database.IncrementJobRunNo(jobID); err != nil {
			return nil, nil, err
		}
		if err := r.database.ResetItemsForRerunMode(jobID, "all"); err != nil {
			return nil, nil, err
		}
	case RunModeContinue:
		if err := r.database.QueuePendingItems(jobID); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("unsupported run mode: %s", mode)
	}

	if err := r.database.StartJob(jobID); err != nil {
		return nil, nil, err
	}

	job, err = r.database.GetJob(jobID)
	if err != nil {
		return nil, nil, err
	}
	items, err = r.database.ListItems(jobID)
	if err != nil {
		return nil, nil, err
	}
	r.emitJobUpdate(jobID)
	if emitPreparedItems {
		for _, item := range items {
			r.emitItemUpdate(jobID, item.ID)
		}
	}

	return job, items, nil
}

func normalizeRunMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", RunModeAuto:
		return RunModeAuto
	case RunModeContinue:
		return RunModeContinue
	case RunModeRetryUnfinished:
		return RunModeRetryUnfinished
	case RunModeRerunAll:
		return RunModeRerunAll
	default:
		return mode
	}
}

func chooseAutoRunMode(job *db.BatchJob, items []*db.BatchItem) string {
	if job.Status == "completed" || job.Status == "failed" {
		if allItemsSucceeded(items) {
			return RunModeRerunAll
		}
		return RunModeRetryUnfinished
	}
	return RunModeContinue
}

func allItemsSucceeded(items []*db.BatchItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.Status != "success" {
			return false
		}
	}
	return true
}

// RunBatch 运行批次
func (r *Runner) RunBatch(ctx context.Context, jobID string) error {
	job, items, err := r.PrepareRun(jobID)
	if err != nil {
		return err
	}

	for _, item := range items {
		if item.Status != "pending" {
			continue
		}

		if err := r.processItem(ctx, job, item); err != nil {
			return err
		}
	}

	if _, _, err := r.database.FinishJobIfDone(jobID); err != nil {
		return err
	}
	r.emitJobUpdate(jobID)
	return nil
}

// processItem 处理单个 item
func (r *Runner) processItem(ctx context.Context, job *db.BatchJob, item *db.BatchItem) error {
	activeExec, err := r.buildActiveExecutor(job)
	if err != nil {
		return err
	}
	if err := r.setItemStatus(job.ID, item.ID, "running"); err != nil {
		return err
	}

	// 执行 worker
	attemptNo, err := r.nextAttemptNo(item.ID)
	if err != nil {
		return err
	}
	attempt, attemptNo, err := r.executeWorkerWithRetries(ctx, activeExec, job, item, attemptNo, "worker")
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		_ = r.requeueItemIfJobStillRunnable(job.ID, item.ID, "execution canceled")
		r.emitItemUpdate(job.ID, item.ID)
		return ctx.Err()
	}
	if isRetryableExecutorAttempt(attempt) {
		return r.setItemStatus(job.ID, item.ID, "failed")
	}

	// 如果没有配置 verifier，直接标记为成功
	if job.VerifierPromptTemplate == "" {
		status := "success"
		if attempt.Status != "success" {
			status = "failed"
		}
		return r.setItemStatus(job.ID, item.ID, status)
	}

	// 执行多轮质检
	startQCNo, err := r.nextQCNo(item.ID)
	if err != nil {
		return err
	}
	return r.runQCLoop(ctx, activeExec, job, item, attemptNo, startQCNo)
}

// runOne 统一封装 executor 调用,返回 Result(含 tokens)。
// 若 exec 实现了 TokenAwareExecutor 则走增强接口,否则走 legacy Execute 并 tokens=0。
func (r *Runner) runOne(ctx context.Context, exec executor.Executor, prompt string) executor.Result {
	if exec == nil {
		exec = executor.NewCodexExecutor() // 兜底,不应发生
	}
	if tae, ok := exec.(executor.TokenAwareExecutor); ok {
		return tae.ExecuteWithTokens(ctx, prompt)
	}
	stdout, stderr, code, err := exec.Execute(ctx, prompt)
	if err != nil {
		if stderr != "" {
			stderr += "\n"
		}
		stderr += err.Error()
		if code == 0 {
			code = -1
		}
	}
	return executor.Result{Stdout: stdout, Stderr: stderr, ExitCode: code, TokensUsed: 0}
}

// buildActiveExecutor 先按 job.ExecutorProvider 选择 CLI,再按 ExecutionMode 包装 prompt。
func (r *Runner) buildActiveExecutor(job *db.BatchJob) (executor.Executor, error) {
	baseExec, err := r.baseExecutorForJob(job)
	if err != nil {
		return nil, err
	}
	switch job.ExecutionMode {
	case "goal_assisted":
		// 用 job 的 prompt 模板作为 goal hint(剥离 {{item}} 占位符让 Codex
		// 更好理解整体目标),stop hint 留空走默认。
		goalHint := "按以下模板完成每个测试项:\n\n" + job.PromptTemplate
		if job.VerifierPromptTemplate != "" {
			goalHint += "\n\n每次完成后应能通过质检:\n" + job.VerifierPromptTemplate
		}
		return executor.NewGoalAssistedExecutor(baseExec, goalHint, ""), nil
	default: // "" 或 "standard"
		return baseExec, nil
	}
}

func (r *Runner) baseExecutorForJob(job *db.BatchJob) (executor.Executor, error) {
	if r.executor != nil {
		return r.executor, nil
	}
	provider := job.ExecutorProvider
	if provider == "" {
		provider = executor.ProviderCodex
	}
	factory := r.executorFactory
	if factory == nil {
		factory = executor.NewExecutorForProvider
	}
	return factory(provider)
}

// chargeItemTokens 把本次调用的 tokens 加到 item 上,返回加完后的总量。
// token_budget_per_item <= 0 视为 "不限制"。
func (r *Runner) chargeItemTokens(itemID string, delta int) (int, error) {
	if delta <= 0 {
		// 读一下当前值即可
		var used int
		err := r.database.Conn().QueryRow(`SELECT tokens_used FROM batch_items WHERE id = ?`, itemID).Scan(&used)
		return used, err
	}
	if _, err := r.database.Conn().Exec(`UPDATE batch_items SET tokens_used = tokens_used + ? WHERE id = ?`, delta, itemID); err != nil {
		return 0, err
	}
	var used int
	err := r.database.Conn().QueryRow(`SELECT tokens_used FROM batch_items WHERE id = ?`, itemID).Scan(&used)
	return used, err
}

// executeWorker 执行 worker 或 repair
func (r *Runner) executeWorker(ctx context.Context, exec executor.Executor, job *db.BatchJob, item *db.BatchItem, attemptNo int, attemptType string) (*db.Attempt, error) {
	prompt := renderWorkerPrompt(job.PromptTemplate, item)
	return r.executeWorkerPrompt(ctx, exec, prompt, item, job.RunNo, attemptNo, attemptType)
}

func (r *Runner) executeWorkerWithRetries(ctx context.Context, exec executor.Executor, job *db.BatchJob, item *db.BatchItem, attemptNo int, attemptType string) (*db.Attempt, int, error) {
	prompt := renderWorkerPrompt(job.PromptTemplate, item)
	return r.executeWorkerPromptWithRetries(ctx, exec, job, item, prompt, attemptNo, attemptType)
}

func (r *Runner) executeWorkerPromptWithRetries(ctx context.Context, exec executor.Executor, job *db.BatchJob, item *db.BatchItem, basePrompt string, attemptNo int, attemptType string) (*db.Attempt, int, error) {
	retries := job.MaxExecutorRetries
	if retries < 0 {
		retries = 0
	}
	if retries > 5 {
		retries = 5
	}
	currentAttemptNo := attemptNo
	var last *db.Attempt
	for retryIndex := 0; ; retryIndex++ {
		prompt := basePrompt
		if retryIndex > 0 {
			prompt = appendExecutorRetryContext(prompt, retryIndex, retries)
		}
		attempt, err := r.executeWorkerPrompt(ctx, exec, prompt, item, job.RunNo, currentAttemptNo, attemptType)
		if err != nil {
			return nil, currentAttemptNo, err
		}
		last = attempt
		if ctx.Err() != nil || !isRetryableExecutorAttempt(attempt) || retryIndex >= retries {
			return last, currentAttemptNo, nil
		}
		currentAttemptNo++
	}
}

func appendExecutorRetryContext(prompt string, retryIndex, maxRetries int) string {
	return fmt.Sprintf(`%s

---
qcloop 自动重试上下文:
- 上一次调用本机 AI CLI 出现执行器基础设施错误,这是第 %d/%d 次自动重试。
- 请继续完成同一个 item,不要因为重试而改变任务边界。`, prompt, retryIndex, maxRetries)
}

func isRetryableExecutorAttempt(attempt *db.Attempt) bool {
	return attempt != nil && attempt.ExitCode != nil && *attempt.ExitCode < 0
}

func (r *Runner) executeWorkerPrompt(ctx context.Context, exec executor.Executor, prompt string, item *db.BatchItem, runNo, attemptNo int, attemptType string) (*db.Attempt, error) {
	attempt := &db.Attempt{
		ID:          db.GenerateID(),
		BatchItemID: item.ID,
		AttemptNo:   attemptNo,
		RunNo:       runNo,
		AttemptType: attemptType,
		Status:      "running",
		StartedAt:   time.Now(),
	}

	if err := r.createAttempt(attempt); err != nil {
		return nil, err
	}

	res := r.runOne(ctx, exec, prompt)

	finishedAt := time.Now()
	attempt.FinishedAt = &finishedAt
	attempt.Stdout = res.Stdout
	attempt.Stderr = res.Stderr
	attempt.ExitCode = &res.ExitCode
	attempt.TokensUsed = res.TokensUsed

	if res.ExitCode != 0 {
		attempt.Status = "failed"
	} else {
		attempt.Status = "success"
	}

	if err := r.updateAttempt(attempt); err != nil {
		return nil, err
	}

	if _, err := r.chargeItemTokens(item.ID, res.TokensUsed); err != nil {
		return nil, err
	}

	// 每次 attempt 落库后广播,前端立刻看到"首次/质检 N"标签增长
	r.emitItemUpdate(item.BatchJobID, item.ID)

	return attempt, nil
}

// runQCLoop 运行质检循环
func (r *Runner) runQCLoop(ctx context.Context, exec executor.Executor, job *db.BatchJob, item *db.BatchItem, startAttemptNo, startQCNo int) error {
	attemptNo := startAttemptNo

	for offset := 0; offset < job.MaxQCRounds; offset++ {
		if ctx.Err() != nil {
			_ = r.requeueItemIfJobStillRunnable(job.ID, item.ID, "execution canceled")
			r.emitItemUpdate(job.ID, item.ID)
			return ctx.Err()
		}
		qcNo := startQCNo + offset
		// 每轮开始检查 token 预算
		if exceeded, err := r.checkBudgetExceeded(job, item); err != nil {
			return err
		} else if exceeded {
			return r.setItemStatus(job.ID, item.ID, "exhausted")
		}

		// 执行 verifier
		qcRound, err := r.executeVerifier(ctx, exec, job, item, qcNo)
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			_ = r.requeueItemIfJobStillRunnable(job.ID, item.ID, "execution canceled")
			r.emitItemUpdate(job.ID, item.ID)
			return ctx.Err()
		}

		if qcRound.Status == "pass" {
			// 质检通过
			return r.setItemStatus(job.ID, item.ID, "success")
		}
		if qcRound.NeedsConfirmation {
			question := qcRound.Question
			if question == "" {
				question = qcRound.Feedback
			}
			if question == "" {
				question = "verifier 请求外层 AI 获取人类确认"
			}
			if err := r.database.MarkItemAwaitingConfirmation(item.ID, question); err != nil {
				return err
			}
			r.emitItemUpdate(job.ID, item.ID)
			return nil
		}

		// 质检失败
		if offset >= job.MaxQCRounds-1 {
			// 达到最大轮次
			return r.setItemStatus(job.ID, item.ID, "exhausted")
		}

		// 执行 repair 前再检查一次(verifier 本身可能已超预算)
		if exceeded, err := r.checkBudgetExceeded(job, item); err != nil {
			return err
		} else if exceeded {
			return r.setItemStatus(job.ID, item.ID, "exhausted")
		}

		// 执行 repair
		attemptNo++
		repairAttempt, usedAttemptNo, err := r.executeRepair(ctx, exec, job, item, attemptNo, qcRound.Feedback)
		if err != nil {
			return err
		}
		attemptNo = usedAttemptNo
		if ctx.Err() != nil {
			_ = r.requeueItemIfJobStillRunnable(job.ID, item.ID, "execution canceled")
			r.emitItemUpdate(job.ID, item.ID)
			return ctx.Err()
		}

		if repairAttempt.Status != "success" {
			// repair 失败，标记为 failed
			return r.setItemStatus(job.ID, item.ID, "failed")
		}
	}

	return nil
}

func (r *Runner) requeueItemIfJobStillRunnable(jobID, itemID, reason string) error {
	job, err := r.database.GetJob(jobID)
	if err != nil {
		return err
	}
	if job != nil && job.Status == "canceled" {
		return nil
	}
	item, err := r.database.GetItem(itemID)
	if err != nil {
		return err
	}
	if item != nil && item.Status == "canceled" {
		return nil
	}
	return r.database.RequeueItem(itemID, reason)
}

// checkBudgetExceeded:预算 > 0 且 item.tokens_used 已达到上限,则返回 true
func (r *Runner) checkBudgetExceeded(job *db.BatchJob, item *db.BatchItem) (bool, error) {
	if job.TokenBudgetPerItem <= 0 {
		return false, nil
	}
	var used int
	err := r.database.Conn().QueryRow(`SELECT tokens_used FROM batch_items WHERE id = ?`, item.ID).Scan(&used)
	if err != nil {
		return false, err
	}
	return used >= job.TokenBudgetPerItem, nil
}

func (r *Runner) nextAttemptNo(itemID string) (int, error) {
	attempts, err := r.listAttempts(itemID)
	if err != nil {
		return 0, err
	}
	maxNo := 0
	for _, attempt := range attempts {
		if attempt.AttemptNo > maxNo {
			maxNo = attempt.AttemptNo
		}
	}
	return maxNo + 1, nil
}

func (r *Runner) nextQCNo(itemID string) (int, error) {
	rounds, err := r.listQCRounds(itemID)
	if err != nil {
		return 0, err
	}
	maxNo := 0
	for _, round := range rounds {
		if round.QCNo > maxNo {
			maxNo = round.QCNo
		}
	}
	return maxNo + 1, nil
}

// executeVerifier 执行 verifier
func (r *Runner) executeVerifier(ctx context.Context, exec executor.Executor, job *db.BatchJob, item *db.BatchItem, qcNo int) (*db.QCRound, error) {
	// 获取最新的 attempt 输出
	attempts, err := r.listAttempts(item.ID)
	if err != nil {
		return nil, err
	}
	if len(attempts) == 0 {
		return nil, fmt.Errorf("没有找到执行记录")
	}

	lastAttempt := attempts[len(attempts)-1]

	// 构造 verifier prompt
	previousRounds, err := r.listQCRounds(item.ID)
	if err != nil {
		return nil, err
	}
	prompt := renderVerifierPrompt(job.VerifierPromptTemplate, item.ItemValue, lastAttempt, previousRounds)

	qcRound := &db.QCRound{
		ID:          db.GenerateID(),
		BatchItemID: item.ID,
		QCNo:        qcNo,
		RunNo:       job.RunNo,
		Status:      "running",
		StartedAt:   time.Now(),
	}

	if err := r.createQCRound(qcRound); err != nil {
		return nil, err
	}

	res := r.runOne(ctx, exec, prompt)
	stdout := res.Stdout
	qcRound.TokensUsed = res.TokensUsed

	finishedAt := time.Now()
	qcRound.FinishedAt = &finishedAt

	// 解析 verdict（期望 JSON 格式）
	var verdict map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &verdict); err != nil {
		// 解析失败，认为质检失败
		qcRound.Status = "fail"
		qcRound.Feedback = "verifier 输出格式错误"
	} else {
		pass, ok := verdict["pass"].(bool)
		if ok && pass {
			qcRound.Status = "pass"
		} else {
			qcRound.Status = "fail"
			if needsConfirmation, ok := verdict["needs_confirmation"].(bool); ok && needsConfirmation {
				qcRound.NeedsConfirmation = true
			}
			if question, ok := verdict["question"].(string); ok {
				qcRound.Question = strings.TrimSpace(question)
			}
			if feedback, ok := verdict["feedback"].(string); ok {
				qcRound.Feedback = feedback
			}
			if qcRound.NeedsConfirmation && qcRound.Feedback == "" {
				qcRound.Feedback = qcRound.Question
			}
		}
		qcRound.Verdict = stdout
	}

	if err := r.updateQCRound(qcRound); err != nil {
		return nil, err
	}

	if _, err := r.chargeItemTokens(item.ID, qcRound.TokensUsed); err != nil {
		return nil, err
	}

	// qc_round 落库后立即广播,质检标签实时刷新
	r.emitItemUpdate(item.BatchJobID, item.ID)

	return qcRound, nil
}

// executeRepair 执行 repair
func (r *Runner) executeRepair(ctx context.Context, exec executor.Executor, job *db.BatchJob, item *db.BatchItem, attemptNo int, feedback string) (*db.Attempt, int, error) {
	attempts, err := r.listAttempts(item.ID)
	if err != nil {
		return nil, attemptNo, err
	}
	if len(attempts) == 0 {
		return nil, attemptNo, fmt.Errorf("没有找到执行记录")
	}
	lastAttempt := attempts[len(attempts)-1]
	prompt := buildRepairPrompt(job.PromptTemplate, item, lastAttempt, feedback)

	return r.executeWorkerPromptWithRetries(ctx, exec, job, item, prompt, attemptNo, "repair")
}

func renderVerifierPrompt(template, itemValue string, attempt *db.Attempt, previousRounds []*db.QCRound) string {
	prompt := strings.ReplaceAll(template, "{{item}}", itemValue)
	prompt = strings.ReplaceAll(prompt, "{{output}}", attempt.Stdout)
	prompt = strings.ReplaceAll(prompt, "{{stdout}}", attempt.Stdout)
	prompt = strings.ReplaceAll(prompt, "{{stderr}}", attempt.Stderr)
	prompt = strings.ReplaceAll(prompt, "{{exit_code}}", formatExitCode(attempt.ExitCode))
	prompt = strings.ReplaceAll(prompt, "{{attempt_status}}", attempt.Status)
	prompt = strings.ReplaceAll(prompt, "{{attempt_type}}", attempt.AttemptType)
	history := formatQCHistoryForPrompt(previousRounds)
	prompt = strings.ReplaceAll(prompt, "{{qc_history}}", history)
	prompt = strings.ReplaceAll(prompt, "{{issue_ledger}}", history)
	if history != "" && !strings.Contains(template, "{{qc_history}}") && !strings.Contains(template, "{{issue_ledger}}") {
		prompt = fmt.Sprintf(`%s

---
qcloop 质检历史 / issue ledger:
%s

请结合上面的历史判断旧问题是否已经修复、是否出现新的未解决问题。只有没有开放问题时才输出 {"pass": true, "feedback": "..."}。`, prompt, history)
	}
	return prompt
}

func formatQCHistoryForPrompt(rounds []*db.QCRound) string {
	if len(rounds) == 0 {
		return ""
	}
	var b strings.Builder
	for _, round := range rounds {
		status := round.Status
		if status == "" {
			status = "unknown"
		}
		feedback := strings.TrimSpace(round.Feedback)
		if feedback == "" {
			feedback = strings.TrimSpace(round.Verdict)
		}
		if feedback == "" {
			feedback = "(无反馈)"
		}
		b.WriteString(fmt.Sprintf("- 质检 %d: %s; %s\n", round.QCNo, status, clipForPrompt(feedback)))
	}
	return strings.TrimSpace(b.String())
}

func renderWorkerPrompt(template string, item *db.BatchItem) string {
	basePrompt := strings.ReplaceAll(template, "{{item}}", item.ItemValue)
	if strings.TrimSpace(item.ConfirmationAnswer) == "" {
		return basePrompt
	}
	return fmt.Sprintf(`%s

---
qcloop 确认上下文:
- 待确认问题: %s
- 人类确认答案: %s

请基于这份确认继续执行当前 item,不要再把同一问题交还给人类。`,
		basePrompt,
		clipForPrompt(item.ConfirmationQuestion),
		clipForPrompt(item.ConfirmationAnswer),
	)
}

func buildRepairPrompt(template string, item *db.BatchItem, attempt *db.Attempt, feedback string) string {
	basePrompt := renderWorkerPrompt(template, item)
	return fmt.Sprintf(`%s

---
这是 qcloop 的 repair 轮次。上一轮测试或质检没有通过,请根据证据直接修复目标工作区中的问题。

测试项:
%s

上一轮执行:
- 类型: %s
- 状态: %s
- 退出码: %s

上一轮 stdout:
%s

上一轮 stderr:
%s

质检反馈：%s

修复要求:
1. 先定位根因,再做最小必要修改;不要绕过测试或降低断言。
2. 修复后重新运行与该测试项相关的最小验证命令。
3. 最终输出修改文件、验证命令、验证结果和剩余风险。
4. 除非人类明确授权,不要执行 git commit / git push / git reset。`,
		basePrompt,
		item.ItemValue,
		attempt.AttemptType,
		attempt.Status,
		formatExitCode(attempt.ExitCode),
		clipForPrompt(attempt.Stdout),
		clipForPrompt(attempt.Stderr),
		feedback,
	)
}

func formatExitCode(exitCode *int) string {
	if exitCode == nil {
		return "未知"
	}
	return fmt.Sprintf("%d", *exitCode)
}

func clipForPrompt(value string) string {
	const maxPromptEvidenceBytes = 12000
	if len(value) <= maxPromptEvidenceBytes {
		return value
	}
	return value[:maxPromptEvidenceBytes] + "\n...[已截断,请按需重新运行相关命令获取完整日志]"
}

// 数据库操作辅助方法
func (r *Runner) createAttempt(attempt *db.Attempt) error {
	if attempt.RunNo <= 0 {
		attempt.RunNo = 1
	}
	query := `INSERT INTO attempts (id, batch_item_id, attempt_no, run_no, attempt_type, status, started_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.database.Conn().Exec(query, attempt.ID, attempt.BatchItemID, attempt.AttemptNo, attempt.RunNo, attempt.AttemptType, attempt.Status, attempt.StartedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := r.database.Conn().Exec(`UPDATE batch_items SET current_attempt_no = current_attempt_no + 1 WHERE id = ?`, attempt.BatchItemID)
	return err
}

func (r *Runner) updateAttempt(attempt *db.Attempt) error {
	query := `UPDATE attempts SET status = ?, stdout = ?, stderr = ?, exit_code = ?, tokens_used = ?, finished_at = ? WHERE id = ?`
	var finishedAt interface{}
	if attempt.FinishedAt != nil {
		finishedAt = attempt.FinishedAt.Format(time.RFC3339)
	}
	_, err := r.database.Conn().Exec(query, attempt.Status, attempt.Stdout, attempt.Stderr, attempt.ExitCode, attempt.TokensUsed, finishedAt, attempt.ID)
	return err
}

func (r *Runner) listAttempts(itemID string) ([]*db.Attempt, error) {
	query := `SELECT id, batch_item_id, attempt_no, run_no, attempt_type, status, stdout, stderr, exit_code, tokens_used, started_at, finished_at FROM attempts WHERE batch_item_id = ? ORDER BY attempt_no`
	rows, err := r.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []*db.Attempt
	for rows.Next() {
		attempt := &db.Attempt{}
		var stdout, stderr, startedAt, finishedAt sql.NullString
		var exitCode sql.NullInt64
		if err := rows.Scan(&attempt.ID, &attempt.BatchItemID, &attempt.AttemptNo, &attempt.RunNo, &attempt.AttemptType, &attempt.Status, &stdout, &stderr, &exitCode, &attempt.TokensUsed, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if attempt.RunNo <= 0 {
			attempt.RunNo = 1
		}
		if stdout.Valid {
			attempt.Stdout = stdout.String
		}
		if stderr.Valid {
			attempt.Stderr = stderr.String
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			attempt.ExitCode = &ec
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			attempt.StartedAt = t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			attempt.FinishedAt = &t
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (r *Runner) createQCRound(qc *db.QCRound) error {
	if qc.RunNo <= 0 {
		qc.RunNo = 1
	}
	query := `INSERT INTO qc_rounds (id, batch_item_id, qc_no, run_no, status, started_at) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := r.database.Conn().Exec(query, qc.ID, qc.BatchItemID, qc.QCNo, qc.RunNo, qc.Status, qc.StartedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := r.database.Conn().Exec(`UPDATE batch_items SET current_qc_no = current_qc_no + 1 WHERE id = ?`, qc.BatchItemID)
	return err
}

func (r *Runner) updateQCRound(qc *db.QCRound) error {
	query := `UPDATE qc_rounds SET status = ?, verdict = ?, feedback = ?, tokens_used = ?, finished_at = ? WHERE id = ?`
	var finishedAt interface{}
	if qc.FinishedAt != nil {
		finishedAt = qc.FinishedAt.Format(time.RFC3339)
	}
	_, err := r.database.Conn().Exec(query, qc.Status, qc.Verdict, qc.Feedback, qc.TokensUsed, finishedAt, qc.ID)
	return err
}

// listQCRounds 拉取单个 item 的所有质检轮次(按 qc_no 升序)。
// 被 emitItemUpdate 用作广播 payload 的一部分。
func (r *Runner) listQCRounds(itemID string) ([]*db.QCRound, error) {
	query := `SELECT id, batch_item_id, qc_no, run_no, status, verdict, feedback, tokens_used, started_at, finished_at FROM qc_rounds WHERE batch_item_id = ? ORDER BY qc_no`
	rows, err := r.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []*db.QCRound
	for rows.Next() {
		round := &db.QCRound{}
		var verdict, feedback, startedAt, finishedAt sql.NullString
		if err := rows.Scan(&round.ID, &round.BatchItemID, &round.QCNo, &round.RunNo, &round.Status, &verdict, &feedback, &round.TokensUsed, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if round.RunNo <= 0 {
			round.RunNo = 1
		}
		if verdict.Valid {
			round.Verdict = verdict.String
		}
		if feedback.Valid {
			round.Feedback = feedback.String
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			round.StartedAt = t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			round.FinishedAt = &t
		}
		rounds = append(rounds, round)
	}
	return rounds, rows.Err()
}
