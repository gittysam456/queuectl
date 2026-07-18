package job

import "time"

// State represents the current state of a job in the queue.
type State string

const (
	StatePending    State = "pending"
	StateProcessing State = "processing"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
	StateDead       State = "dead"
)

// Job represents a background job.
type Job struct {
	ID              string    `json:"id"`
	Command         string    `json:"command"`
	State           State     `json:"state"`
	Attempts        int       `json:"attempts"`
	MaxRetries      int       `json:"max_retries"`
	Priority        int       `json:"priority"`
	Timeout         int       `json:"timeout"`
	Output          string    `json:"output"`
	ExecutionTimeMs int64     `json:"execution_time_ms"`
	RunAfter        time.Time `json:"run_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
