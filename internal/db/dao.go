package db

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// CreateJob 创建批次
func (db *DB) CreateJob(job *BatchJob) error {
	query := `INSERT INTO batch_jobs (id, name, prompt_template, status, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := db.conn.Exec(query, job.ID, job.Name, job.PromptTemplate, job.Status, job.CreatedAt.Format(time.RFC3339))
	return err
}

// GetJob 获取批次
func (db *DB) GetJob(id string) (*BatchJob, error) {
	query := `SELECT id, name, prompt_template, status, created_at, finished_at FROM batch_jobs WHERE id = ?`
	job := &BatchJob{}
	var finishedAt sql.NullString
	err := db.conn.QueryRow(query, id).Scan(&job.ID, &job.Name, &job.PromptTemplate, &job.Status, &job.CreatedAt, &finishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		job.FinishedAt = &t
	}
	return job, nil
}

// UpdateJobStatus 更新批次状态
func (db *DB) UpdateJobStatus(id, status string) error {
	query := `UPDATE batch_jobs SET status = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, id)
	return err
}

// FinishJob 完成批次
func (db *DB) FinishJob(id, status string) error {
	now := time.Now()
	query := `UPDATE batch_jobs SET status = ?, finished_at = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, now.Format(time.RFC3339), id)
	return err
}

// CreateItem 创建批次项
func (db *DB) CreateItem(item *BatchItem) error {
	query := `INSERT INTO batch_items (id, batch_job_id, item_value, status, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := db.conn.Exec(query, item.ID, item.BatchJobID, item.ItemValue, item.Status, item.CreatedAt.Format(time.RFC3339))
	return err
}

// GetItem 获取批次项
func (db *DB) GetItem(id string) (*BatchItem, error) {
	query := `SELECT id, batch_job_id, item_value, status, stdout, stderr, exit_code, created_at, finished_at FROM batch_items WHERE id = ?`
	item := &BatchItem{}
	var exitCode sql.NullInt64
	var finishedAt sql.NullString
	err := db.conn.QueryRow(query, id).Scan(&item.ID, &item.BatchJobID, &item.ItemValue, &item.Status, &item.Stdout, &item.Stderr, &exitCode, &item.CreatedAt, &finishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if exitCode.Valid {
		ec := int(exitCode.Int64)
		item.ExitCode = &ec
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		item.FinishedAt = &t
	}
	return item, nil
}

// ListItems 获取批次的所有项
func (db *DB) ListItems(jobID string) ([]*BatchItem, error) {
	query := `SELECT id, batch_job_id, item_value, status, stdout, stderr, exit_code, created_at, finished_at FROM batch_items WHERE batch_job_id = ? ORDER BY created_at`
	rows, err := db.conn.Query(query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BatchItem
	for rows.Next() {
		item := &BatchItem{}
		var exitCode sql.NullInt64
		var finishedAt sql.NullString
		err := rows.Scan(&item.ID, &item.BatchJobID, &item.ItemValue, &item.Status, &item.Stdout, &item.Stderr, &exitCode, &item.CreatedAt, &finishedAt)
		if err != nil {
			return nil, err
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			item.ExitCode = &ec
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			item.FinishedAt = &t
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateItemStatus 更新批次项状态
func (db *DB) UpdateItemStatus(id, status string) error {
	query := `UPDATE batch_items SET status = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, id)
	return err
}

// FinishItem 完成批次项
func (db *DB) FinishItem(id, status, stdout, stderr string, exitCode *int) error {
	now := time.Now()
	query := `UPDATE batch_items SET status = ?, stdout = ?, stderr = ?, exit_code = ?, finished_at = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, stdout, stderr, exitCode, now.Format(time.RFC3339), id)
	return err
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
