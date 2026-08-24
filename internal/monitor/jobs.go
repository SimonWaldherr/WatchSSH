package monitor

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	sshclient "github.com/SimonWaldherr/WatchSSH/internal/ssh"
)

const jobOutputLimit = 8 * 1024

var scheduledJobParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// JobManager runs enabled local jobs at most once for each due schedule. It
// keeps no durable state: after a WatchSSH restart, only jobs with
// run_on_start: true run immediately; the next normal cron occurrence handles
// all other jobs.
type JobManager struct {
	mu      sync.Mutex
	lastRun map[string]time.Time
	running map[string]bool
	results []JobResult
	notify  func([]JobResult)
	stopped bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewJobManager() *JobManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &JobManager{
		lastRun: make(map[string]time.Time),
		running: make(map[string]bool),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (m *Monitor) runJobLoop() {
	m.runDueJobs()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.runDueJobs()
		case <-m.done:
			return
		}
	}
}

func (m *Monitor) runDueJobs() {
	if m.jobMgr == nil {
		return
	}
	m.cfgMu.RLock()
	cfg := m.cfg
	m.cfgMu.RUnlock()
	m.jobMgr.RunDue(cfg)
}

// RunDue starts jobs whose next cron occurrence is due. Running jobs are
// never overlapped; the next occurrence is calculated from completion time so
// a slow import cannot trigger an immediate catch-up run.
func (jm *JobManager) RunDue(cfg *config.Config) {
	if cfg == nil {
		return
	}
	now := time.Now()
	for _, job := range cfg.Jobs {
		if !job.Enabled {
			continue
		}
		schedule, err := scheduledJobParser.Parse(job.Schedule)
		if err != nil {
			// Configuration validation rejects this before the monitor starts.
			log.Printf("job %q ignored: invalid schedule: %v", job.Name, err)
			continue
		}
		if !jm.startDue(job, schedule, now) {
			continue
		}
		go func(job config.ScheduledJobConfig) {
			defer jm.wg.Done()
			defer jm.finished(job.Name, time.Now())
			jm.record(jm.run(cfg, job))
		}(job)
	}
}

func (jm *JobManager) startDue(job config.ScheduledJobConfig, schedule cron.Schedule, now time.Time) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if jm.stopped || jm.running[job.Name] {
		return false
	}
	lastRun, hasRun := jm.lastRun[job.Name]
	if !hasRun {
		if !job.RunOnStart {
			jm.lastRun[job.Name] = now
			return false
		}
	} else if schedule.Next(lastRun).After(now) {
		return false
	}
	jm.running[job.Name] = true
	jm.wg.Add(1)
	return true
}

func (jm *JobManager) finished(name string, finishedAt time.Time) {
	jm.mu.Lock()
	delete(jm.running, name)
	jm.lastRun[name] = finishedAt
	jm.mu.Unlock()
}

func (jm *JobManager) Stop() {
	if jm == nil {
		return
	}
	jm.cancel()
	jm.mu.Lock()
	jm.stopped = true
	jm.mu.Unlock()
	jm.wg.Wait()
}

// SetNotify receives a bounded, newest-first copy of completed job results.
// It is intended for the web state and must not perform blocking job work.
func (jm *JobManager) SetNotify(notify func([]JobResult)) {
	jm.mu.Lock()
	jm.notify = notify
	jm.mu.Unlock()
}

func (jm *JobManager) Recent() []JobResult {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	return newestJobResults(jm.results)
}

func (jm *JobManager) run(cfg *config.Config, job config.ScheduledJobConfig) JobResult {
	startedAt := time.Now()
	result := JobResult{Name: job.Name, StartedAt: startedAt, Status: "failed"}
	ctx, cancel := context.WithTimeout(jm.ctx, time.Duration(job.Timeout)*time.Second)
	defer cancel()

	if job.Command != "" {
		if _, err := runScheduledCommand(ctx, job); err != nil {
			result.Error = err.Error()
			return finishJobResult(result)
		}
	}

	servers := make(map[string]config.Server, len(cfg.Servers))
	for _, server := range cfg.Servers {
		servers[server.Name] = server
	}
	var uploaded int64
	for _, upload := range job.Uploads {
		bytesWritten, err := uploadJobArtifact(ctx, cfg, servers, upload)
		uploadResult := JobUploadResult{Server: upload.Server, Source: upload.Source, Destination: upload.Destination, Bytes: bytesWritten}
		if err != nil {
			uploadResult.Error = err.Error()
			result.Uploads = append(result.Uploads, uploadResult)
			result.Error = err.Error()
			return finishJobResult(result)
		}
		result.Uploads = append(result.Uploads, uploadResult)
		uploaded += bytesWritten
	}
	result.Status = "succeeded"
	result = finishJobResult(result)
	log.Printf("job %q succeeded in %s (%d artifact(s), %d bytes uploaded)", job.Name, result.FinishedAt.Sub(startedAt).Round(time.Millisecond), len(job.Uploads), uploaded)
	return result
}

func finishJobResult(result JobResult) JobResult {
	result.FinishedAt = time.Now()
	result.DurationMs = float64(result.FinishedAt.Sub(result.StartedAt).Microseconds()) / 1000
	if result.Error != "" && result.Status != "cancelled" && result.Error == context.Canceled.Error() {
		result.Status = "cancelled"
	}
	return result
}

func (jm *JobManager) record(result JobResult) {
	jm.mu.Lock()
	jm.results = append(jm.results, result)
	if len(jm.results) > 100 {
		jm.results = jm.results[len(jm.results)-100:]
	}
	notify := jm.notify
	results := newestJobResults(jm.results)
	jm.mu.Unlock()
	if result.Status == "failed" {
		log.Printf("job %q failed after %s: %s", result.Name, result.FinishedAt.Sub(result.StartedAt).Round(time.Millisecond), result.Error)
	}
	if notify != nil {
		notify(results)
	}
}

func newestJobResults(results []JobResult) []JobResult {
	copyResults := append([]JobResult(nil), results...)
	for left, right := 0, len(copyResults)-1; left < right; left, right = left+1, right-1 {
		copyResults[left], copyResults[right] = copyResults[right], copyResults[left]
	}
	return copyResults
}

func runScheduledCommand(ctx context.Context, job config.ScheduledJobConfig) (string, error) {
	command := exec.CommandContext(ctx, "sh", "-c", job.Command)
	command.Dir = job.WorkingDirectory
	var output cappedJobOutput
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if ctx.Err() != nil {
		return output.String(), ctx.Err()
	}
	if err != nil {
		return output.String(), fmt.Errorf("local command exited unsuccessfully: %w", err)
	}
	return output.String(), nil
}

func uploadJobArtifact(ctx context.Context, cfg *config.Config, servers map[string]config.Server, upload config.JobUploadConfig) (int64, error) {
	target, exists := servers[upload.Server]
	if !exists || target.Local {
		return 0, fmt.Errorf("upload target %q is not an SSH server", upload.Server)
	}
	file, err := os.Open(upload.Source)
	if err != nil {
		return 0, fmt.Errorf("opening upload source %s: %w", upload.Source, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stating upload source %s: %w", upload.Source, err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("upload source %s is not a regular file", upload.Source)
	}
	connectTimeout := time.Duration(cfg.Timeout) * time.Second
	client, err := sshclient.New(ctx, target, cfg, connectTimeout)
	if err != nil {
		return 0, fmt.Errorf("connecting to upload target %q: %w", upload.Server, err)
	}
	defer client.Close()
	bytesWritten, err := client.Upload(ctx, file, upload.Destination, upload.CreateDirectories)
	if err != nil {
		return bytesWritten, fmt.Errorf("uploading %s to %s:%s: %w", upload.Source, upload.Server, upload.Destination, err)
	}
	return bytesWritten, nil
}

type cappedJobOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (o *cappedJobOutput) Write(data []byte) (int, error) {
	remaining := jobOutputLimit - o.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			_, _ = o.buffer.Write(data[:remaining])
			o.truncated = true
		} else {
			_, _ = o.buffer.Write(data)
		}
	} else {
		o.truncated = true
	}
	return len(data), nil
}

func (o *cappedJobOutput) String() string {
	if o.truncated {
		return o.buffer.String() + "\n[output truncated]"
	}
	return o.buffer.String()
}
