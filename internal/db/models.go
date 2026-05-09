package db

import "time"

// BatchJob 批次
//
// ExecutionMode 支持两种模式:
//   - "standard":直接调 codex exec 执行原 prompt(默认)
//   - "goal_assisted":把 prompt 包装成 goal-style(GOAL/TASK/STOP WHEN 段落)
//     让 Codex 在单次 exec 内尽可能收敛,外层仍由 max_qc_rounds 硬停止
type BatchJob struct {
	ID                     string     `json:"id"`
	Name                   string     `json:"name"`
	PromptTemplate         string     `json:"prompt_template"`
	VerifierPromptTemplate string     `json:"verifier_prompt_template"`
	MaxQCRounds            int        `json:"max_qc_rounds"`
	TokenBudgetPerItem     int        `json:"token_budget_per_item"` // 每个 item 的 token 预算
	ExecutionMode          string     `json:"execution_mode"`        // standard | goal_assisted
	Status                 string     `json:"status"`                // pending/running/completed/failed/paused
	CreatedAt              time.Time  `json:"created_at"`
	FinishedAt             *time.Time `json:"finished_at"`
}

// BatchItem 批次项
type BatchItem struct {
	ID               string     `json:"id"`
	BatchJobID       string     `json:"batch_job_id"`
	ItemValue        string     `json:"item_value"`
	Status           string     `json:"status"` // pending/running/success/failed/exhausted
	CurrentAttemptNo int        `json:"current_attempt_no"`
	CurrentQCNo      int        `json:"current_qc_no"`
	TokensUsed       int        `json:"tokens_used"`       // 已使用的 token 数量
	TimeUsedSeconds  int        `json:"time_used_seconds"` // 已使用的时间（秒）
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at"`
}

// Attempt 执行尝试
type Attempt struct {
	ID          string     `json:"id"`
	BatchItemID string     `json:"batch_item_id"`
	AttemptNo   int        `json:"attempt_no"`
	AttemptType string     `json:"attempt_type"` // worker/repair
	Status      string     `json:"status"`       // running/success/failed
	Stdout      string     `json:"stdout"`
	Stderr      string     `json:"stderr"`
	ExitCode    *int       `json:"exit_code"`
	TokensUsed  int        `json:"tokens_used"`  // 本次尝试使用的 token 数量
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

// QCRound 质检轮次
type QCRound struct {
	ID          string     `json:"id"`
	BatchItemID string     `json:"batch_item_id"`
	QCNo        int        `json:"qc_no"`
	Status      string     `json:"status"` // running/pass/fail
	Verdict     string     `json:"verdict"`
	Feedback    string     `json:"feedback"`
	TokensUsed  int        `json:"tokens_used"` // 本次质检使用的 token 数量
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}
