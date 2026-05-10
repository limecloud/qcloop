package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const jobColumns = `id, name, prompt_template, verifier_prompt_template, max_qc_rounds, token_budget_per_item, execution_mode, executor_provider, run_no, status, created_at, finished_at`
const itemColumns = `id, batch_job_id, item_value, status, current_attempt_no, current_qc_no, tokens_used, lock_owner, lock_expires_at, queued_at, last_error, confirmation_question, confirmation_answer, created_at, finished_at`

// CreateJob 创建批次
func (db *DB) CreateJob(job *BatchJob) error {
	if job.ExecutionMode == "" {
		job.ExecutionMode = "standard"
	}
	if job.ExecutorProvider == "" {
		job.ExecutorProvider = "codex"
	}
	if job.RunNo <= 0 {
		job.RunNo = 1
	}
	query := `INSERT INTO batch_jobs (id, name, prompt_template, verifier_prompt_template, max_qc_rounds, token_budget_per_item, execution_mode, executor_provider, run_no, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.conn.Exec(query, job.ID, job.Name, job.PromptTemplate, job.VerifierPromptTemplate, job.MaxQCRounds, job.TokenBudgetPerItem, job.ExecutionMode, job.ExecutorProvider, job.RunNo, job.Status, job.CreatedAt.Format(time.RFC3339))
	return err
}

// GetJob 获取批次
func (db *DB) GetJob(id string) (*BatchJob, error) {
	if _, _, err := db.ReconcileJobStatusIfDone(id); err != nil {
		return nil, err
	}
	query := `SELECT ` + jobColumns + ` FROM batch_jobs WHERE id = ?`
	job := &BatchJob{}
	var createdAt, finishedAt sql.NullString
	err := db.conn.QueryRow(query, id).Scan(&job.ID, &job.Name, &job.PromptTemplate, &job.VerifierPromptTemplate, &job.MaxQCRounds, &job.TokenBudgetPerItem, &job.ExecutionMode, &job.ExecutorProvider, &job.RunNo, &job.Status, &createdAt, &finishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	applyJobTimes(job, createdAt, finishedAt)
	return job, nil
}

func applyJobTimes(job *BatchJob, createdAt, finishedAt sql.NullString) {
	if job.RunNo <= 0 {
		job.RunNo = 1
	}
	if job.ExecutionMode == "" {
		job.ExecutionMode = "standard"
	}
	if job.ExecutorProvider == "" {
		job.ExecutorProvider = "codex"
	}
	if createdAt.Valid {
		if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
			job.CreatedAt = t
		}
	}
	if finishedAt.Valid {
		if t, err := time.Parse(time.RFC3339, finishedAt.String); err == nil {
			job.FinishedAt = &t
		}
	}
}

// UpdateJobStatus 更新批次状态
func (db *DB) UpdateJobStatus(id, status string) error {
	query := `UPDATE batch_jobs SET status = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, id)
	return err
}

// UpdateJob 更新批次配置,不触碰运行历史和状态。
func (db *DB) UpdateJob(job *BatchJob) error {
	if job.ExecutionMode == "" {
		job.ExecutionMode = "standard"
	}
	if job.ExecutorProvider == "" {
		job.ExecutorProvider = "codex"
	}
	query := `
		UPDATE batch_jobs
		SET name = ?, prompt_template = ?, verifier_prompt_template = ?,
			max_qc_rounds = ?, token_budget_per_item = ?, execution_mode = ?, executor_provider = ?
		WHERE id = ?
	`
	_, err := db.conn.Exec(query, job.Name, job.PromptTemplate, job.VerifierPromptTemplate, job.MaxQCRounds, job.TokenBudgetPerItem, job.ExecutionMode, job.ExecutorProvider, job.ID)
	return err
}

// DeleteJob 删除批次及其 items/attempts/qc_rounds。外键级联负责清理明细。
func (db *DB) DeleteJob(id string) error {
	_, err := db.conn.Exec(`DELETE FROM batch_jobs WHERE id = ?`, id)
	return err
}

// StartJob 标记批次进入运行中,并清空上一次完成时间。
func (db *DB) StartJob(id string) error {
	query := `UPDATE batch_jobs SET status = 'running', finished_at = NULL WHERE id = ?`
	_, err := db.conn.Exec(query, id)
	return err
}

// IncrementJobRunNo 开启新运行轮次。
func (db *DB) IncrementJobRunNo(id string) error {
	_, err := db.conn.Exec(`UPDATE batch_jobs SET run_no = CASE WHEN run_no < 1 THEN 1 ELSE run_no + 1 END WHERE id = ?`, id)
	return err
}

// FinishJob 完成批次
func (db *DB) FinishJob(id, status string) error {
	now := time.Now()
	query := `UPDATE batch_jobs SET status = ?, finished_at = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, now.Format(time.RFC3339), id)
	return err
}

// FinishJobIfDone 在 running job 没有 pending/running item 时标记终态。
//
// 语义:
//   - 全部 item 成功 => job.completed
//   - 任一 item failed/exhausted => job.failed
//
// 这样 Web 面板的批次状态不会把“有失败项的跑完”误展示成“已完成/通过”。
func (db *DB) FinishJobIfDone(jobID string) (bool, string, error) {
	done, finalStatus, err := db.finalJobStatusIfDone(jobID)
	if err != nil {
		return false, "", err
	}
	if !done {
		return false, "", nil
	}
	now := time.Now().Format(time.RFC3339)
	res, err := db.conn.Exec(
		`UPDATE batch_jobs SET status = ?, finished_at = CASE WHEN ? = 'waiting_confirmation' THEN NULL ELSE ? END WHERE id = ? AND status = 'running'`,
		finalStatus,
		finalStatus,
		now,
		jobID,
	)
	if err != nil {
		return false, "", err
	}
	changed, _ := res.RowsAffected()
	return changed > 0, finalStatus, nil
}

// ReconcileJobStatusIfDone 修正历史或异步路径留下的终态漂移。
//
// 旧版本可能把包含 failed/exhausted item 的批次标成 completed;读取时
// 主动收敛一次,避免 Web 面板把"跑完但未通过"误展示为"已完成"。
func (db *DB) ReconcileJobStatusIfDone(jobID string) (bool, string, error) {
	var currentStatus string
	if err := db.conn.QueryRow(`SELECT status FROM batch_jobs WHERE id = ?`, jobID).Scan(&currentStatus); err != nil {
		if err == sql.ErrNoRows {
			return false, "", nil
		}
		return false, "", err
	}
	if currentStatus != "running" && currentStatus != "waiting_confirmation" && currentStatus != "completed" && currentStatus != "failed" {
		return false, currentStatus, nil
	}
	done, finalStatus, err := db.finalJobStatusIfDone(jobID)
	if err != nil {
		return false, "", err
	}
	if !done {
		return false, currentStatus, nil
	}
	now := time.Now().Format(time.RFC3339)
	res, err := db.conn.Exec(
		`UPDATE batch_jobs SET status = ?, finished_at = CASE WHEN ? = 'waiting_confirmation' THEN NULL ELSE COALESCE(finished_at, ?) END WHERE id = ? AND status IN ('running', 'waiting_confirmation', 'completed', 'failed') AND (status <> ? OR finished_at IS NULL)`,
		finalStatus,
		finalStatus,
		now,
		jobID,
		finalStatus,
	)
	if err != nil {
		return false, "", err
	}
	changed, _ := res.RowsAffected()
	return changed > 0, finalStatus, nil
}

// ReconcileAllDoneJobStatuses 批量修正列表页会读到的历史终态漂移。
func (db *DB) ReconcileAllDoneJobStatuses() error {
	rows, err := db.conn.Query(`SELECT id FROM batch_jobs WHERE status IN ('running', 'waiting_confirmation', 'completed', 'failed')`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, _, err := db.ReconcileJobStatusIfDone(id); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) finalJobStatusIfDone(jobID string) (bool, string, error) {
	var remaining int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM batch_items WHERE batch_job_id = ? AND status IN ('pending', 'running')`, jobID).Scan(&remaining); err != nil {
		return false, "", err
	}
	if remaining > 0 {
		return false, "", nil
	}
	var awaiting int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM batch_items WHERE batch_job_id = ? AND status = 'awaiting_confirmation'`, jobID).Scan(&awaiting); err != nil {
		return false, "", err
	}
	if awaiting > 0 {
		return true, "waiting_confirmation", nil
	}
	finalStatus := "completed"
	var blocked int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM batch_items WHERE batch_job_id = ? AND status IN ('failed', 'exhausted')`, jobID).Scan(&blocked); err != nil {
		return false, "", err
	}
	if blocked > 0 {
		finalStatus = "failed"
	}
	return true, finalStatus, nil
}

// CompleteJobIfDone 保留旧调用签名;新代码优先使用 FinishJobIfDone。
func (db *DB) CompleteJobIfDone(jobID string) (bool, error) {
	changed, _, err := db.FinishJobIfDone(jobID)
	return changed, err
}

// CreateItem 创建批次项
func (db *DB) CreateItem(item *BatchItem) error {
	query := `INSERT INTO batch_items (id, batch_job_id, item_value, status, queued_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	var queuedAt interface{}
	if item.QueuedAt != nil {
		queuedAt = item.QueuedAt.Format(time.RFC3339)
	}
	_, err := db.conn.Exec(query, item.ID, item.BatchJobID, item.ItemValue, item.Status, queuedAt, item.CreatedAt.Format(time.RFC3339))
	return err
}

// GetItem 获取批次项
func (db *DB) GetItem(id string) (*BatchItem, error) {
	query := `SELECT ` + itemColumns + ` FROM batch_items WHERE id = ?`
	item := &BatchItem{}
	err := scanItem(db.conn.QueryRow(query, id), item)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

// ListItems 获取批次的所有项
func (db *DB) ListItems(jobID string) ([]*BatchItem, error) {
	query := `SELECT ` + itemColumns + ` FROM batch_items WHERE batch_job_id = ? ORDER BY created_at`
	rows, err := db.conn.Query(query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BatchItem
	for rows.Next() {
		item := &BatchItem{}
		if err := scanItem(rows, item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanItem(row rowScanner, item *BatchItem) error {
	var lockOwner, lockExpiresAt, queuedAt, lastError, confirmationQuestion, confirmationAnswer, createdAt, finishedAt sql.NullString
	if err := row.Scan(&item.ID, &item.BatchJobID, &item.ItemValue, &item.Status, &item.CurrentAttemptNo, &item.CurrentQCNo, &item.TokensUsed, &lockOwner, &lockExpiresAt, &queuedAt, &lastError, &confirmationQuestion, &confirmationAnswer, &createdAt, &finishedAt); err != nil {
		return err
	}
	if lockOwner.Valid {
		item.LockOwner = lockOwner.String
	}
	if t := parseNullableTime(lockExpiresAt); t != nil {
		item.LockExpiresAt = t
	}
	if t := parseNullableTime(queuedAt); t != nil {
		item.QueuedAt = t
	}
	if lastError.Valid {
		item.LastError = lastError.String
	}
	if confirmationQuestion.Valid {
		item.ConfirmationQuestion = confirmationQuestion.String
	}
	if confirmationAnswer.Valid {
		item.ConfirmationAnswer = confirmationAnswer.String
	}
	if t := parseNullableTime(createdAt); t != nil {
		item.CreatedAt = *t
	}
	if t := parseNullableTime(finishedAt); t != nil {
		item.FinishedAt = t
	}
	return nil
}

func parseNullableTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value.String)
	if err != nil {
		return nil
	}
	return &t
}

// UpdateItemStatus 更新批次项状态
func (db *DB) UpdateItemStatus(id, status string) error {
	if status == "running" {
		_, err := db.conn.Exec(`UPDATE batch_items SET status = ? WHERE id = ?`, status, id)
		return err
	}
	query := `UPDATE batch_items SET status = ?, lock_owner = NULL, lock_expires_at = NULL WHERE id = ?`
	_, err := db.conn.Exec(query, status, id)
	return err
}

// ResetItemsForRerun 把批次内所有 item 放回 pending,保留历史 attempts/qc_rounds。
func (db *DB) ResetItemsForRerun(jobID string) error {
	return db.ResetItemsForRerunMode(jobID, "all")
}

// ResetItemsForRerunMode 根据模式重置 item 当前轮状态。
func (db *DB) ResetItemsForRerunMode(jobID, scope string) error {
	now := time.Now().Format(time.RFC3339)
	if scope == "unfinished" {
		if _, err := db.conn.Exec(`UPDATE batch_items SET current_attempt_no = 0, current_qc_no = 0, tokens_used = 0, lock_owner = NULL, lock_expires_at = NULL, queued_at = NULL, last_error = NULL, confirmation_question = NULL, confirmation_answer = NULL WHERE batch_job_id = ? AND status = 'success'`, jobID); err != nil {
			return err
		}
		_, err := db.conn.Exec(`UPDATE batch_items SET status = 'pending', current_attempt_no = 0, current_qc_no = 0, tokens_used = 0, finished_at = NULL, lock_owner = NULL, lock_expires_at = NULL, queued_at = ?, last_error = NULL, confirmation_question = NULL, confirmation_answer = NULL WHERE batch_job_id = ? AND status <> 'success'`, now, jobID)
		return err
	}
	_, err := db.conn.Exec(`UPDATE batch_items SET status = 'pending', current_attempt_no = 0, current_qc_no = 0, tokens_used = 0, finished_at = NULL, lock_owner = NULL, lock_expires_at = NULL, queued_at = ?, last_error = NULL, confirmation_question = NULL, confirmation_answer = NULL WHERE batch_job_id = ?`, now, jobID)
	return err
}

// QueuePendingItems 标记当前 pending 项进入队列。
func (db *DB) QueuePendingItems(jobID string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.conn.Exec(`UPDATE batch_items SET queued_at = ?, last_error = NULL WHERE batch_job_id = ? AND status = 'pending'`, now, jobID)
	return err
}

// FinishItem 完成批次项
func (db *DB) FinishItem(id, status string) error {
	now := time.Now()
	query := `UPDATE batch_items SET status = ?, finished_at = ?, lock_owner = NULL, lock_expires_at = NULL, last_error = NULL WHERE id = ?`
	_, err := db.conn.Exec(query, status, now.Format(time.RFC3339), id)
	return err
}

// MarkItemAwaitingConfirmation 暂停单个 item,等待外层 AI 获取人类确认后继续。
func (db *DB) MarkItemAwaitingConfirmation(id, question string) error {
	question = fmt.Sprintf("%s", question)
	_, err := db.conn.Exec(
		`UPDATE batch_items SET status = 'awaiting_confirmation', finished_at = NULL, lock_owner = NULL, lock_expires_at = NULL, last_error = ?, confirmation_question = ?, confirmation_answer = NULL WHERE id = ?`,
		question,
		question,
		id,
	)
	return err
}

// AnswerItemConfirmation 保存外层 AI 写回的人类确认;resume=true 时重新入队。
func (db *DB) AnswerItemConfirmation(id, answer string, resume bool) error {
	if resume {
		_, err := db.conn.Exec(
			`UPDATE batch_items SET status = 'pending', finished_at = NULL, lock_owner = NULL, lock_expires_at = NULL, queued_at = ?, last_error = NULL, confirmation_answer = ? WHERE id = ?`,
			time.Now().Format(time.RFC3339),
			answer,
			id,
		)
		return err
	}
	_, err := db.conn.Exec(`UPDATE batch_items SET confirmation_answer = ? WHERE id = ?`, answer, id)
	return err
}

// RequeueItem 把运行中的 item 放回 pending,通常用于 pause/cancel/stale recovery。
func (db *DB) RequeueItem(id, reason string) error {
	_, err := db.conn.Exec(`UPDATE batch_items SET status = 'pending', finished_at = NULL, lock_owner = NULL, lock_expires_at = NULL, queued_at = ?, last_error = ? WHERE id = ?`, time.Now().Format(time.RFC3339), reason, id)
	return err
}

// RenewItemLease 续租正在运行的 item。
func (db *DB) RenewItemLease(itemID, owner string, expiresAt time.Time) error {
	_, err := db.conn.Exec(`UPDATE batch_items SET lock_expires_at = ? WHERE id = ? AND lock_owner = ? AND status = 'running'`, expiresAt.Format(time.RFC3339), itemID, owner)
	return err
}

// ClaimNextItem 原子领取一个可运行 item。没有任务时返回 nil,nil,nil。
func (db *DB) ClaimNextItem(workerID string, leaseUntil time.Time) (*BatchJob, *BatchItem, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var itemID string
	query := `
		SELECT i.id
		FROM batch_items i
		JOIN batch_jobs j ON j.id = i.batch_job_id
		WHERE j.status = 'running' AND i.status = 'pending'
		ORDER BY COALESCE(i.queued_at, i.created_at), j.created_at, i.created_at
		LIMIT 1
	`
	err = tx.QueryRow(query).Scan(&itemID)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	res, err := tx.Exec(`UPDATE batch_items SET status = 'running', lock_owner = ?, lock_expires_at = ?, last_error = NULL WHERE id = ? AND status = 'pending'`, workerID, leaseUntil.Format(time.RFC3339), itemID)
	if err != nil {
		return nil, nil, err
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		return nil, nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	item, err := db.GetItem(itemID)
	if err != nil || item == nil {
		return nil, nil, err
	}
	job, err := db.GetJob(item.BatchJobID)
	if err != nil {
		return nil, nil, err
	}
	return job, item, nil
}

// RecoverStaleRunningItems 回收租约过期的 running item。
func (db *DB) RecoverStaleRunningItems(now time.Time) ([]string, error) {
	rows, err := db.conn.Query(`
		SELECT i.id
		FROM batch_items i
		JOIN batch_jobs j ON j.id = i.batch_job_id
		WHERE j.status = 'running'
		  AND i.status = 'running'
		  AND i.lock_expires_at IS NOT NULL
		  AND i.lock_expires_at < ?
	`, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	message := fmt.Sprintf("stale running item recovered at %s", now.Format(time.RFC3339))
	for _, id := range ids {
		_, _ = db.conn.Exec(`UPDATE attempts SET status = 'failed', stderr = COALESCE(stderr, '') || ?, exit_code = COALESCE(exit_code, -1), finished_at = ? WHERE batch_item_id = ? AND status = 'running'`, "\n[queue] "+message, now.Format(time.RFC3339), id)
		_, _ = db.conn.Exec(`UPDATE qc_rounds SET status = 'fail', feedback = COALESCE(feedback, '') || ?, finished_at = ? WHERE batch_item_id = ? AND status = 'running'`, "\n[queue] "+message, now.Format(time.RFC3339), id)
		if err := db.RequeueItem(id, message); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

// GetJobStats 获取批次统计
func (db *DB) GetJobStats(jobID string) (total, success, failed, pending int, err error) {
	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending
		FROM batch_items WHERE batch_job_id = ?
	`
	err = db.conn.QueryRow(query, jobID).Scan(&total, &success, &failed, &pending)
	return
}

// GenerateID 生成 UUID
func GenerateID() string {
	return uuid.New().String()
}
