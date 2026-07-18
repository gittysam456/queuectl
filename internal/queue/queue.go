package queue

import (
	"fmt"
	"queuectl/internal/job"
	"queuectl/internal/storage"
	"time"
)

// Manager handles queue operations.
type Manager struct {
	store *storage.Storage
}

// NewManager creates a new queue Manager.
func NewManager(store *storage.Storage) *Manager {
	return &Manager{store: store}
}

// Enqueue adds a new job to the queue.
func (m *Manager) Enqueue(j *job.Job) error {
	query := `
		INSERT INTO jobs (id, command, state, attempts, max_retries, priority, timeout, output, execution_time_ms, run_after, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now().UTC()
	if j.RunAfter.IsZero() {
		j.RunAfter = now
	}
	j.CreatedAt = now
	j.UpdatedAt = now
	j.State = job.StatePending

	_, err := m.store.DB().Exec(query, j.ID, j.Command, j.State, j.Attempts, j.MaxRetries, j.Priority, j.Timeout, j.Output, j.ExecutionTimeMs, j.RunAfter, j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}
	return nil
}

// List returns jobs, optionally filtered by state.
func (m *Manager) List(stateFilter string) ([]*job.Job, error) {
	query := `SELECT id, command, state, attempts, max_retries, priority, timeout, output, execution_time_ms, run_after, created_at, updated_at FROM jobs`
	var args []interface{}
	if stateFilter != "" {
		query += ` WHERE state = ?`
		args = append(args, stateFilter)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := m.store.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*job.Job
	for rows.Next() {
		j := &job.Job{}
		if err := rows.Scan(&j.ID, &j.Command, &j.State, &j.Attempts, &j.MaxRetries, &j.Priority, &j.Timeout, &j.Output, &j.ExecutionTimeMs, &j.RunAfter, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Status returns a count of jobs in each state.
func (m *Manager) Status() (map[string]int, error) {
	query := `SELECT state, COUNT(*) FROM jobs GROUP BY state`
	rows, err := m.store.DB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}
	defer rows.Close()

	status := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		status[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return status, nil
}

// RetryDeadJob resets a dead job's attempts to 0 and moves it back to pending.
func (m *Manager) RetryDeadJob(jobID string) error {
	query := `
		UPDATE jobs 
		SET state = ?, attempts = 0, run_after = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ? AND state = ?
	`
	res, err := m.store.DB().Exec(query, string(job.StatePending), jobID, string(job.StateDead))
	if err != nil {
		return fmt.Errorf("failed to retry dlq job: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("job %s not found or not in dead state", jobID)
	}

	return nil
}
