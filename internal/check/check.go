// Package check implements connectivity tests (ping, TCP port, HTTP) that are
// run from the monitoring machine — no SSH required.
package check

import (
	"context"
	"crypto/tls"
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

// DNSResult holds the outcome of a DNS lookup probe.
type DNSResult struct {
	Name      string
	Host      string
	Type      string
	Server    string
	Answers   []string
	OK        bool
	LatencyMs float64
	Err       error
}

// CheckDNS resolves host with the requested record type.
func CheckDNS(name, host, recordType, server, expected string, timeoutSec int) DNSResult {
	if recordType == "" {
		recordType = "A"
	}
	recordType = strings.ToUpper(recordType)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	resolver := net.DefaultResolver
	if strings.TrimSpace(server) != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				addr := server
				if _, _, err := net.SplitHostPort(addr); err != nil {
					addr = net.JoinHostPort(addr, "53")
				}
				d := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
				return d.DialContext(ctx, network, addr)
			},
		}
	}

	start := time.Now()
	answers, err := lookupDNS(ctx, resolver, host, recordType)
	latencyMs := float64(time.Since(start).Milliseconds())
	ok := err == nil && len(answers) > 0
	if ok && expected != "" {
		ok = false
		for _, answer := range answers {
			if strings.Contains(answer, expected) {
				ok = true
				break
			}
		}
	}
	return DNSResult{Name: name, Host: host, Type: recordType, Server: server, Answers: answers, OK: ok, LatencyMs: latencyMs, Err: err}
}

func lookupDNS(ctx context.Context, resolver *net.Resolver, host, recordType string) ([]string, error) {
	switch recordType {
	case "A":
		ips, err := resolver.LookupIP(ctx, "ip4", host)
		return ipsToStrings(ips), err
	case "AAAA":
		ips, err := resolver.LookupIP(ctx, "ip6", host)
		return ipsToStrings(ips), err
	case "CNAME":
		cname, err := resolver.LookupCNAME(ctx, host)
		if err != nil {
			return nil, err
		}
		return []string{strings.TrimSuffix(cname, ".")}, nil
	case "MX":
		mxs, err := resolver.LookupMX(ctx, host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			out = append(out, strings.TrimSuffix(mx.Host, "."))
		}
		return out, nil
	case "TXT":
		return resolver.LookupTXT(ctx, host)
	default:
		return nil, fmt.Errorf("unsupported DNS record type %q", recordType)
	}
}

func ipsToStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

// TracerouteResult holds the outcome of a traceroute probe.
type TracerouteResult struct {
	Name      string
	Host      string
	OK        bool
	Hops      int
	LatencyMs float64
	Output    string
	Err       error
}

// CheckTraceroute runs traceroute and counts observed hop lines.
func CheckTraceroute(name, host string, maxHops, timeoutSec int) TracerouteResult {
	if maxHops <= 0 {
		maxHops = 30
	}
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec+2)*time.Second)
	defer cancel()

	args := []string{"-m", strconv.Itoa(maxHops), "-w", strconv.Itoa(timeoutSec), host}
	start := time.Now()
	out, err := exec.CommandContext(ctx, "traceroute", args...).CombinedOutput()
	latencyMs := float64(time.Since(start).Milliseconds())
	output := strings.TrimSpace(string(out))
	hops := ParseTracerouteHops(output)
	return TracerouteResult{Name: name, Host: host, OK: err == nil && hops > 0, Hops: hops, LatencyMs: latencyMs, Output: output, Err: err}
}

// ParseTracerouteHops counts hop rows in traceroute output.
func ParseTracerouteHops(output string) int {
	count := 0
	hopRE := regexp.MustCompile(`^\s*\d+\s+`)
	for _, line := range strings.Split(output, "\n") {
		if hopRE.MatchString(line) {
			count++
		}
	}
	return count
}

// TLSResult holds the outcome of a TLS certificate probe.
type TLSResult struct {
	Name            string
	Host            string
	Port            int
	ServerName      string
	OK              bool
	LatencyMs       float64
	CertExpiresDays *float64
	Issuer          string
	Subject         string
	Err             error
}

// CheckTLS connects to host:port and reports certificate validity information.
func CheckTLS(name, host string, port int, serverName string, timeoutSec int) TLSResult {
	if port <= 0 {
		port = 443
	}
	if serverName == "" {
		serverName = host
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second},
		Config:    &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12},
	}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	latencyMs := float64(time.Since(start).Milliseconds())
	if err != nil {
		return TLSResult{Name: name, Host: host, Port: port, ServerName: serverName, OK: false, LatencyMs: latencyMs, Err: err}
	}
	defer conn.Close()

	state := conn.(*tls.Conn).ConnectionState()
	result := TLSResult{Name: name, Host: host, Port: port, ServerName: serverName, OK: len(state.PeerCertificates) > 0, LatencyMs: latencyMs}
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		days := time.Until(cert.NotAfter).Hours() / 24
		result.CertExpiresDays = &days
		result.Issuer = cert.Issuer.CommonName
		result.Subject = cert.Subject.CommonName
	}
	return result
}

// ParsePingAvg is exported for testing.
var ParsePingAvg = parsePingAvg

// ParsePingAvgFromLines is a convenience wrapper for test input.
func ParsePingAvgFromLines(lines ...string) float64 {
	return parsePingAvg(strings.Join(lines, "\n"))
}
