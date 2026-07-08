// Package history provides optional persistence for metric and alert history.
package history

import (
	"context"
	"fmt"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// MetricRecord is the storage-friendly representation of one metrics sample.
type MetricRecord struct {
	ID             string
	CollectedAt    string
	ServerName     string
	Host           string
	Platform       string
	HasError       bool
	CPUUsage       *float64
	MemoryUsage    *float64
	SwapUsage      *float64
	Load1          *float64
	DiskRootUsage  *float64
	PingOK         *bool
	PingLatencyMS  *float64
	DNSOK          *bool
	TLSCertMinDays *float64
	TracerouteHops *float64
	PayloadJSON    string
}

// FiringRecord is the storage-friendly representation of one alert firing.
type FiringRecord struct {
	ID          string
	FiredAt     string
	RuleName    string
	Metric      string
	Server      string
	Value       float64
	Message     string
	PayloadJSON string
}

// Store persists metric and alert history.
type Store interface {
	RecordMetrics(ctx context.Context, records []MetricRecord) error
	RecordFirings(ctx context.Context, records []FiringRecord) error
	RecentMetrics(ctx context.Context, serverName string, limit int) ([]MetricRecord, error)
	RecentFirings(ctx context.Context, limit int) ([]FiringRecord, error)
	Close() error
}

type noopStore struct{}

// New opens the configured history store. The default is a no-op store.
func New(cfg config.StorageConfig) (Store, error) {
	switch cfg.Type {
	case "", "none":
		return noopStore{}, nil
	case "tinysql":
		return OpenTinySQLWithConfig(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type %q", cfg.Type)
	}
}

func (noopStore) RecordMetrics(context.Context, []MetricRecord) error {
	return nil
}

func (noopStore) RecordFirings(context.Context, []FiringRecord) error {
	return nil
}

func (noopStore) RecentMetrics(context.Context, string, int) ([]MetricRecord, error) {
	return nil, nil
}

func (noopStore) RecentFirings(context.Context, int) ([]FiringRecord, error) {
	return nil, nil
}

func (noopStore) Close() error {
	return nil
}
