package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coso/qcloop/internal/api"
	"github.com/coso/qcloop/internal/core"
	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
	"github.com/spf13/cobra"
)

var dbPath string

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "qcloop",
	Short: "qcloop - 批量测试编排工具",
	Long:  `qcloop 是一个批量测试编排工具，支持多轮质检和自动返修。`,
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "创建批次",
	RunE:  runCreate,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "运行批次",
	RunE:  runRun,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查询状态",
	RunE:  runStatus,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 HTTP API 服务器",
	RunE:  runServe,
}

var (
	createName           string
	createPrompt         string
	createVerifierPrompt string
	createItems          string
	createMaxQCRounds    int
	createTokenBudget    int
	createExecutionMode  string
	createExecutor       string

	runJobID     string
	statusJobID  string
	serveAddr    string
	serveWorkers int
)

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", getDefaultDBPath(), "数据库路径")

	createCmd.Flags().StringVar(&createName, "name", "", "批次名称（必填）")
	createCmd.Flags().StringVar(&createPrompt, "prompt", "", "prompt 模板（必填）")
	createCmd.Flags().StringVar(&createVerifierPrompt, "verifier-prompt", "", "verifier 模板")
	createCmd.Flags().StringVar(&createItems, "items", "", "测试项列表，逗号分隔（必填）")
	createCmd.Flags().IntVar(&createMaxQCRounds, "max-qc-rounds", 3, "最大质检轮次")
	createCmd.Flags().IntVar(&createTokenBudget, "token-budget", 0, "每个 item 的 token 预算，超出后标记 exhausted；0 表示不限制")
	createCmd.Flags().StringVar(&createExecutionMode, "execution-mode", "standard", "执行模式:standard | goal_assisted(Codex Goal 风格 prompt 包装)")
	createCmd.Flags().StringVar(&createExecutor, "executor-provider", "", "执行器:codex | claude_code | gemini_cli | kiro_cli；空值使用 QCLOOP_EXECUTOR_PROVIDER 或 codex")
	createCmd.MarkFlagRequired("name")
	createCmd.MarkFlagRequired("prompt")
	createCmd.MarkFlagRequired("items")

	runCmd.Flags().StringVar(&runJobID, "job-id", "", "批次 ID（必填）")
	runCmd.MarkFlagRequired("job-id")

	statusCmd.Flags().StringVar(&statusJobID, "job-id", "", "批次 ID（必填）")
	statusCmd.MarkFlagRequired("job-id")

	serveCmd.Flags().StringVar(&serveAddr, "addr", ":8080", "服务器地址")
	serveCmd.Flags().IntVar(&serveWorkers, "workers", defaultWorkerCount(), "全局队列并发 worker 数")

	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(serveCmd)
}

func getDefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".qcloop", "qcloop.db")
}

func runCreate(cmd *cobra.Command, args []string) error {
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	items := strings.Split(createItems, ",")
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	provider := createExecutor
	if strings.TrimSpace(provider) == "" {
		provider, err = executor.DefaultProviderFromEnv()
	} else {
		provider, err = executor.NormalizeProvider(provider)
	}
	if err != nil {
		return err
	}

	jobID := db.GenerateID()
	job := &db.BatchJob{
		ID:                     jobID,
		Name:                   createName,
		PromptTemplate:         createPrompt,
		VerifierPromptTemplate: createVerifierPrompt,
		MaxQCRounds:            createMaxQCRounds,
		TokenBudgetPerItem:     createTokenBudget,
		ExecutionMode:          createExecutionMode,
		ExecutorProvider:       provider,
		Status:                 "pending",
		CreatedAt:              time.Now(),
	}

	if err := database.CreateJob(job); err != nil {
		return err
	}

	for _, itemValue := range items {
		item := &db.BatchItem{
			ID:         db.GenerateID(),
			BatchJobID: jobID,
			ItemValue:  itemValue,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}
		if err := database.CreateItem(item); err != nil {
			return err
		}
	}

	fmt.Println("✅ 批次创建成功")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("批次 ID: %s\n", jobID)
	fmt.Printf("批次名称: %s\n", createName)
	fmt.Printf("测试项数: %d\n", len(items))
	fmt.Printf("最大质检轮次: %d\n", createMaxQCRounds)
	fmt.Printf("执行器: %s\n", provider)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf("运行批次: qcloop run --job-id %s\n", jobID)
	fmt.Printf("查询状态: qcloop status --job-id %s\n", jobID)

	return nil
}

func runRun(cmd *cobra.Command, args []string) error {
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	job, err := database.GetJob(runJobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("批次不存在: %s", runJobID)
	}

	fmt.Printf("🚀 开始执行批次: %s\n", job.Name)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	runner := core.NewRunner(database)
	ctx := context.Background()

	if err := runner.RunBatch(ctx, runJobID); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ 批次执行完成")

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	job, err := database.GetJob(statusJobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("批次不存在: %s", statusJobID)
	}

	total, success, failed, pending, err := database.GetJobStats(statusJobID)
	if err != nil {
		return err
	}

	fmt.Printf("批次状态: %s\n", job.Name)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("批次 ID: %s\n", job.ID)
	fmt.Printf("状态: %s\n", job.Status)
	fmt.Printf("创建时间: %s\n", job.CreatedAt.Format("2006-01-02 15:04:05"))
	if job.FinishedAt != nil {
		fmt.Printf("完成时间: %s\n", job.FinishedAt.Format("2006-01-02 15:04:05"))
		duration := job.FinishedAt.Sub(job.CreatedAt)
		fmt.Printf("总耗时: %s\n", duration.Round(time.Second))
	}
	fmt.Println()
	fmt.Println("测试项统计:")
	fmt.Printf("  总数: %d\n", total)
	if total > 0 {
		fmt.Printf("  ✅ 成功: %d (%.1f%%)\n", success, float64(success)/float64(total)*100)
		fmt.Printf("  ❌ 失败: %d (%.1f%%)\n", failed, float64(failed)/float64(total)*100)
		fmt.Printf("  ⏳ 待处理: %d (%.1f%%)\n", pending, float64(pending)/float64(total)*100)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

func runServe(cmd *cobra.Command, args []string) error {
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	server := api.NewServerWithQueueOptions(database, core.QueueOptions{
		WorkerCount:   serveWorkers,
		LeaseDuration: core.DefaultLeaseDuration,
		PollInterval:  core.DefaultPollInterval,
	})

	fmt.Printf("🚀 启动 HTTP API 服务器: %s\n", serveAddr)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("全局队列 worker: %d\n", serveWorkers)
	fmt.Println("API 端点:")
	fmt.Println("  POST   /api/jobs          - 创建批次")
	fmt.Println("  GET    /api/jobs/:id      - 获取批次")
	fmt.Println("  POST   /api/jobs/run      - 运行批次")
	fmt.Println("  POST   /api/jobs/pause    - 暂停批次")
	fmt.Println("  POST   /api/jobs/resume   - 恢复批次")
	fmt.Println("  GET    /api/items?job_id= - 获取批次项")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return http.ListenAndServe(serveAddr, server)
}

func defaultWorkerCount() int {
	value := strings.TrimSpace(os.Getenv("QCLOOP_WORKER_COUNT"))
	if value == "" {
		return core.DefaultWorkerCount
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return core.DefaultWorkerCount
	}
	return parsed
}
