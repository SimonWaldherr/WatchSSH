package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

type fakeTargetPortDialer struct {
	host string
	port int
	err  error
}

func (d *fakeTargetPortDialer) DialTCP(_ context.Context, host string, port int) (time.Duration, error) {
	d.host = host
	d.port = port
	return 17 * time.Millisecond, d.err
}

func TestRunTargetPortChecksUsesDirectTCP(t *testing.T) {
	dialer := &fakeTargetPortDialer{}
	srv := config.Server{Checks: config.Checks{Ports: []config.PortCheck{
		{Host: "db.internal", Port: 5432, Source: "target", Timeout: 1},
		{Host: "app.example.test", Port: 443, Source: "monitor", Timeout: 1},
	}}}

	results := runTargetPortChecks(context.Background(), dialer, srv)
	if dialer.host != "db.internal" || dialer.port != 5432 {
		t.Fatalf("DialTCP target = %s:%d", dialer.host, dialer.port)
	}
	if len(results) != 1 || !results[0].Open || results[0].Source != "target" || results[0].LatencyMs != 17 {
		t.Fatalf("remote port results = %#v", results)
	}
}

func TestRunTargetPortChecksCapturesFailure(t *testing.T) {
	dialer := &fakeTargetPortDialer{err: errors.New("administratively prohibited")}
	srv := config.Server{Checks: config.Checks{Ports: []config.PortCheck{{Host: "db.internal", Port: 5432, Source: "target", Timeout: 1}}}}

	results := runTargetPortChecks(context.Background(), dialer, srv)
	if len(results) != 1 || results[0].Open || results[0].Error == "" {
		t.Fatalf("failed remote port result = %#v", results)
	}
}
