# QueueCTL

QueueCTL is a minimalist, single-binary job queue system built entirely with the Go Standard Library and a CGO-free embedded SQLite database. It is designed to be highly portable, reliable, and easy to use.

## Setup Instructions

To get QueueCTL running on your local machine, follow these simple steps:

1. **Prerequisites**: Make sure you have [Go installed](https://go.dev/doc/install) on your system.
2. **Open Terminal**: Navigate to the `queuectl` project folder in your terminal or command prompt.
3. **Build the Binary**: Run the following command to compile the Go code into an executable:
   ```bash
   go build -o queuectl.exe main.go
   ```
   *(Note: If you are on Linux or macOS, you can use `go build -o queuectl main.go`)*

4. **Run the Application**: You can now run the tool using the generated executable:
   ```bash
   ./queuectl.exe
   ```

## Usage Examples

QueueCTL comes with a built-in Command Line Interface (CLI). Here are some common examples of how to use it:

### 1. Enqueue a Job
You can add a new job to the queue by providing a JSON payload with an `id` and a `command`.
```bash
./queuectl.exe enqueue "{\"id\": \"job-1\", \"command\": \"echo Hello World\"}"
```
**Output:** `Successfully enqueued job job-1`

### 2. Start the Workers
To actually process the jobs in the queue, you need to start the worker processes. You can specify how many concurrent workers to run.
```bash
./queuectl.exe worker start -count=2
```

### 3. Check Queue Status
See how many jobs are currently pending, processing, completed, failed, or dead.
```bash
./queuectl.exe status
```
**Example Output:**
```
=== Job Status ===
pending      : 1
processing   : 0
completed    : 5
failed       : 0
dead         : 0
```

### 4. List Jobs
You can list all jobs or filter them by their current state (e.g., `pending`, `completed`).
```bash
./queuectl.exe list -state=pending
```

### 5. Start the HTTP Server
QueueCTL also comes with an HTTP API and dashboard!
```bash
./queuectl.exe server -port 8080
```
*(You can then visit `http://localhost:8080` in your web browser to view the dashboard)*

## Architecture Overview

QueueCTL is designed to be simple yet robust. Here is how it works under the hood:

- **Job Lifecycle**: Jobs go through different states: `pending` -> `processing` -> `completed`. If a job's command fails, it moves to `failed` and is automatically retried. If it exceeds the maximum number of retries, it is marked as `dead` (moved to the Dead Letter Queue).
- **Data Persistence**: We use an embedded **SQLite Database**. The database is configured with Write-Ahead Logging (WAL), which allows multiple workers to read and write concurrently without locking up the database. This means no data is lost even if the application crashes.
- **Worker Logic**: Workers run in an infinite loop, constantly polling the database for `pending` jobs. Once a job is found, the worker "claims" it, executes the shell command via the OS (`cmd.exe` on Windows, `sh` on Unix), captures the output, and updates the database with the result.

## Assumptions & Trade-offs

During development, a few key decisions and simplifications were made to keep the system lightweight:

- **Database as a Queue**: Instead of using a heavy message broker (like RabbitMQ or Kafka), we use SQLite for job locking and state management. This makes the system incredibly simple to deploy (just one file!) but trades off sub-millisecond pub/sub latency for a polling mechanism.
- **Atomic Locking**: We rely on SQLite's transactional guarantees to ensure no two workers claim the same job. We use an `UPDATE ... RETURNING` query to handle this gracefully without needing complex distributed locks.
- **No External Dependencies**: By relying strictly on the Go standard library and a CGO-free SQLite driver, the binary is 100% portable across different operating systems without needing external C compilers.

## Testing Instructions

To quickly verify that everything is working as expected, a PowerShell validation script is included in the project.

1. Open PowerShell and navigate to the project directory.
2. Run the validation script:
   ```powershell
   .\validate.ps1
   ```
3. **What the script does**:
   - Compiles the Go code into `queuectl.exe`.
   - Cleans up any old database files for a fresh test.
   - Enqueues a successful job (a simple `echo` command) and a failing job (an invalid command).
   - Prints the initial queue status.
   - Starts the background workers for 5 seconds to process the queued jobs.
   - Prints the final queue status to verify that the jobs successfully moved to the `completed` and `dead` states.
   
## DEMO VIDEO LINK
https://drive.google.com/file/d/1GF39AHpdmaLVYpX0cZLubrXonwkn_e8h/view?usp=sharing
