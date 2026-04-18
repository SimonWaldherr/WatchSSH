// Package check implements connectivity tests (ping, TCP port, HTTP) that are
// run from the monitoring machine — no SSH required.
package check

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PingResult holds the outcome of a ping check.
type PingResult struct {
	OK        bool
	LatencyMs float64
	Err       error
}

// Ping sends count ICMP echo requests to host and returns the average round-trip
// time. The underlying ping(8) binary must be available on the monitoring host.
func Ping(host string, count, timeoutSec int) PingResult {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(timeoutSec+2)*time.Second,
	)
	defer cancel()

	args := []string{
		"-c", strconv.Itoa(count),
		"-W", strconv.Itoa(timeoutSec),
		host,
	}
	out, err := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
	if err != nil {
		return PingResult{OK: false, Err: fmt.Errorf("ping %s: %w", host, err)}
	}
	return PingResult{OK: true, LatencyMs: parsePingAvg(string(out))}
}

// parsePingAvg extracts the average round-trip time from ping output.
// Supports Linux (rtt min/avg/max/mdev) and macOS (round-trip min/avg/max/stddev).
var (
	linuxPingRE = regexp.MustCompile(`rtt min/avg/max/mdev\s*=\s*[\d.]+/([\d.]+)/`)
	bsdPingRE   = regexp.MustCompile(`round-trip min/avg/max/(?:std-?dev|stddev)\s*=\s*[\d.]+/([\d.]+)/`)
)

func parsePingAvg(output string) float64 {
	if m := linuxPingRE.FindStringSubmatch(output); len(m) >= 2 {
		v, _ := strconv.ParseFloat(m[1], 64)
		return v
	}
	if m := bsdPingRE.FindStringSubmatch(output); len(m) >= 2 {
		v, _ := strconv.ParseFloat(m[1], 64)
		return v
	}
	// Fallback: parse individual "time=X ms" lines and compute the average.
	timeRE := regexp.MustCompile(`time=([\d.]+)\s*ms`)
	matches := timeRE.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0
	}
	var sum float64
	for _, m := range matches {
		v, _ := strconv.ParseFloat(m[1], 64)
		sum += v
	}
	return sum / float64(len(matches))
}

// PortResult holds the outcome of a TCP port check.
type PortResult struct {
	Port int
	Open bool
}

// CheckPort tests whether host:port accepts TCP connections within timeoutSec.
func CheckPort(host string, port, timeoutSec int) PortResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeoutSec)*time.Second)
	if err != nil {
		return PortResult{Port: port, Open: false}
	}
	_ = conn.Close()
	return PortResult{Port: port, Open: true}
}

// HTTPResult holds the outcome of an HTTP health check.
type HTTPResult struct {
	URL             string
	StatusCode      int
	OK              bool
	LatencyMs       float64
	CertExpiresDays *float64
}

// CheckHTTP sends a GET request to url and checks the response status.
// Redirects are not followed so that the direct response status is observed.
// A context with the given timeout governs the entire request.
func CheckHTTP(url string, expectedStatus, timeoutSec int) HTTPResult {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HTTPResult{URL: url, OK: false}
	}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMs := float64(time.Since(start).Milliseconds())
	if err != nil {
		return HTTPResult{URL: url, OK: false, LatencyMs: latencyMs}
	}
	defer resp.Body.Close()
	var certDays *float64
	if shouldCheckTLSCert(url, req.URL.Scheme) && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		days := time.Until(resp.TLS.PeerCertificates[0].NotAfter).Hours() / 24
		certDays = &days
	}
	return HTTPResult{
		URL:             url,
		StatusCode:      resp.StatusCode,
		OK:              resp.StatusCode == expectedStatus,
		LatencyMs:       latencyMs,
		CertExpiresDays: certDays,
	}
}

func shouldCheckTLSCert(rawURL, reqScheme string) bool {
	if strings.EqualFold(reqScheme, "https") {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https")
}

// ParsePingAvg is exported for testing.
var ParsePingAvg = parsePingAvg

// ParsePingAvgFromLines is a convenience wrapper for test input.
func ParsePingAvgFromLines(lines ...string) float64 {
	return parsePingAvg(strings.Join(lines, "\n"))
}
