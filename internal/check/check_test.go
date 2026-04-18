package check

import (
	"testing"
)

func TestParsePingAvgLinux(t *testing.T) {
	output := `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=118 time=12.3 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=118 time=11.8 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=118 time=12.1 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2002ms
rtt min/avg/max/mdev = 11.800/12.067/12.300/0.208 ms`

	got := parsePingAvg(output)
	if got < 11 || got > 13 {
		t.Errorf("parsePingAvg = %.3f; want ~12.067", got)
	}
}

func TestParsePingAvgBSD(t *testing.T) {
	output := `PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: icmp_seq=0 ttl=118 time=5.432 ms

--- 8.8.8.8 ping statistics ---
1 packets transmitted, 1 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 5.432/5.432/5.432/0.000 ms`

	got := parsePingAvg(output)
	if got < 5 || got > 6 {
		t.Errorf("parsePingAvg BSD = %.3f; want ~5.432", got)
	}
}

func TestParsePingAvgFallback(t *testing.T) {
	// Only individual time= lines, no summary line.
	output := `PING host: 56 data bytes
64 bytes from host: time=20.1 ms
64 bytes from host: time=19.9 ms`

	got := parsePingAvg(output)
	want := (20.1 + 19.9) / 2
	if got < want-0.1 || got > want+0.1 {
		t.Errorf("parsePingAvg fallback = %.3f; want %.3f", got, want)
	}
}

func TestCheckPort(t *testing.T) {
	// Port 1 on localhost should always be closed.
	r := CheckPort("127.0.0.1", 1, 1)
	if r.Open {
		t.Error("expected port 1 to be closed")
	}
	if r.Port != 1 {
		t.Errorf("expected Port=1, got %d", r.Port)
	}
}

func TestCheckHTTP(t *testing.T) {
	// Unreachable URL should return OK=false.
	r := CheckHTTP("http://127.0.0.1:19999/nonexistent", 200, 1)
	if r.OK {
		t.Error("expected HTTP check to fail for unreachable URL")
	}
}

func TestShouldCheckTLSCert(t *testing.T) {
	if !shouldCheckTLSCert("https://example.com/health", "https") {
		t.Fatal("expected https URL to require TLS cert check")
	}
	if shouldCheckTLSCert("http://example.com/health", "http") {
		t.Fatal("expected http URL not to require TLS cert check")
	}
}
