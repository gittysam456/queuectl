package tests

import (
	"context"
	"os"
	"path/filepath"
	"queuectl/internal/config"
	"queuectl/internal/job"
	"queuectl/internal/queue"
	"queuectl/internal/storage"
	"queuectl/internal/worker"
	"testing"
	"time"
)

func setupTestEnv(t *testing.T) (*storage.Storage, *queue.Manager, *config.Manager, func()) {
	tmpDir, err := os.MkdirTemp("", "queuectl_test")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	cfgMgr := config.NewManager(store)
	qMgr := queue.NewManager(store)

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, qMgr, cfgMgr, cleanup
}

func TestJobCompletion(t *testing.T) {
	store, qMgr, cfgMgr, cleanup := setupTestEnv(t)
	defer cleanup()

	j := &job.Job{
		ID:         "test-1",
		Command:    "go version", // universally valid
		MaxRetries: 3,
	}
	if err := qMgr.Enqueue(j); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	wMgr := worker.NewManager(store, cfgMgr)

	// Timeout allows worker enough time to poll and execute before gracefully exiting
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	wMgr.StartWithContext(ctx, 1)

	jobs, err := qMgr.List("")
	if err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(jobs))
	}

	if jobs[0].State != job.StateCompleted {
		t.Fatalf("Expected job to be completed, got %s", jobs[0].State)
	}
}

func TestJobRetryAndDLQ(t *testing.T) {
	store, qMgr, cfgMgr, cleanup := setupTestEnv(t)
	defer cleanup()

	// Speed up backoff for test
	cfgMgr.Set("backoff_base", "1")

	j := &job.Job{
		ID:         "test-2",
		Command:    "invalid_command_should_fail",
		MaxRetries: 1, // Only 1 retry -> fails twice -> DLQ
	}
	qMgr.Enqueue(j)

	wMgr := worker.NewManager(store, cfgMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wMgr.StartWithContext(ctx, 1)

	jobs, _ := qMgr.List(string(job.StateDead))
	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job in DLQ, got %d", len(jobs))
	}

	if jobs[0].ID != "test-2" {
		t.Fatalf("Expected job ID test-2, got %s", jobs[0].ID)
	}
}
