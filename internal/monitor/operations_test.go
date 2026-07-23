package monitor

import (
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

func TestSuppressDependentFirings(t *testing.T) {
	cfg := config.Config{Servers: []config.Server{{Name: "router"}, {Name: "app", DependsOn: []string{"router"}}}}
	firings := []Firing{{Server: "router", RuleName: "ping"}, {Server: "app", RuleName: "http"}}
	metrics := []ServerMetrics{{ServerName: "router", Error: "connection refused"}, {ServerName: "app"}}
	active := SuppressDependentFirings(firings, metrics, cfg)
	if len(active) != 1 || active[0].Server != "router" {
		t.Fatalf("active firings = %#v", active)
	}
}

func TestBuildSecurityFindings(t *testing.T) {
	cfg := config.Config{Servers: []config.Server{{Name: "legacy", InsecureIgnoreHostKey: true, Auth: config.Auth{Type: config.AuthTypePassword}}}}
	findings := BuildSecurityFindings(cfg, []ServerMetrics{{ServerName: "legacy", Connectivity: ConnectivityStats{TLS: []TLSResult{{OK: false, Error: "expired"}}}}})
	if len(findings) != 3 {
		t.Fatalf("findings = %#v", findings)
	}
}
