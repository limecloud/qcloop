package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coso/qcloop/internal/core"
	"github.com/coso/qcloop/internal/db"
)

// Server HTTP API 服务器
type Server struct {
	database       *db.DB
	runner         *core.Runner
	mux            *http.ServeMux
	runningJobs    map[string]context.CancelFunc
	runningJobsMux sync.RWMutex
	wsHub          *WSHub
}

// NewServer 创建 API 服务器
func NewServer(database *db.DB) *Server {
	wsHub := NewWSHub()
	go wsHub.Run()

	runner := core.NewRunner(database)
	runner.SetBroadcaster(wsHub)

	s := &Server{
		database:    database,
		runner:      runner,
		mux:         http.NewServeMux(),
		runningJobs: make(map[string]context.CancelFunc),
		wsHub:       wsHub,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/jobs/", s.handleJob)
	s.mux.HandleFunc("/api/jobs/run", s.handleRunJob)
	s.mux.HandleFunc("/api/jobs/pause", s.handlePauseJob)
	s.mux.HandleFunc("/api/jobs/resume", s.handleResumeJob)
	s.mux.HandleFunc("/api/items/", s.handleItems)
	s.mux.HandleFunc("/ws", s.handleWebSocket)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

// handleJobs 处理批次列表
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.listJobs(w, r)
	} else if r.Method == "POST" {
		s.createJob(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleJob 处理单个批次
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	// 从 URL 中提取 job ID
	// /api/jobs/{id}
	path := r.URL.Path
	id := path[len("/api/jobs/"):]

	if id == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getJob(w, id)
	case http.MethodPut:
		s.updateJob(w, r, id)
	case http.MethodDelete:
		s.deleteJob(w, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getJob(w http.ResponseWriter, id string) {
	job, err := s.database.GetJob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request, id string) {
	job, err := s.database.GetJob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.Status == "running" {
		http.Error(w, "running job cannot be updated", http.StatusConflict)
		return
	}

	var req jobPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := normalizeJobPayload(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job.Name = req.Name
	job.PromptTemplate = req.PromptTemplate
	job.VerifierPromptTemplate = req.VerifierPromptTemplate
	job.MaxQCRounds = req.MaxQCRounds
	job.TokenBudgetPerItem = req.TokenBudgetPerItem
	job.ExecutionMode = req.ExecutionMode

	if err := s.database.UpdateJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fresh, err := s.database.GetJob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fresh)
}

func (s *Server) deleteJob(w http.ResponseWriter, id string) {
	job, err := s.database.GetJob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.Status == "running" {
		http.Error(w, "running job cannot be deleted", http.StatusConflict)
		return
	}
	if err := s.database.DeleteJob(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	})
}

type jobPayload struct {
	Name                   string `json:"name"`
	PromptTemplate         string `json:"prompt_template"`
	VerifierPromptTemplate string `json:"verifier_prompt_template"`
	MaxQCRounds            int    `json:"max_qc_rounds"`
	TokenBudgetPerItem     int    `json:"token_budget_per_item"`
	ExecutionMode          string `json:"execution_mode"`
}

func normalizeJobPayload(req *jobPayload) error {
	req.Name = strings.TrimSpace(req.Name)
	req.PromptTemplate = strings.TrimSpace(req.PromptTemplate)
	req.VerifierPromptTemplate = strings.TrimSpace(req.VerifierPromptTemplate)
	req.ExecutionMode = strings.TrimSpace(req.ExecutionMode)
	if req.Name == "" {
		return fmt.Errorf("name required")
	}
	if req.PromptTemplate == "" {
		return fmt.Errorf("prompt_template required")
	}
	if req.MaxQCRounds <= 0 {
		req.MaxQCRounds = 3
	}
	if req.TokenBudgetPerItem < 0 {
		return fmt.Errorf("token_budget_per_item must be >= 0")
	}
	if req.ExecutionMode == "" {
		req.ExecutionMode = "standard"
	}
	if req.ExecutionMode != "standard" && req.ExecutionMode != "goal_assisted" {
		return fmt.Errorf("execution_mode must be standard or goal_assisted")
	}
	return nil
}

// handleRunJob 处理运行批次
func (s *Server) handleRunJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JobID string `json:"job_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.JobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}
	if job, err := s.database.GetJob(req.JobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	status, err := s.startRun(req.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
	})
}

func (s *Server) startRun(jobID string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s.runningJobsMux.Lock()
	if _, exists := s.runningJobs[jobID]; exists {
		s.runningJobsMux.Unlock()
		cancel()
		return "already_running", nil
	}
	s.runningJobs[jobID] = cancel
	s.runningJobsMux.Unlock()

	cleanup := func() {
		s.runningJobsMux.Lock()
		delete(s.runningJobs, jobID)
		s.runningJobsMux.Unlock()
	}

	// 先同步切换可见状态再返回,避免 UI 头部显示 running、明细仍停在旧成功态。
	if _, _, err := s.runner.PrepareRun(jobID); err != nil {
		cleanup()
		cancel()
		return "", err
	}

	go func() {
		defer cleanup()
		if err := s.runner.RunBatch(ctx, jobID); err != nil {
			fmt.Printf("RunBatch failed: %v\n", err)
		}
	}()

	return "started", nil
}

// handlePauseJob 处理暂停批次
func (s *Server) handlePauseJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JobID string `json:"job_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 取消正在运行的批次
	s.runningJobsMux.Lock()
	cancel, exists := s.runningJobs[req.JobID]
	s.runningJobsMux.Unlock()

	if exists {
		cancel()
		// 更新批次状态为 paused
		if err := s.database.UpdateJobStatus(req.JobID, "paused"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if job, _ := s.database.GetJob(req.JobID); job != nil {
			s.wsHub.BroadcastJobUpdate(req.JobID, job)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "paused",
	})
}

// handleResumeJob 处理恢复批次
func (s *Server) handleResumeJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JobID string `json:"job_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.JobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}
	if job, err := s.database.GetJob(req.JobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	status, err := s.startRun(req.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
	})
}

// handleItems 处理批次项列表
func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	items, err := s.database.ListItems(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取每个 item 的 attempts 和 qc_rounds
	type ItemDetail struct {
		*db.BatchItem
		Attempts []*db.Attempt `json:"attempts"`
		QCRounds []*db.QCRound `json:"qc_rounds"`
	}

	var details []ItemDetail
	for _, item := range items {
		attempts, _ := s.listAttempts(item.ID)
		qcRounds, _ := s.listQCRounds(item.ID)
		if attempts == nil {
			attempts = []*db.Attempt{}
		}
		if qcRounds == nil {
			qcRounds = []*db.QCRound{}
		}
		details = append(details, ItemDetail{
			BatchItem: item,
			Attempts:  attempts,
			QCRounds:  qcRounds,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(details)
}

// listJobs 列出所有批次
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, name, prompt_template, verifier_prompt_template, max_qc_rounds, token_budget_per_item, execution_mode, status, created_at, finished_at FROM batch_jobs ORDER BY created_at DESC`
	rows, err := s.database.Conn().Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var jobs []*db.BatchJob
	for rows.Next() {
		job := &db.BatchJob{}
		var createdAt sql.NullString
		var finishedAt sql.NullString
		if err := rows.Scan(&job.ID, &job.Name, &job.PromptTemplate, &job.VerifierPromptTemplate, &job.MaxQCRounds, &job.TokenBudgetPerItem, &job.ExecutionMode, &job.Status, &createdAt, &finishedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if createdAt.Valid {
			t, _ := time.Parse(time.RFC3339, createdAt.String)
			job.CreatedAt = t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			job.FinishedAt = &t
		}
		jobs = append(jobs, job)
	}

	if jobs == nil {
		jobs = []*db.BatchJob{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// createJob 创建批次
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                   string   `json:"name"`
		PromptTemplate         string   `json:"prompt_template"`
		VerifierPromptTemplate string   `json:"verifier_prompt_template"`
		MaxQCRounds            int      `json:"max_qc_rounds"`
		TokenBudgetPerItem     int      `json:"token_budget_per_item"`
		ExecutionMode          string   `json:"execution_mode"`
		Items                  []string `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := jobPayload{
		Name:                   req.Name,
		PromptTemplate:         req.PromptTemplate,
		VerifierPromptTemplate: req.VerifierPromptTemplate,
		MaxQCRounds:            req.MaxQCRounds,
		TokenBudgetPerItem:     req.TokenBudgetPerItem,
		ExecutionMode:          req.ExecutionMode,
	}
	if err := normalizeJobPayload(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job := &db.BatchJob{
		ID:                     generateID(),
		Name:                   payload.Name,
		PromptTemplate:         payload.PromptTemplate,
		VerifierPromptTemplate: payload.VerifierPromptTemplate,
		MaxQCRounds:            payload.MaxQCRounds,
		TokenBudgetPerItem:     payload.TokenBudgetPerItem,
		ExecutionMode:          payload.ExecutionMode,
		Status:                 "pending",
		CreatedAt:              time.Now(),
	}

	if err := s.database.CreateJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 创建批次项
	for _, itemValue := range req.Items {
		item := &db.BatchItem{
			ID:         generateID(),
			BatchJobID: job.ID,
			ItemValue:  itemValue,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}
		if err := s.database.CreateItem(item); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) listAttempts(itemID string) ([]*db.Attempt, error) {
	query := `SELECT id, batch_item_id, attempt_no, attempt_type, status, stdout, stderr, exit_code, tokens_used, started_at, finished_at FROM attempts WHERE batch_item_id = ? ORDER BY attempt_no`
	rows, err := s.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []*db.Attempt
	for rows.Next() {
		attempt := &db.Attempt{}
		var stdout, stderr, startedAt, finishedAt sql.NullString
		var exitCode sql.NullInt64
		if err := rows.Scan(&attempt.ID, &attempt.BatchItemID, &attempt.AttemptNo, &attempt.AttemptType, &attempt.Status, &stdout, &stderr, &exitCode, &attempt.TokensUsed, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if stdout.Valid {
			attempt.Stdout = stdout.String
		}
		if stderr.Valid {
			attempt.Stderr = stderr.String
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			attempt.ExitCode = &ec
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			attempt.StartedAt = t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			attempt.FinishedAt = &t
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Server) listQCRounds(itemID string) ([]*db.QCRound, error) {
	query := `SELECT id, batch_item_id, qc_no, status, verdict, feedback, tokens_used, started_at, finished_at FROM qc_rounds WHERE batch_item_id = ? ORDER BY qc_no`
	rows, err := s.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []*db.QCRound
	for rows.Next() {
		round := &db.QCRound{}
		var verdict, feedback, startedAt, finishedAt sql.NullString
		if err := rows.Scan(&round.ID, &round.BatchItemID, &round.QCNo, &round.Status, &verdict, &feedback, &round.TokensUsed, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if verdict.Valid {
			round.Verdict = verdict.String
		}
		if feedback.Valid {
			round.Feedback = feedback.String
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			round.StartedAt = t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			round.FinishedAt = &t
		}
		rounds = append(rounds, round)
	}
	return rounds, rows.Err()
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
