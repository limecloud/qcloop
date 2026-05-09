package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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

	// 在后台运行批次
	go func() {
		ctx, cancel := context.WithCancel(context.Background())

		s.runningJobsMux.Lock()
		s.runningJobs[req.JobID] = cancel
		s.runningJobsMux.Unlock()

		defer func() {
			s.runningJobsMux.Lock()
			delete(s.runningJobs, req.JobID)
			s.runningJobsMux.Unlock()
		}()

		s.runner.RunBatch(ctx, req.JobID)
	}()

	json.NewEncoder(w).Encode(map[string]string{
		"status": "started",
	})
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

	// 更新批次状态为 running
	if err := s.database.UpdateJobStatus(req.JobID, "running"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job, _ := s.database.GetJob(req.JobID); job != nil {
		s.wsHub.BroadcastJobUpdate(req.JobID, job)
	}

	// 在后台继续运行批次
	go func() {
		ctx, cancel := context.WithCancel(context.Background())

		s.runningJobsMux.Lock()
		s.runningJobs[req.JobID] = cancel
		s.runningJobsMux.Unlock()

		defer func() {
			s.runningJobsMux.Lock()
			delete(s.runningJobs, req.JobID)
			s.runningJobsMux.Unlock()
		}()

		s.runner.RunBatch(ctx, req.JobID)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "resumed",
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

	if req.ExecutionMode == "" {
		req.ExecutionMode = "standard"
	}

	job := &db.BatchJob{
		ID:                     generateID(),
		Name:                   req.Name,
		PromptTemplate:         req.PromptTemplate,
		VerifierPromptTemplate: req.VerifierPromptTemplate,
		MaxQCRounds:            req.MaxQCRounds,
		TokenBudgetPerItem:     req.TokenBudgetPerItem,
		ExecutionMode:          req.ExecutionMode,
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
			ID:          generateID(),
			BatchJobID:  job.ID,
			ItemValue:   itemValue,
			Status:      "pending",
			CreatedAt:   time.Now(),
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
	query := `SELECT id, batch_item_id, attempt_no, attempt_type, status, stdout, stderr, exit_code, started_at, finished_at FROM attempts WHERE batch_item_id = ? ORDER BY attempt_no`
	rows, err := s.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []*db.Attempt
	for rows.Next() {
		attempt := &db.Attempt{}
		var exitCode interface{}
		var startedAt, finishedAt string
		if err := rows.Scan(&attempt.ID, &attempt.BatchItemID, &attempt.AttemptNo, &attempt.AttemptType, &attempt.Status, &attempt.Stdout, &attempt.Stderr, &exitCode, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if exitCode != nil {
			ec := int(exitCode.(int64))
			attempt.ExitCode = &ec
		}
		if startedAt != "" {
			t, _ := time.Parse(time.RFC3339, startedAt)
			attempt.StartedAt = t
		}
		if finishedAt != "" {
			t, _ := time.Parse(time.RFC3339, finishedAt)
			attempt.FinishedAt = &t
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Server) listQCRounds(itemID string) ([]*db.QCRound, error) {
	query := `SELECT id, batch_item_id, qc_no, status, verdict, feedback, started_at, finished_at FROM qc_rounds WHERE batch_item_id = ? ORDER BY qc_no`
	rows, err := s.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []*db.QCRound
	for rows.Next() {
		round := &db.QCRound{}
		var startedAt, finishedAt string
		if err := rows.Scan(&round.ID, &round.BatchItemID, &round.QCNo, &round.Status, &round.Verdict, &round.Feedback, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if startedAt != "" {
			t, _ := time.Parse(time.RFC3339, startedAt)
			round.StartedAt = t
		}
		if finishedAt != "" {
			t, _ := time.Parse(time.RFC3339, finishedAt)
			round.FinishedAt = &t
		}
		rounds = append(rounds, round)
	}
	return rounds, rows.Err()
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
