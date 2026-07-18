package worker

import (
	"context"
	"database/sql"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"queuectl/internal/config"
	"queuectl/internal/job"
	"queuectl/internal/storage"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Manager handles the worker pool.
type Manager struct {
	store  *storage.Storage
	cfgMgr *config.Manager
	logger *slog.Logger
}

// NewManager creates a new worker Manager.
func NewManager(store *storage.Storage, cfgMgr *config.Manager) *Manager {
	return &Manager{
		store:  store,
		cfgMgr: cfgMgr,
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

// Start spawns count workers and blocks until a termination signal is received.
func (m *Manager) Start(count int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		m.logger.Info("Shutdown signal received, waiting for workers to finish...")
		cancel()
	}()

	m.StartWithContext(ctx, count)
}

// StartWithContext spawns count workers and blocks until the context is canceled.
func (m *Manager) StartWithContext(ctx context.Context, count int) {
	var wg sync.WaitGroup

	m.logger.Info("Starting worker pool", "count", count)

	for i := 1; i <= count; i++ {
		wg.Add(1)
		go m.workerLoop(ctx, &wg, i)
	}

	wg.Wait()
	m.logger.Info("All workers gracefully stopped.")
}

func (m *Manager) workerLoop(ctx context.Context, wg *sync.WaitGroup, id int) {
	defer wg.Done()

	ticker := time.NewTicker(2 * time.Second) // poll interval
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Worker stopped", "id", id)
			return
		case <-ticker.C:
			// Attempt to claim a job
			j, err := m.claimJob()
			if err != nil {
				m.logger.Error("Failed to claim job", "error", err)
				continue
			}
			if j == nil {
				continue // No jobs available
			}

			m.logger.Info("Job claimed", "worker_id", id, "job_id", j.ID)

			jobCtx := ctx
			var cancel context.CancelFunc
			if j.Timeout > 0 {
				jobCtx, cancel = context.WithTimeout(ctx, time.Duration(j.Timeout)*time.Second)
				defer cancel()
			}

			startTime := time.Now()
			// Execute job
			output, execErr := runCommand(jobCtx, j.Command)
			execTimeMs := time.Since(startTime).Milliseconds()

			if execErr != nil {
				m.handleFailure(j, execErr, output, execTimeMs)
			} else {
				m.logger.Info("Job completed", "job_id", j.ID)
				m.updateJobState(j.ID, job.StateCompleted, output, execTimeMs)
			}
		}
	}
}

// claimJob atomically claims a pending job.
func (m *Manager) claimJob() (*job.Job, error) {
	// SQLite specific atomic update using RETURNING
	now := time.Now().UTC()
	query := `
		UPDATE jobs 
		SET state = 'processing', updated_at = ? 
		WHERE id = (
			SELECT id FROM jobs 
			WHERE state = 'pending' AND run_after <= ? 
			ORDER BY priority DESC, created_at ASC 
			LIMIT 1
		) 
		RETURNING id, command, attempts, max_retries, output, priority, timeout, execution_time_ms
	`

	row := m.store.DB().QueryRow(query, now, now)
	var j job.Job
	err := row.Scan(&j.ID, &j.Command, &j.Attempts, &j.MaxRetries, &j.Output, &j.Priority, &j.Timeout, &j.ExecutionTimeMs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No jobs available
		}
		return nil, err
	}
	return &j, nil
}

func (m *Manager) updateJobState(id string, state job.State, output string, execTimeMs int64) {
	_, err := m.store.DB().Exec(`UPDATE jobs SET state = ?, output = ?, execution_time_ms = ?, updated_at = ? WHERE id = ?`, state, output, execTimeMs, time.Now().UTC(), id)
	if err != nil {
		m.logger.Error("Failed to update job state", "job_id", id, "error", err)
	}
}

func (m *Manager) handleFailure(j *job.Job, execErr error, output string, execTimeMs int64) {
	j.Attempts++
	if j.Attempts > j.MaxRetries {
		m.logger.Error("Job failed permanently, moving to DLQ", "job_id", j.ID, "error", execErr, "attempts", j.Attempts)
		m.updateJobState(j.ID, job.StateDead, output, execTimeMs)
		return
	}

	// Exponential backoff: base ^ attempts
	baseStr := m.cfgMgr.Get("backoff_base", "2")
	base, _ := strconv.ParseFloat(baseStr, 64)
	if base == 0 {
		base = 2.0
	}
	delaySeconds := math.Pow(base, float64(j.Attempts))
	delay := time.Duration(delaySeconds) * time.Second

	m.logger.Warn("Job failed, scheduling retry", "job_id", j.ID, "error", execErr, "attempts", j.Attempts, "delay", delay)

	query := `UPDATE jobs SET state = ?, attempts = ?, output = ?, execution_time_ms = ?, run_after = ?, updated_at = ? WHERE id = ?`
	runAfter := time.Now().UTC().Add(delay)
	_, err := m.store.DB().Exec(query, string(job.StatePending), j.Attempts, output, execTimeMs, runAfter, time.Now().UTC(), j.ID)
	if err != nil {
		m.logger.Error("Failed to update job for retry", "job_id", j.ID, "error", err)
	}
}

func runCommand(ctx context.Context, command string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
