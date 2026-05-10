package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newDAOTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "qcloop-dao-test.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		_ = os.Remove(path)
	})
	return database
}

func TestBatchTemplateCRUD(t *testing.T) {
	database := newDAOTestDB(t)
	template := &BatchTemplate{
		ID:                     GenerateID(),
		Name:                   "文档批量 review",
		Description:            "按文件逐个审查并修复",
		PromptTemplate:         "review {{item}}",
		VerifierPromptTemplate: "verify {{output}}",
		MaxQCRounds:            5,
		TokenBudgetPerItem:     12000,
		MaxExecutorRetries:     2,
		ExecutionMode:          "standard",
		ExecutorProvider:       "codex",
		ItemsText:              "docs/a.md\ndocs/b.md",
		CreatedAt:              time.Now(),
	}
	if err := database.CreateTemplate(template); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	got, err := database.GetTemplate(template.ID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got == nil || got.Name != template.Name || got.ItemsText != template.ItemsText {
		t.Fatalf("template after create = %#v", got)
	}

	got.Name = "文档批量 review v2"
	got.MaxExecutorRetries = 3
	got.ItemsText = "docs/c.md"
	if err := database.UpdateTemplate(got); err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	updated, err := database.GetTemplate(template.ID)
	if err != nil {
		t.Fatalf("GetTemplate updated: %v", err)
	}
	if updated.Name != "文档批量 review v2" || updated.MaxExecutorRetries != 3 || updated.ItemsText != "docs/c.md" || updated.UpdatedAt == nil {
		t.Fatalf("template after update = %#v", updated)
	}

	templates, err := database.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].ID != template.ID {
		t.Fatalf("templates = %#v", templates)
	}

	if err := database.DeleteTemplate(template.ID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	deleted, err := database.GetTemplate(template.ID)
	if err != nil {
		t.Fatalf("GetTemplate deleted: %v", err)
	}
	if deleted != nil {
		t.Fatalf("template should be deleted, got %#v", deleted)
	}
}

func TestRetryItemPreservesHistoryAndQueuesNextAttemptNumber(t *testing.T) {
	database := newDAOTestDB(t)
	jobID := GenerateID()
	job := &BatchJob{
		ID:             jobID,
		Name:           "单项重试",
		PromptTemplate: "do {{item}}",
		MaxQCRounds:    3,
		Status:         "failed",
		CreatedAt:      time.Now(),
	}
	if err := database.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	itemID := GenerateID()
	if err := database.CreateItem(&BatchItem{
		ID:               itemID,
		BatchJobID:       jobID,
		ItemValue:        "docs/a.md",
		Status:           "failed",
		CurrentAttemptNo: 2,
		CurrentQCNo:      1,
		TokensUsed:       100,
		CreatedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if _, err := database.Conn().Exec(`UPDATE batch_items SET current_attempt_no = 2, current_qc_no = 1, tokens_used = 100, last_error = 'old' WHERE id = ?`, itemID); err != nil {
		t.Fatalf("seed item counters: %v", err)
	}
	_, err := database.Conn().Exec(
		`INSERT INTO attempts (id, batch_item_id, attempt_no, run_no, attempt_type, status, started_at) VALUES (?, ?, 7, 1, 'worker', 'failed', ?)`,
		GenerateID(),
		itemID,
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert attempt: %v", err)
	}

	fresh, err := database.RetryItem(itemID)
	if err != nil {
		t.Fatalf("RetryItem: %v", err)
	}
	if fresh.Status != "pending" || fresh.CurrentAttemptNo != 0 || fresh.CurrentQCNo != 0 || fresh.TokensUsed != 0 || fresh.QueuedAt == nil {
		t.Fatalf("fresh item after retry = %#v", fresh)
	}
	var attemptCount int
	if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM attempts WHERE batch_item_id = ?`, itemID).Scan(&attemptCount); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("attempt history count = %d, want preserved 1", attemptCount)
	}
}
