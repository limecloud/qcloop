package db

const Schema = `
-- 批次表
CREATE TABLE IF NOT EXISTS batch_jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    prompt_template TEXT NOT NULL,
    verifier_prompt_template TEXT,
    max_qc_rounds INTEGER NOT NULL DEFAULT 3,
    token_budget_per_item INTEGER NOT NULL DEFAULT 0,
    max_executor_retries INTEGER NOT NULL DEFAULT 1,
    execution_mode TEXT NOT NULL DEFAULT 'standard',
    executor_provider TEXT NOT NULL DEFAULT 'codex',
    run_no INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    finished_at TEXT
);

-- 批次项表
CREATE TABLE IF NOT EXISTS batch_items (
    id TEXT PRIMARY KEY,
    batch_job_id TEXT NOT NULL,
    item_value TEXT NOT NULL,
    status TEXT NOT NULL,
    current_attempt_no INTEGER NOT NULL DEFAULT 0,
    current_qc_no INTEGER NOT NULL DEFAULT 0,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    lock_owner TEXT,
    lock_expires_at TEXT,
    queued_at TEXT,
    last_error TEXT,
    confirmation_question TEXT,
    confirmation_answer TEXT,
    created_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (batch_job_id) REFERENCES batch_jobs(id) ON DELETE CASCADE
);

-- 执行尝试表
CREATE TABLE IF NOT EXISTS attempts (
    id TEXT PRIMARY KEY,
    batch_item_id TEXT NOT NULL,
    attempt_no INTEGER NOT NULL,
    run_no INTEGER NOT NULL DEFAULT 1,
    attempt_type TEXT NOT NULL,
    status TEXT NOT NULL,
    stdout TEXT,
    stderr TEXT,
    exit_code INTEGER,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (batch_item_id) REFERENCES batch_items(id) ON DELETE CASCADE,
    UNIQUE(batch_item_id, attempt_no)
);

-- 质检轮次表
CREATE TABLE IF NOT EXISTS qc_rounds (
    id TEXT PRIMARY KEY,
    batch_item_id TEXT NOT NULL,
    qc_no INTEGER NOT NULL,
    run_no INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL,
    verdict TEXT,
    feedback TEXT,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (batch_item_id) REFERENCES batch_items(id) ON DELETE CASCADE,
    UNIQUE(batch_item_id, qc_no)
);

CREATE INDEX IF NOT EXISTS idx_batch_items_job ON batch_items(batch_job_id);
CREATE INDEX IF NOT EXISTS idx_batch_items_status ON batch_items(batch_job_id, status);
CREATE INDEX IF NOT EXISTS idx_attempts_item ON attempts(batch_item_id);
CREATE INDEX IF NOT EXISTS idx_qc_rounds_item ON qc_rounds(batch_item_id);

-- 批次模板表
CREATE TABLE IF NOT EXISTS batch_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    prompt_template TEXT NOT NULL,
    verifier_prompt_template TEXT,
    max_qc_rounds INTEGER NOT NULL DEFAULT 3,
    token_budget_per_item INTEGER NOT NULL DEFAULT 0,
    max_executor_retries INTEGER NOT NULL DEFAULT 1,
    execution_mode TEXT NOT NULL DEFAULT 'standard',
    executor_provider TEXT NOT NULL DEFAULT 'codex',
    items_text TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_batch_templates_name ON batch_templates(name);

-- 幂等 migration:若已有旧表缺列,补加
`

// MigrationStatements 是启动时运行的幂等 ALTER,用于补齐旧表缺列。
// 每条都允许失败(列已存在时 sqlite 会报错),调用方忽略错误即可。
var MigrationStatements = []string{
	`ALTER TABLE batch_jobs ADD COLUMN token_budget_per_item INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE batch_jobs ADD COLUMN max_executor_retries INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE batch_jobs ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'standard'`,
	`ALTER TABLE batch_jobs ADD COLUMN executor_provider TEXT NOT NULL DEFAULT 'codex'`,
	`ALTER TABLE batch_jobs ADD COLUMN run_no INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE batch_items ADD COLUMN tokens_used INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE batch_items ADD COLUMN lock_owner TEXT`,
	`ALTER TABLE batch_items ADD COLUMN lock_expires_at TEXT`,
	`ALTER TABLE batch_items ADD COLUMN queued_at TEXT`,
	`ALTER TABLE batch_items ADD COLUMN last_error TEXT`,
	`ALTER TABLE batch_items ADD COLUMN confirmation_question TEXT`,
	`ALTER TABLE batch_items ADD COLUMN confirmation_answer TEXT`,
	`ALTER TABLE attempts ADD COLUMN tokens_used INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE attempts ADD COLUMN run_no INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE qc_rounds ADD COLUMN tokens_used INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE qc_rounds ADD COLUMN run_no INTEGER NOT NULL DEFAULT 1`,
	`CREATE INDEX IF NOT EXISTS idx_batch_items_queue ON batch_items(status, queued_at, lock_expires_at)`,
}
