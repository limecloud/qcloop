package db

import "time"

// BatchJobStatus 批次状态
type BatchJobStatus string

const (
	JobStatusPending   BatchJobStatus = "pending"
	JobStatusRunning   BatchJobStatus = "running"
	JobStatusCompleted BatchJobStatus = "completed"
	JobStatusFailed    BatchJobStatus = "failed"
	JobStatusCanceled  BatchJobStatus = "canceled"
	JobStatusPaused    BatchJobStatus = "paused"
)

// BatchJob 批次
type BatchJob struct {
	ID                     string         `db:"id"`
	Name                   string         `db:"name"`
	Description            string         `db:"description"`
	Status                 BatchJobStatus `db:"status"`
	WorkerPromptTemplate   string         `db:"worker_prompt_template"`
	VerifierPromptTemplate string         `db:"verifier_prompt_template"`
	RepairPromptTemplate   string         `db:"repair_prompt_template"`
	MaxQCRounds            int            `db:"max_qc_rounds"`
	Concurrency            int            `db:"concurrency"`
	CreatedAt              time.Time      `db:"created_at"`
	StartedAt              *time.Time     `db:"started_at"`
	FinishedAt             *time.Time     `db:"finished_at"`
	TotalItems             int            `db:"total_items"`
	PassedItems            int            `db:"passed_items"`
	ExhaustedItems         int            `db:"exhausted_items"`
	CanceledItems          int            `db:"canceled_items"`
}

// BatchItemStatus 批次项状态
type BatchItemStatus string

const (
	ItemStatusPending      BatchItemStatus = "pending"
	ItemStatusClaimed      BatchItemStatus = "claimed"
	ItemStatusRunning      BatchItemStatus = "running"
	ItemStatusQCRunning    BatchItemStatus = "qc_running"
	ItemStatusQCFailed     BatchItemStatus = "qc_failed"
	ItemStatusRetryPending BatchItemStatus = "retry_pending"
	ItemStatusPassed       BatchItemStatus = "passed"
	ItemStatusExhausted    BatchItemStatus = "exhausted"
	ItemStatusCanceled     BatchItemStatus = "canceled"
)

// BatchItem 批次项
type BatchItem struct {
	ID               string          `db:"id"`
	BatchJobID       string          `db:"batch_job_id"`
	ItemKey          string          `db:"item_key"`
	Params           string          `db:"params"` // JSON
	Status           BatchItemStatus `db:"status"`
	LeaseOwner       *string         `db:"lease_owner"`
	LeaseExpiresAt   *time.Time      `db:"lease_expires_at"`
	CurrentAttemptNo int             `db:"current_attempt_no"`
	CurrentQCNo      int             `db:"current_qc_no"`
	CreatedAt        time.Time       `db:"created_at"`
	ClaimedAt        *time.Time      `db:"claimed_at"`
	FinishedAt       *time.Time      `db:"finished_at"`
}

// AttemptStatus 执行尝试状态
type AttemptStatus string

const (
	AttemptStatusRunning  AttemptStatus = "running"
	AttemptStatusSuccess  AttemptStatus = "success"
	AttemptStatusError    AttemptStatus = "error"
	AttemptStatusTimeout  AttemptStatus = "timeout"
	AttemptStatusCanceled AttemptStatus = "canceled"
)

// Attempt 执行尝试
type Attempt struct {
	ID            string        `db:"id"`
	BatchItemID   string        `db:"batch_item_id"`
	AttemptNo     int           `db:"attempt_no"`
	Status        AttemptStatus `db:"status"`
	ExecutorType  string        `db:"executor_type"`
	ThreadID      *string       `db:"thread_id"`
	SessionID     *string       `db:"session_id"`
	StartedAt     time.Time     `db:"started_at"`
	FinishedAt    *time.Time    `db:"finished_at"`
	DurationMs    *int64        `db:"duration_ms"`
	ExitCode      *int          `db:"exit_code"`
	Stdout        string        `db:"stdout"`
	Stderr        string        `db:"stderr"`
	ErrorMessage  string        `db:"error_message"`
}

// QCStatus 质检状态
type QCStatus string

const (
	QCStatusRunning QCStatus = "running"
	QCStatusPass    QCStatus = "pass"
	QCStatusFail    QCStatus = "fail"
	QCStatusError   QCStatus = "error"
)

// QCRound 质检轮次
type QCRound struct {
	ID               string     `db:"id"`
	BatchItemID      string     `db:"batch_item_id"`
	QCNo             int        `db:"qc_no"`
	Status           QCStatus   `db:"status"`
	VerifierThreadID *string    `db:"verifier_thread_id"`
	StartedAt        time.Time  `db:"started_at"`
	FinishedAt       *time.Time `db:"finished_at"`
	DurationMs       *int64     `db:"duration_ms"`
	Verdict          string     `db:"verdict"` // JSON
	Feedback         string     `db:"feedback"`
}

// ArtifactType 产物类型
type ArtifactType string

const (
	ArtifactTypePromptSnapshot ArtifactType = "prompt_snapshot"
	ArtifactTypeDiff           ArtifactType = "diff"
	ArtifactTypeLog            ArtifactType = "log"
	ArtifactTypeVerdict        ArtifactType = "verdict"
)

// Artifact 产物
type Artifact struct {
	ID           string       `db:"id"`
	BatchItemID  string       `db:"batch_item_id"`
	ArtifactType ArtifactType `db:"artifact_type"`
	Content      string       `db:"content"`
	CreatedAt    time.Time    `db:"created_at"`
}
