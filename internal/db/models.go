package db

import "time"

// BatchJob 批次
type BatchJob struct {
	ID                     string     `json:"id"`
	Name                   string     `json:"name"`
	PromptTemplate         string     `json:"prompt_template"`
	VerifierPromptTemplate string     `json:"verifier_prompt_template"`
	MaxQCRounds            int        `json:"max_qc_rounds"`
	Status                 string     `json:"status"` // pending/running/completed/failed
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
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}
