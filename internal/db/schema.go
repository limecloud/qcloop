package db

const Schema = `
-- 批次表
CREATE TABLE IF NOT EXISTS batch_jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    worker_prompt_template TEXT NOT NULL,
    verifier_prompt_template TEXT,
    repair_prompt_template TEXT,
    max_qc_rounds INTEGER NOT NULL DEFAULT 3,
    concurrency INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    total_items INTEGER NOT NULL DEFAULT 0,
    passed_items INTEGER NOT NULL DEFAULT 0,
    exhausted_items INTEGER NOT NULL DEFAULT 0,
    canceled_items INTEGER NOT NULL DEFAULT 0
);

-- 批次项表
CREATE TABLE IF NOT EXISTS batch_items (
    id TEXT PRIMARY KEY,
    batch_job_id TEXT NOT NULL,
    item_key TEXT NOT NULL,
    params TEXT NOT NULL,
    status TEXT NOT NULL,
    lease_owner TEXT,
    lease_expires_at TEXT,
    current_attempt_no INTEGER NOT NULL DEFAULT 0,
    current_qc_no INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    claimed_at TEXT,
    finished_at TEXT,
    FOREIGN KEY (batch_job_id) REFERENCES batch_jobs(id) ON DELETE CASCADE,
    UNIQUE(batch_job_id, item_key)
);

-- 执行尝试表
CREATE TABLE IF NOT EXISTS attempts (
    id TEXT PRIMARY KEY,
    batch_item_id TEXT NOT NULL,
    attempt_no INTEGER NOT NULL,
    status TEXT NOT NULL,
    executor_type TEXT NOT NULL,
    thread_id TEXT,
    session_id TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    duration_ms INTEGER,
    exit_code INTEGER,
    stdout TEXT,
    stderr TEXT,
    error_message TEXT,
    FOREIGN KEY (batch_item_id) REFERENCES batch_items(id) ON DELETE CASCADE,
    UNIQUE(batch_item_id, attempt_no)
);

-- 质检轮次表
CREATE TABLE IF NOT EXISTS qc_rounds (
    id TEXT PRIMARY KEY,
    batch_item_id TEXT NOT NULL,
    qc_no INTEGER NOT NULL,
    status TEXT NOT NULL,
    verifier_thread_id TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    duration_ms INTEGER,
    verdict TEXT,
    feedback TEXT,
    FOREIGN KEY (batch_item_id) REFERENCES batch_items(id) ON DELETE CASCADE,
    UNIQUE(batch_item_id, qc_no)
);

-- 产物表
CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    batch_item_id TEXT NOT NULL,
    artifact_type TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (batch_item_id) REFERENCES batch_items(id) ON DELETE CASCADE
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_batch_items_job_status ON batch_items(batch_job_id, status);
CREATE INDEX IF NOT EXISTS idx_batch_items_lease ON batch_items(lease_expires_at) WHERE status IN ('claimed', 'running', 'qc_running');
CREATE INDEX IF NOT EXISTS idx_attempts_item ON attempts(batch_item_id);
CREATE INDEX IF NOT EXISTS idx_qc_rounds_item ON qc_rounds(batch_item_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_item ON artifacts(batch_item_id);
`
