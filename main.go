package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"queuectl/internal/config"
	"queuectl/internal/job"
	"queuectl/internal/queue"
	"queuectl/internal/storage"
	"queuectl/internal/worker"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: queuectl <command> [args]")
		os.Exit(1)
	}

	dbPath := filepath.Join(".", "queuectl.db")
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize storage: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	qMgr := queue.NewManager(store)
	cfgMgr := config.NewManager(store)

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "enqueue":
		if len(args) < 1 {
			fmt.Println("Usage: queuectl enqueue [command JSON]")
			os.Exit(1)
		}
		var j job.Job
		if err := json.Unmarshal([]byte(args[0]), &j); err != nil {
			fmt.Printf("invalid JSON format: %v\n", err)
			os.Exit(1)
		}
		if j.ID == "" || j.Command == "" {
			fmt.Println("job must contain 'id' and 'command'")
			os.Exit(1)
		}
		if j.MaxRetries == 0 {
			maxRetriesStr := cfgMgr.Get("max_retries", "3")
			maxR, _ := strconv.Atoi(maxRetriesStr)
			if maxR == 0 {
				maxR = 3
			}
			j.MaxRetries = maxR
		}
		if err := qMgr.Enqueue(&j); err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully enqueued job %s\n", j.ID)

	case "worker":
		if len(args) < 1 {
			fmt.Println("Usage: queuectl worker [start|stop]")
			os.Exit(1)
		}
		subCmd := args[0]
		if subCmd == "start" {
			fs := flag.NewFlagSet("worker start", flag.ExitOnError)
			count := fs.Int("count", 1, "Number of worker processes to start")
			fs.Parse(args[1:])
			wMgr := worker.NewManager(store, cfgMgr)
			wMgr.Start(*count)
		} else if subCmd == "stop" {
			fmt.Println("To stop workers gracefully, send SIGINT (Ctrl+C) to the process running 'queuectl worker start'.")
		} else {
			fmt.Println("Unknown worker command:", subCmd)
		}

	case "status":
		status, err := qMgr.Status()
		if err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("=== Job Status ===")
		states := []string{"pending", "processing", "completed", "failed", "dead"}
		for _, state := range states {
			fmt.Printf("%-12s : %d\n", state, status[state])
		}

	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		stateFilter := fs.String("state", "", "Filter jobs by state")
		fs.Parse(args)
		jobs, err := qMgr.List(*stateFilter)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
		if len(jobs) == 0 {
			fmt.Println("No jobs found.")
			return
		}
		fmt.Printf("%-20s %-15s %-10s %s\n", "ID", "STATE", "ATTEMPTS", "COMMAND")
		for _, j := range jobs {
			fmt.Printf("%-20s %-15s %-10d %s\n", j.ID, string(j.State), j.Attempts, j.Command)
		}

	case "dlq":
		if len(args) < 1 {
			fmt.Println("Usage: queuectl dlq [list|retry]")
			os.Exit(1)
		}
		subCmd := args[0]
		if subCmd == "list" {
			jobs, err := qMgr.List(string(job.StateDead))
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			if len(jobs) == 0 {
				fmt.Println("DLQ is empty.")
				return
			}
			fmt.Printf("%-20s %-15s %-10s %s\n", "ID", "STATE", "ATTEMPTS", "COMMAND")
			for _, j := range jobs {
				fmt.Printf("%-20s %-15s %-10d %s\n", j.ID, string(j.State), j.Attempts, j.Command)
			}
		} else if subCmd == "retry" {
			if len(args) < 2 {
				fmt.Println("Usage: queuectl dlq retry [job-id]")
				os.Exit(1)
			}
			jobID := args[1]
			if err := qMgr.RetryDeadJob(jobID); err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully moved job %s back to pending queue.\n", jobID)
		}

	case "config":
		if len(args) < 1 {
			fmt.Println("Usage: queuectl config [get|set]")
			os.Exit(1)
		}
		subCmd := args[0]
		if subCmd == "get" {
			if len(args) < 2 {
				fmt.Println("Usage: queuectl config get [key]")
				os.Exit(1)
			}
			val := cfgMgr.Get(args[1], "")
			if val == "" {
				fmt.Printf("Config key '%s' not found or empty.\n", args[1])
			} else {
				fmt.Println(val)
			}
		} else if subCmd == "set" {
			if len(args) < 3 {
				fmt.Println("Usage: queuectl config set [key] [value]")
				os.Exit(1)
			}
			if err := cfgMgr.Set(args[1], args[2]); err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully set %s = %s\n", args[1], args[2])
		}

	case "server":
		fs := flag.NewFlagSet("server", flag.ExitOnError)
		port := fs.Int("port", 8080, "Port to run the HTTP server on")
		fs.Parse(args)

		http.HandleFunc("/api/enqueue", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var j job.Job
			if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
				http.Error(w, "invalid JSON format", http.StatusBadRequest)
				return
			}
			if j.ID == "" || j.Command == "" {
				http.Error(w, "job must contain 'id' and 'command'", http.StatusBadRequest)
				return
			}
			if j.MaxRetries == 0 {
				maxRetriesStr := cfgMgr.Get("max_retries", "3")
				maxR, _ := strconv.Atoi(maxRetriesStr)
				if maxR == 0 {
					maxR = 3
				}
				j.MaxRetries = maxR
			}
			if err := qMgr.Enqueue(&j); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, "Successfully enqueued job %s\n", j.ID)
		})

		http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
			status, err := qMgr.Status()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(status)
		})

		http.HandleFunc("/api/list", func(w http.ResponseWriter, r *http.Request) {
			stateFilter := r.URL.Query().Get("state")
			jobs, err := qMgr.List(stateFilter)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jobs)
		})

		http.HandleFunc("/api/dlq", func(w http.ResponseWriter, r *http.Request) {
			jobs, err := qMgr.List(string(job.StateDead))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jobs)
		})

		// Serve static dashboard
		fsHandler := http.FileServer(http.Dir("./public"))
		http.Handle("/", fsHandler)

		addr := fmt.Sprintf(":%d", *port)
		fmt.Printf("Starting HTTP server on %s...\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
