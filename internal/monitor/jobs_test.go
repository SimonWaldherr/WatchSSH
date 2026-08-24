package monitor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/schedule"
)

func TestRunScheduledCommandUsesConfiguredWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	output, err := runScheduledCommand(context.Background(), config.ScheduledJobConfig{
		Command:          "printf generated > artifact.txt; pwd",
		WorkingDirectory: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output == "" {
		t.Fatal("expected command output")
	}
	data, err := os.ReadFile(filepath.Join(dir, "artifact.txt"))
	if err != nil || string(data) != "generated" {
		t.Fatalf("artifact = %q, %v", data, err)
	}
}

func TestRunScheduledCommandHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := runScheduledCommand(ctx, config.ScheduledJobConfig{Command: "sleep 1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestJobManagerStartsRunOnStartOnlyOnce(t *testing.T) {
	manager := NewJobManager()
	defer manager.Stop()
	definition, err := schedule.Parse("@yearly")
	if err != nil {
		t.Fatal(err)
	}
	job := config.ScheduledJobConfig{Name: "osm", RunOnStart: true}
	now := time.Now()
	if !manager.startDue(job, definition, now) {
		t.Fatal("run_on_start job should start")
	}
	if manager.startDue(job, definition, now) {
		t.Fatal("running job must not overlap")
	}
	manager.finished(job.Name, now)
	manager.wg.Done()
	if manager.startDue(job, definition, now.Add(time.Minute)) {
		t.Fatal("job should wait for its next cron occurrence")
	}
}

func TestUploadJobArtifactRejectsLocalTarget(t *testing.T) {
	_, err := uploadJobArtifact(context.Background(), &config.Config{Timeout: 1}, map[string]config.Server{
		"local": {Name: "local", Local: true},
	}, config.JobUploadConfig{Server: "local", Source: "/tmp/anything", Destination: "/srv/anything"})
	if err == nil {
		t.Fatal("expected local upload target to be rejected")
	}
}
