package db

const Schema = `
-- 批次表
CREATE TABLE IF NOT EXISTS batch_jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    prompt_template TEXT NOT NULL,
    verifier_prompt_template TEXT,
    max_qc_rounds INTEGER NOT NULL DEFAULT 3,
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
    created_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (batch_job_id) REFERENCES batch_jobs(id) ON DELETE CASCADE
);

-- 执行尝试表
CREATE TABLE IF NOT EXISTS attempts (
    id TEXT PRIMARY KEY,
    batch_item_id TEXT NOT NULL,
    attempt_no INTEGER NOT NULL,
    attempt_type TEXT NOT NULL,
    status TEXT NOT NULL,
    stdout TEXT,
    stderr TEXT,
    exit_code INTEGER,
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
    status TEXT NOT NULL,
    verdict TEXT,
    feedback TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (batch_item_id) REFERENCES batch_items(id) ON DELETE CASCADE,
    UNIQUE(batch_item_id, qc_no)
);

CREATE INDEX IF NOT EXISTS idx_batch_items_job ON batch_items(batch_job_id);
CREATE INDEX IF NOT EXISTS idx_batch_items_status ON batch_items(batch_job_id, status);
CREATE INDEX IF NOT EXISTS idx_attempts_item ON attempts(batch_item_id);
CREATE INDEX IF NOT EXISTS idx_qc_rounds_item ON qc_rounds(batch_item_id);
`
