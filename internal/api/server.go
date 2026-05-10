package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coso/qcloop/internal/core"
	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

// Server HTTP API 服务器
type Server struct {
	database *db.DB
	runner   *core.Runner
	queue    *core.QueueManager
	mux      *http.ServeMux
	wsHub    *WSHub
}

// NewServer 创建 API 服务器
func NewServer(database *db.DB) *Server {
	return NewServerWithQueueOptions(database, core.QueueOptions{})
}

// NewServerWithQueueOptions 创建带全局队列的 API 服务器。
func NewServerWithQueueOptions(database *db.DB, options core.QueueOptions) *Server {
	wsHub := NewWSHub()
	go wsHub.Run()

	runner := core.NewRunner(database)
	runner.SetBroadcaster(wsHub)
	queue := core.NewQueueManager(database, runner, options)
	queue.Start(context.Background())

	s := &Server{
		database: database,
		runner:   runner,
		queue:    queue,
		mux:      http.NewServeMux(),
		wsHub:    wsHub,
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
	s.mux.HandleFunc("/api/jobs/cancel", s.handleCancelJob)
	s.mux.HandleFunc("/api/templates", s.handleTemplates)
	s.mux.HandleFunc("/api/templates/", s.handleTemplate)
	s.mux.HandleFunc("/api/queue/metrics", s.handleQueueMetrics)
	s.mux.HandleFunc("/api/items/answer", s.handleAnswerItem)
	s.mux.HandleFunc("/api/items/retry", s.handleRetryItem)
	s.mux.HandleFunc("/api/items/cancel", s.handleCancelItem)
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
	if req.ExecutorProvider == "" {
		req.ExecutorProvider = job.ExecutorProvider
	}
	if req.MaxExecutorRetries == nil {
		existingRetries := job.MaxExecutorRetries
		req.MaxExecutorRetries = &existingRetries
	}
	if err := normalizeJobPayload(&req, job.ExecutorProvider); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job.Name = req.Name
	job.PromptTemplate = req.PromptTemplate
	job.VerifierPromptTemplate = req.VerifierPromptTemplate
	job.MaxQCRounds = req.MaxQCRounds
	job.TokenBudgetPerItem = req.TokenBudgetPerItem
	if req.MaxExecutorRetries != nil {
		job.MaxExecutorRetries = *req.MaxExecutorRetries
	}
	job.ExecutionMode = req.ExecutionMode
	job.ExecutorProvider = req.ExecutorProvider

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
	MaxExecutorRetries     *int   `json:"max_executor_retries"`
	ExecutionMode          string `json:"execution_mode"`
	ExecutorProvider       string `json:"executor_provider"`
}

func normalizeJobPayload(req *jobPayload, defaultProvider string) error {
	req.Name = strings.TrimSpace(req.Name)
	req.PromptTemplate = strings.TrimSpace(req.PromptTemplate)
	req.VerifierPromptTemplate = strings.TrimSpace(req.VerifierPromptTemplate)
	req.ExecutionMode = strings.TrimSpace(req.ExecutionMode)
	req.ExecutorProvider = strings.TrimSpace(req.ExecutorProvider)
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
	if req.MaxExecutorRetries == nil {
		defaultRetries := 1
		req.MaxExecutorRetries = &defaultRetries
	}
	if *req.MaxExecutorRetries < 0 || *req.MaxExecutorRetries > 5 {
		return fmt.Errorf("max_executor_retries must be between 0 and 5")
	}
	if req.ExecutionMode == "" {
		req.ExecutionMode = "standard"
	}
	if req.ExecutionMode != "standard" && req.ExecutionMode != "goal_assisted" {
		return fmt.Errorf("execution_mode must be standard or goal_assisted")
	}
	if req.ExecutorProvider == "" {
		req.ExecutorProvider = defaultProvider
	}
	provider, err := executor.NormalizeProvider(req.ExecutorProvider)
	if err != nil {
		return err
	}
	req.ExecutorProvider = provider
	return nil
}

type templatePayload struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	PromptTemplate         string `json:"prompt_template"`
	VerifierPromptTemplate string `json:"verifier_prompt_template"`
	MaxQCRounds            int    `json:"max_qc_rounds"`
	TokenBudgetPerItem     int    `json:"token_budget_per_item"`
	MaxExecutorRetries     *int   `json:"max_executor_retries"`
	ExecutionMode          string `json:"execution_mode"`
	ExecutorProvider       string `json:"executor_provider"`
	ItemsText              string `json:"items_text"`
}

func normalizeTemplatePayload(req *templatePayload, defaultProvider string) error {
	jobReq := jobPayload{
		Name:                   req.Name,
		PromptTemplate:         req.PromptTemplate,
		VerifierPromptTemplate: req.VerifierPromptTemplate,
		MaxQCRounds:            req.MaxQCRounds,
		TokenBudgetPerItem:     req.TokenBudgetPerItem,
		MaxExecutorRetries:     req.MaxExecutorRetries,
		ExecutionMode:          req.ExecutionMode,
		ExecutorProvider:       req.ExecutorProvider,
	}
	if err := normalizeJobPayload(&jobReq, defaultProvider); err != nil {
		return err
	}
	req.Name = jobReq.Name
	req.Description = strings.TrimSpace(req.Description)
	req.PromptTemplate = jobReq.PromptTemplate
	req.VerifierPromptTemplate = jobReq.VerifierPromptTemplate
	req.MaxQCRounds = jobReq.MaxQCRounds
	req.TokenBudgetPerItem = jobReq.TokenBudgetPerItem
	req.MaxExecutorRetries = jobReq.MaxExecutorRetries
	req.ExecutionMode = jobReq.ExecutionMode
	req.ExecutorProvider = jobReq.ExecutorProvider
	req.ItemsText = strings.TrimSpace(req.ItemsText)
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
		Mode  string `json:"mode"`
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
	} else if job.Status == "canceled" {
		http.Error(w, "canceled job cannot be run", http.StatusConflict)
		return
	}

	status, err := s.queue.EnqueueJob(req.JobID, req.Mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
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
	if err := s.queue.PauseJob(req.JobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
	status, err := s.queue.ResumeJob(req.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
	})
}

// handleCancelJob 处理批次取消。取消是不可恢复终态,区别于暂停。
func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JobID  string `json:"job_id"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.JobID) == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}
	if job, err := s.database.GetJob(req.JobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	} else if job.Status == "completed" || job.Status == "failed" || job.Status == "canceled" {
		http.Error(w, "terminal job cannot be canceled", http.StatusConflict)
		return
	}
	if err := s.queue.CancelJob(req.JobID, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "canceled",
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

func (s *Server) handleRetryItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ItemID string `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ItemID = strings.TrimSpace(req.ItemID)
	if req.ItemID == "" {
		http.Error(w, "item_id required", http.StatusBadRequest)
		return
	}
	item, err := s.database.GetItem(req.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if item == nil {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	job, err := s.database.GetJob(item.BatchJobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.Status == "canceled" {
		http.Error(w, "canceled job cannot retry items", http.StatusConflict)
		return
	}
	fresh, err := s.database.RetryItem(req.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := s.database.StartJob(item.BatchJobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.queue.Wake()
	s.runner.EmitItemUpdate(item.BatchJobID, item.ID)
	s.runner.EmitJobUpdate(item.BatchJobID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "retried",
		"item":   fresh,
	})
}

func (s *Server) handleCancelItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ItemID string `json:"item_id"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ItemID = strings.TrimSpace(req.ItemID)
	if req.ItemID == "" {
		http.Error(w, "item_id required", http.StatusBadRequest)
		return
	}
	fresh, err := s.queue.CancelItem(req.ItemID, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "canceled",
		"item":   fresh,
	})
}

func (s *Server) handleQueueMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	metrics, err := s.queue.Metrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := s.database.ListTemplates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if templates == nil {
			templates = []*db.BatchTemplate{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)
	case http.MethodPost:
		var req templatePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defaultProvider, err := executor.DefaultProviderFromEnv()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := normalizeTemplatePayload(&req, defaultProvider); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		template := &db.BatchTemplate{
			ID:                     db.GenerateID(),
			Name:                   req.Name,
			Description:            req.Description,
			PromptTemplate:         req.PromptTemplate,
			VerifierPromptTemplate: req.VerifierPromptTemplate,
			MaxQCRounds:            req.MaxQCRounds,
			TokenBudgetPerItem:     req.TokenBudgetPerItem,
			MaxExecutorRetries:     *req.MaxExecutorRetries,
			ExecutionMode:          req.ExecutionMode,
			ExecutorProvider:       req.ExecutorProvider,
			ItemsText:              req.ItemsText,
			CreatedAt:              time.Now(),
		}
		if err := s.database.CreateTemplate(template); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	if id == "" {
		http.Error(w, "template_id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		template, err := s.database.GetTemplate(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if template == nil {
			http.Error(w, "template not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)
	case http.MethodPut:
		existing, err := s.database.GetTemplate(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "template not found", http.StatusNotFound)
			return
		}
		var req templatePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.MaxExecutorRetries == nil {
			existingRetries := existing.MaxExecutorRetries
			req.MaxExecutorRetries = &existingRetries
		}
		if req.ExecutorProvider == "" {
			req.ExecutorProvider = existing.ExecutorProvider
		}
		if err := normalizeTemplatePayload(&req, existing.ExecutorProvider); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing.Name = req.Name
		existing.Description = req.Description
		existing.PromptTemplate = req.PromptTemplate
		existing.VerifierPromptTemplate = req.VerifierPromptTemplate
		existing.MaxQCRounds = req.MaxQCRounds
		existing.TokenBudgetPerItem = req.TokenBudgetPerItem
		existing.MaxExecutorRetries = *req.MaxExecutorRetries
		existing.ExecutionMode = req.ExecutionMode
		existing.ExecutorProvider = req.ExecutorProvider
		existing.ItemsText = req.ItemsText
		if err := s.database.UpdateTemplate(existing); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fresh, err := s.database.GetTemplate(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fresh)
	case http.MethodDelete:
		if err := s.database.DeleteTemplate(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAnswerItem 处理外层 AI 写回的人类确认答案。
func (s *Server) handleAnswerItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ItemID string `json:"item_id"`
		Answer string `json:"answer"`
		Resume bool   `json:"resume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ItemID = strings.TrimSpace(req.ItemID)
	req.Answer = strings.TrimSpace(req.Answer)
	if req.ItemID == "" {
		http.Error(w, "item_id required", http.StatusBadRequest)
		return
	}
	if req.Answer == "" {
		http.Error(w, "answer required", http.StatusBadRequest)
		return
	}

	item, err := s.database.GetItem(req.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if item == nil {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	if err := s.database.AnswerItemConfirmation(req.ItemID, req.Answer, req.Resume); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.Resume {
		if err := s.database.StartJob(item.BatchJobID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.queue.Wake()
	}
	s.runner.EmitItemUpdate(item.BatchJobID, item.ID)
	s.runner.EmitJobUpdate(item.BatchJobID)

	fresh, err := s.database.GetItem(req.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "answered",
		"item":   fresh,
	})
}

// listJobs 列出所有批次
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	if err := s.database.ReconcileAllDoneJobStatuses(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	query := `SELECT id, name, prompt_template, verifier_prompt_template, max_qc_rounds, token_budget_per_item, max_executor_retries, execution_mode, executor_provider, run_no, status, created_at, finished_at FROM batch_jobs ORDER BY created_at DESC`
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
		if err := rows.Scan(&job.ID, &job.Name, &job.PromptTemplate, &job.VerifierPromptTemplate, &job.MaxQCRounds, &job.TokenBudgetPerItem, &job.MaxExecutorRetries, &job.ExecutionMode, &job.ExecutorProvider, &job.RunNo, &job.Status, &createdAt, &finishedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if job.RunNo <= 0 {
			job.RunNo = 1
		}
		if job.ExecutionMode == "" {
			job.ExecutionMode = "standard"
		}
		if job.ExecutorProvider == "" {
			job.ExecutorProvider = executor.ProviderCodex
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
		MaxExecutorRetries     *int     `json:"max_executor_retries"`
		ExecutionMode          string   `json:"execution_mode"`
		ExecutorProvider       string   `json:"executor_provider"`
		Items                  []string `json:"items"`
		ItemsText              string   `json:"items_text"`
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
		MaxExecutorRetries:     req.MaxExecutorRetries,
		ExecutionMode:          req.ExecutionMode,
		ExecutorProvider:       req.ExecutorProvider,
	}
	defaultProvider, err := executor.DefaultProviderFromEnv()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := normalizeJobPayload(&payload, defaultProvider); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items, err := normalizeItems(req.Items, req.ItemsText)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(items) == 0 {
		http.Error(w, "items or items_text required", http.StatusBadRequest)
		return
	}

	job := &db.BatchJob{
		ID:                     generateID(),
		Name:                   payload.Name,
		PromptTemplate:         payload.PromptTemplate,
		VerifierPromptTemplate: payload.VerifierPromptTemplate,
		MaxQCRounds:            payload.MaxQCRounds,
		TokenBudgetPerItem:     payload.TokenBudgetPerItem,
		MaxExecutorRetries:     *payload.MaxExecutorRetries,
		ExecutionMode:          payload.ExecutionMode,
		ExecutorProvider:       payload.ExecutorProvider,
		Status:                 "pending",
		CreatedAt:              time.Now(),
	}

	if err := s.database.CreateJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 创建批次项
	for _, itemValue := range items {
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

func normalizeItems(items []string, itemsText string) ([]string, error) {
	if len(items) > 0 {
		out := make([]string, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out, nil
	}
	return parseItemsText(itemsText)
}

func parseItemsText(itemsText string) ([]string, error) {
	lines := strings.Split(itemsText, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(line), &parsed); err == nil {
			if _, ok := parsed.(map[string]interface{}); ok {
				compact, err := json.Marshal(parsed)
				if err != nil {
					return nil, err
				}
				items = append(items, string(compact))
				continue
			}
		}
		items = append(items, line)
	}
	return items, nil
}

func (s *Server) listAttempts(itemID string) ([]*db.Attempt, error) {
	query := `SELECT id, batch_item_id, attempt_no, run_no, attempt_type, status, stdout, stderr, exit_code, tokens_used, started_at, finished_at FROM attempts WHERE batch_item_id = ? ORDER BY attempt_no`
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
		if err := rows.Scan(&attempt.ID, &attempt.BatchItemID, &attempt.AttemptNo, &attempt.RunNo, &attempt.AttemptType, &attempt.Status, &stdout, &stderr, &exitCode, &attempt.TokensUsed, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if attempt.RunNo <= 0 {
			attempt.RunNo = 1
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
	query := `SELECT id, batch_item_id, qc_no, run_no, status, verdict, feedback, tokens_used, started_at, finished_at FROM qc_rounds WHERE batch_item_id = ? ORDER BY qc_no`
	rows, err := s.database.Conn().Query(query, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []*db.QCRound
	for rows.Next() {
		round := &db.QCRound{}
		var verdict, feedback, startedAt, finishedAt sql.NullString
		if err := rows.Scan(&round.ID, &round.BatchItemID, &round.QCNo, &round.RunNo, &round.Status, &verdict, &feedback, &round.TokensUsed, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if round.RunNo <= 0 {
			round.RunNo = 1
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
