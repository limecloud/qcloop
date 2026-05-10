package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coso/qcloop/internal/db"
)

const (
	DefaultWorkerCount   = 2
	DefaultLeaseDuration = 15 * time.Minute
	DefaultPollInterval  = 1 * time.Second
)

type QueueOptions struct {
	WorkerCount   int
	LeaseDuration time.Duration
	PollInterval  time.Duration
}

// QueueManager 是 serve 模式下的全局队列调度器。
// 它只通过 SQLite claim pending item,因此进程重启后仍能从 DB 恢复。
type QueueManager struct {
	database *db.DB
	runner   *Runner
	options  QueueOptions

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	wake chan struct{}

	activeMu sync.Mutex
	active   map[string]map[string]context.CancelFunc // jobID -> itemID -> cancel
}

func NewQueueManager(database *db.DB, runner *Runner, options QueueOptions) *QueueManager {
	if options.WorkerCount <= 0 {
		options.WorkerCount = DefaultWorkerCount
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = DefaultLeaseDuration
	}
	if options.PollInterval <= 0 {
		options.PollInterval = DefaultPollInterval
	}
	return &QueueManager{
		database: database,
		runner:   runner,
		options:  options,
		wake:     make(chan struct{}, 1),
		active:   make(map[string]map[string]context.CancelFunc),
	}
}

func (q *QueueManager) Start(parent context.Context) {
	if q.ctx != nil {
		return
	}
	q.ctx, q.cancel = context.WithCancel(parent)
	for i := 0; i < q.options.WorkerCount; i++ {
		workerID := fmt.Sprintf("qcloop-worker-%d", i+1)
		q.wg.Add(1)
		go q.workerLoop(workerID)
	}
	q.signal()
}

func (q *QueueManager) Stop() {
	if q.cancel != nil {
		q.cancel()
	}
	q.cancelAll()
	q.wg.Wait()
}

func (q *QueueManager) EnqueueJob(jobID, mode string) (string, error) {
	if _, _, err := q.runner.PrepareRunMode(jobID, mode); err != nil {
		return "", err
	}
	if done, _, err := q.database.FinishJobIfDone(jobID); err == nil && done {
		q.runner.emitJobUpdate(jobID)
		return "started", nil
	}
	q.signal()
	return "started", nil
}

func (q *QueueManager) PauseJob(jobID string) error {
	if err := q.database.UpdateJobStatus(jobID, "paused"); err != nil {
		return err
	}
	q.cancelJob(jobID)
	q.runner.emitJobUpdate(jobID)
	return nil
}

func (q *QueueManager) ResumeJob(jobID string) (string, error) {
	return q.EnqueueJob(jobID, RunModeContinue)
}

func (q *QueueManager) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *QueueManager) workerLoop(workerID string) {
	defer q.wg.Done()
	ticker := time.NewTicker(q.options.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		default:
		}

		_, _ = q.database.RecoverStaleRunningItems(time.Now())
		job, item, err := q.database.ClaimNextItem(workerID, time.Now().Add(q.options.LeaseDuration))
		if err != nil {
			fmt.Printf("queue claim failed: %v\n", err)
			q.wait(ticker)
			continue
		}
		if item == nil || job == nil {
			q.wait(ticker)
			continue
		}

		q.runner.emitItemUpdate(job.ID, item.ID)
		q.processClaimedItem(workerID, job, item)
	}
}

func (q *QueueManager) wait(ticker *time.Ticker) {
	select {
	case <-q.ctx.Done():
	case <-q.wake:
	case <-ticker.C:
	}
}

func (q *QueueManager) processClaimedItem(workerID string, job *db.BatchJob, item *db.BatchItem) {
	itemCtx, cancel := context.WithCancel(q.ctx)
	q.addActive(job.ID, item.ID, cancel)
	defer q.removeActive(job.ID, item.ID)

	heartbeatCtx, stopHeartbeat := context.WithCancel(q.ctx)
	defer stopHeartbeat()
	go q.renewLease(heartbeatCtx, item.ID, workerID)

	err := q.runner.processItem(itemCtx, job, item)
	cancel()
	stopHeartbeat()

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Printf("queue item failed: job=%s item=%s err=%v\n", job.ID, item.ID, err)
		_ = q.database.RequeueItem(item.ID, err.Error())
		q.runner.emitItemUpdate(job.ID, item.ID)
		return
	}

	done, _, err := q.database.FinishJobIfDone(job.ID)
	if err != nil {
		fmt.Printf("queue complete job failed: %v\n", err)
	} else if done {
		q.runner.emitJobUpdate(job.ID)
	}
	q.signal()
}

func (q *QueueManager) renewLease(ctx context.Context, itemID, owner string) {
	interval := q.options.LeaseDuration / 3
	if interval <= 0 {
		interval = time.Second
	}
	if interval > time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = q.database.RenewItemLease(itemID, owner, time.Now().Add(q.options.LeaseDuration))
		}
	}
}

func (q *QueueManager) addActive(jobID, itemID string, cancel context.CancelFunc) {
	q.activeMu.Lock()
	defer q.activeMu.Unlock()
	if q.active[jobID] == nil {
		q.active[jobID] = make(map[string]context.CancelFunc)
	}
	q.active[jobID][itemID] = cancel
}

func (q *QueueManager) removeActive(jobID, itemID string) {
	q.activeMu.Lock()
	defer q.activeMu.Unlock()
	delete(q.active[jobID], itemID)
	if len(q.active[jobID]) == 0 {
		delete(q.active, jobID)
	}
}

func (q *QueueManager) cancelJob(jobID string) {
	q.activeMu.Lock()
	defer q.activeMu.Unlock()
	for _, cancel := range q.active[jobID] {
		cancel()
	}
}

func (q *QueueManager) cancelAll() {
	q.activeMu.Lock()
	defer q.activeMu.Unlock()
	for _, items := range q.active {
		for _, cancel := range items {
			cancel()
		}
	}
}
