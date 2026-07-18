# Build the executable
Write-Host "Building queuectl..."
go build -o queuectl.exe main.go

# Reset the database for a clean test
if (Test-Path "queuectl.db") { Remove-Item queuectl.db -Force }
if (Test-Path "queuectl.db-shm") { Remove-Item queuectl.db-shm -Force }
if (Test-Path "queuectl.db-wal") { Remove-Item queuectl.db-wal -Force }

# 1. Enqueue a successful job
Write-Host "Enqueuing successful job..."
./queuectl.exe enqueue "{ \""id\"": \""job-success\"", \""command\"": \""echo Test Success\"" }"

# 2. Enqueue a failing job (max_retries 1)
Write-Host "Enqueuing failing job..."
./queuectl.exe enqueue "{ \""id\"": \""job-fail\"", \""command\"": \""invalid_cmd_test\"", \""max_retries\"": 1 }"

# 3. Check Initial Status
Write-Host "Initial Status:"
./queuectl.exe status

# 4. Start Worker for a few seconds to process jobs
Write-Host "Starting workers for 5 seconds..."
$workerProcess = Start-Process -FilePath ".\queuectl.exe" -ArgumentList "worker","start","--count","2" -PassThru -NoNewWindow
Start-Sleep -Seconds 8
Stop-Process -Id $workerProcess.Id -Force

# 5. Check Final Status (job-success should be completed, job-fail should be dead since it retried fast)
Write-Host "Final Status:"
./queuectl.exe status

Write-Host "Validation script completed!"
