package platform

import (
	"context"
	"strings"
)

type windowsCollector struct{}

func (c *windowsCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
	s := &Snapshot{}

	hostOut, _ := r.Run(ctx, "hostname")
	verOut, _ := r.Run(ctx, "cmd /c ver")
	s.SystemInfo = SystemInfo{
		OS:       string(Windows),
		Kernel:   strings.TrimSpace(verOut),
		Hostname: strings.TrimSpace(hostOut),
	}

	for _, metric := range []string{
		"uptime", "load", "cpu", "memory", "swap", "disks", "disk_inodes",
		"network", "processes", "file_descriptors", "cpu_cores", "inodes",
		"logged_users",
	} {
		s.setUnsupported(metric)
	}
	return s, nil
}
