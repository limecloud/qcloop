package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

// Runner 批次执行器
type Runner struct {
	database *db.DB
	executor executor.Executor // 基础 executor
	// activeExec 仅在一次 RunBatch 生命周期内使用:若 job.ExecutionMode 为
	// goal_assisted,则包上 GoalAssistedExecutor;否则 == executor。
	activeExec executor.Executor
}

// NewRunner 创建执行器(默认使用 CodexExecutor)
func NewRunner(database *db.DB) *Runner {
	return &Runner{
		database: database,
		executor: executor.NewCodexExecutor(),
	}
}

// NewRunnerWithExecutor 用指定 executor 创建 Runner(测试用)
func NewRunnerWithExecutor(database *db.DB, exec executor.Executor) *Runner {
	return &Runner{
		database: database,
		executor: exec,
	}
}

// RunBatch 运行批次
func (r *Runner) RunBatch(ctx context.Context, jobID string) error {
	job, err := r.database.GetJob(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("批次不存在")
	}

	// 根据 execution_mode 决定本次 Batch 用什么 executor。
	// goal_assisted:包一层 GoalAssistedExecutor,注入 Goal-style prompt;
	// 其他(含空值):直接用基础 executor。
	r.activeExec = r.buildActiveExecutor(job)
	defer func() { r.activeExec = nil }()

	if err := r.database.UpdateJobStatus(jobID, "running"); err != nil {
		return err
	}

	items, err := r.database.ListItems(jobID)
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

	return r.database.FinishJob(jobID, "completed")
}

// processItem 处理单个 item
func (r *Runner) processItem(ctx context.Context, job *db.BatchJob, item *db.BatchItem) error {
	// 执行 worker
	attemptNo := 1
	attempt, err := r.executeWorker(ctx, job, item, attemptNo, "worker")
	if err != nil {
		return err
	}

	// 如果没有配置 verifier，直接标记为成功
	if job.VerifierPromptTemplate == "" {
		status := "success"
		if attempt.Status != "success" {
			status = "failed"
		}
		return r.database.UpdateItemStatus(item.ID, status)
	}

	// 执行多轮质检
	return r.runQCLoop(ctx, job, item, attemptNo)
}

// runOne 统一封装 executor 调用,返回 Result(含 tokens)。
// 若 activeExec 实现了 TokenAwareExecutor 则走增强接口,否则走 legacy Execute 并 tokens=0。
func (r *Runner) runOne(ctx context.Context, prompt string) executor.Result {
	exec := r.activeExec
	if exec == nil {
		exec = r.executor // 兜底,不应发生
	}
	if tae, ok := exec.(executor.TokenAwareExecutor); ok {
		return tae.ExecuteWithTokens(ctx, prompt)
	}
	stdout, stderr, code, _ := exec.Execute(ctx, prompt)
	return executor.Result{Stdout: stdout, Stderr: stderr, ExitCode: code, TokensUsed: 0}
}

// buildActiveExecutor 根据 job.ExecutionMode 返回本次 Batch 要用的 executor
func (r *Runner) buildActiveExecutor(job *db.BatchJob) executor.Executor {
	switch job.ExecutionMode {
	case "goal_assisted":
		// 用 job 的 prompt 模板作为 goal hint(剥离 {{item}} 占位符让 Codex
		// 更好理解整体目标),stop hint 留空走默认
		goalHint := "按以下模板完成每个测试项:\n\n" + job.PromptTemplate
		if job.VerifierPromptTemplate != "" {
			goalHint += "\n\n每次完成后应能通过质检:\n" + job.VerifierPromptTemplate
		}
		return executor.NewGoalAssistedExecutor(r.executor, goalHint, "")
	default: // "" 或 "standard"
		return r.executor
	}
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
func (r *Runner) executeWorker(ctx context.Context, job *db.BatchJob, item *db.BatchItem, attemptNo int, attemptType string) (*db.Attempt, error) {
	prompt := strings.ReplaceAll(job.PromptTemplate, "{{item}}", item.ItemValue)

	attempt := &db.Attempt{
		ID:          db.GenerateID(),
		BatchItemID: item.ID,
		AttemptNo:   attemptNo,
		AttemptType: attemptType,
		Status:      "running",
		StartedAt:   time.Now(),
	}

	if err := r.createAttempt(attempt); err != nil {
		return nil, err
	}

	res := r.runOne(ctx, prompt)

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

	return attempt, nil
}

// runQCLoop 运行质检循环
func (r *Runner) runQCLoop(ctx context.Context, job *db.BatchJob, item *db.BatchItem, startAttemptNo int) error {
	attemptNo := startAttemptNo

	for qcNo := 1; qcNo <= job.MaxQCRounds; qcNo++ {
		// 每轮开始检查 token 预算
		if exceeded, err := r.checkBudgetExceeded(job, item); err != nil {
			return err
		} else if exceeded {
			return r.database.UpdateItemStatus(item.ID, "exhausted")
		}

		// 执行 verifier
		qcRound, err := r.executeVerifier(ctx, job, item, qcNo)
		if err != nil {
			return err
		}

		if qcRound.Status == "pass" {
			// 质检通过
			return r.database.UpdateItemStatus(item.ID, "success")
		}

		// 质检失败
		if qcNo >= job.MaxQCRounds {
			// 达到最大轮次
			return r.database.UpdateItemStatus(item.ID, "exhausted")
		}

		// 执行 repair 前再检查一次(verifier 本身可能已超预算)
		if exceeded, err := r.checkBudgetExceeded(job, item); err != nil {
			return err
		} else if exceeded {
			return r.database.UpdateItemStatus(item.ID, "exhausted")
		}

		// 执行 repair
		attemptNo++
		repairAttempt, err := r.executeRepair(ctx, job, item, attemptNo, qcRound.Feedback)
		if err != nil {
			return err
		}

		if repairAttempt.Status != "success" {
			// repair 失败，标记为 failed
			return r.database.UpdateItemStatus(item.ID, "failed")
		}
	}

	return nil
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

// executeVerifier 执行 verifier
func (r *Runner) executeVerifier(ctx context.Context, job *db.BatchJob, item *db.BatchItem, qcNo int) (*db.QCRound, error) {
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
	prompt := strings.ReplaceAll(job.VerifierPromptTemplate, "{{item}}", item.ItemValue)
	prompt = strings.ReplaceAll(prompt, "{{output}}", lastAttempt.Stdout)

	qcRound := &db.QCRound{
		ID:          db.GenerateID(),
		BatchItemID: item.ID,
		QCNo:        qcNo,
		Status:      "running",
		StartedAt:   time.Now(),
	}

	if err := r.createQCRound(qcRound); err != nil {
		return nil, err
	}

	res := r.runOne(ctx, prompt)
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
			if feedback, ok := verdict["feedback"].(string); ok {
				qcRound.Feedback = feedback
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

	return qcRound, nil
}

// executeRepair 执行 repair
func (r *Runner) executeRepair(ctx context.Context, job *db.BatchJob, item *db.BatchItem, attemptNo int, feedback string) (*db.Attempt, error) {
	prompt := strings.ReplaceAll(job.PromptTemplate, "{{item}}", item.ItemValue)
	prompt = fmt.Sprintf("%s\n\n质检反馈：%s\n请根据反馈修复问题。", prompt, feedback)

	return r.executeWorker(ctx, job, item, attemptNo, "repair")
}

// 数据库操作辅助方法
func (r *Runner) createAttempt(attempt *db.Attempt) error {
	query := `INSERT INTO attempts (id, batch_item_id, attempt_no, attempt_type, status, started_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := r.database.Conn().Exec(query, attempt.ID, attempt.BatchItemID, attempt.AttemptNo, attempt.AttemptType, attempt.Status, attempt.StartedAt.Format(time.RFC3339))
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
	query := `SELECT id, batch_item_id, attempt_no, attempt_type, status, stdout, stderr, exit_code, started_at, finished_at FROM attempts WHERE batch_item_id = ? ORDER BY attempt_no`
	rows, err := r.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []*db.Attempt
	for rows.Next() {
		attempt := &db.Attempt{}
		var exitCode interface{}
		var startedAt, finishedAt string
		if err := rows.Scan(&attempt.ID, &attempt.BatchItemID, &attempt.AttemptNo, &attempt.AttemptType, &attempt.Status, &attempt.Stdout, &attempt.Stderr, &exitCode, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if exitCode != nil {
			ec := int(exitCode.(int64))
			attempt.ExitCode = &ec
		}
		if startedAt != "" {
			t, _ := time.Parse(time.RFC3339, startedAt)
			attempt.StartedAt = t
		}
		if finishedAt != "" {
			t, _ := time.Parse(time.RFC3339, finishedAt)
			attempt.FinishedAt = &t
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (r *Runner) createQCRound(qc *db.QCRound) error {
	query := `INSERT INTO qc_rounds (id, batch_item_id, qc_no, status, started_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.database.Conn().Exec(query, qc.ID, qc.BatchItemID, qc.QCNo, qc.Status, qc.StartedAt.Format(time.RFC3339))
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
