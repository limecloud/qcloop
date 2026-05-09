package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coso/qcloop/internal/core"
	"github.com/coso/qcloop/internal/db"
)

// Server HTTP API 服务器
type Server struct {
	database *db.DB
	runner   *core.Runner
	mux      *http.ServeMux
}

// NewServer 创建 API 服务器
func NewServer(database *db.DB) *Server {
	s := &Server{
		database: database,
		runner:   core.NewRunner(database),
		mux:      http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/jobs/", s.handleJob)
	s.mux.HandleFunc("/api/jobs/run", s.handleRunJob)
	s.mux.HandleFunc("/api/items/", s.handleItems)
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

// handleJobs 处理批次列表和创建
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.listJobs(w, r)
	case "POST":
		s.createJob(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleJob 处理单个批次
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Path[len("/api/jobs/"):]
	if jobID == "" {
		http.Error(w, "Job ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		s.getJob(w, r, jobID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunJob 运行批次
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
		ctx := context.Background()
		s.runner.RunBatch(ctx, req.JobID)
	}()

	json.NewEncoder(w).Encode(map[string]string{
		"status": "started",
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
	// 简化实现：返回空列表
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

// createJob 创建批次
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                   string   `json:"name"`
		PromptTemplate         string   `json:"prompt_template"`
		VerifierPromptTemplate string   `json:"verifier_prompt_template"`
		MaxQCRounds            int      `json:"max_qc_rounds"`
		Items                  []string `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.MaxQCRounds == 0 {
		req.MaxQCRounds = 3
	}

	jobID := db.GenerateID()
	job := &db.BatchJob{
		ID:                     jobID,
		Name:                   req.Name,
		PromptTemplate:         req.PromptTemplate,
		VerifierPromptTemplate: req.VerifierPromptTemplate,
		MaxQCRounds:            req.MaxQCRounds,
		Status:                 "pending",
		CreatedAt:              time.Now(),
	}

	if err := s.database.CreateJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, itemValue := range req.Items {
		item := &db.BatchItem{
			ID:         db.GenerateID(),
			BatchJobID: jobID,
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

// getJob 获取批次详情
func (s *Server) getJob(w http.ResponseWriter, r *http.Request, jobID string) {
	job, err := s.database.GetJob(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// 辅助方法
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
		var exitCode, finishedAt interface{}
		if err := rows.Scan(&attempt.ID, &attempt.BatchItemID, &attempt.AttemptNo, &attempt.AttemptType, &attempt.Status, &attempt.Stdout, &attempt.Stderr, &exitCode, &attempt.StartedAt, &finishedAt); err != nil {
			return nil, err
		}
		if exitCode != nil {
			ec := int(exitCode.(int64))
			attempt.ExitCode = &ec
		}
		if finishedAt != nil {
			t, _ := time.Parse(time.RFC3339, finishedAt.(string))
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

	var qcRounds []*db.QCRound
	for rows.Next() {
		qc := &db.QCRound{}
		var finishedAt interface{}
		if err := rows.Scan(&qc.ID, &qc.BatchItemID, &qc.QCNo, &qc.Status, &qc.Verdict, &qc.Feedback, &qc.StartedAt, &finishedAt); err != nil {
			return nil, err
		}
		if finishedAt != nil {
			t, _ := time.Parse(time.RFC3339, finishedAt.(string))
			qc.FinishedAt = &t
		}
		qcRounds = append(qcRounds, qc)
	}
	return qcRounds, rows.Err()
}

// Start 启动服务器
func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s)
}
