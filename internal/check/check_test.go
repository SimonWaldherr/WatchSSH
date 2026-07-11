package check

import (
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestParsePingLoss(t *testing.T) {
	if got := parsePingLoss("3 packets transmitted, 2 received, 33.3% packet loss"); got != 33.3 {
		t.Fatalf("parsePingLoss = %v, want 33.3", got)
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

func TestCheckBanner(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("SSH-2.0-WatchSSH-Test\r\n"))
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	result := CheckBanner("ssh", "127.0.0.1", port, "SSH-", 1)
	if !result.OK || result.Banner != "SSH-2.0-WatchSSH-Test" {
		t.Fatalf("banner result = %#v", result)
	}
}

func TestCheckHTTP(t *testing.T) {
	// Unreachable URL should return OK=false.
	r := CheckHTTP("http://127.0.0.1:19999/nonexistent", 200, 1)
	if r.OK {
		t.Error("expected HTTP check to fail for unreachable URL")
	}
}

func TestCheckHTTPWithOptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %q, want HEAD", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	r := CheckHTTPWithOptions(ts.URL, http.MethodHead, http.StatusNoContent, "", 1)
	if !r.OK || r.Method != http.MethodHead || r.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected HTTP result: %+v", r)
	}
}

func TestCheckHTTPWithExpectedBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer ts.Close()

	if r := CheckHTTPWithOptions(ts.URL, http.MethodGet, http.StatusOK, `"status":"ready"`, 1); !r.OK {
		t.Fatalf("expected matching body result to succeed: %+v", r)
	}
	if r := CheckHTTPWithOptions(ts.URL, http.MethodGet, http.StatusOK, "missing", 1); r.OK {
		t.Fatalf("expected mismatching body result to fail: %+v", r)
	}
}

func TestCheckNTP(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		request := make([]byte, 48)
		n, addr, readErr := conn.ReadFromUDP(request)
		if readErr != nil || n < 48 {
			return
		}
		response := make([]byte, 48)
		response[0] = 0x24 // version 4, server mode
		response[1] = 2
		now := time.Now().Add(2 * time.Millisecond)
		binary.BigEndian.PutUint32(response[40:44], uint32(now.Unix()+ntpEpochOffset))
		binary.BigEndian.PutUint32(response[44:48], uint32((uint64(now.Nanosecond())<<32)/1e9))
		_, _ = conn.WriteToUDP(response, addr)
	}()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	r := CheckNTP("local-time", "127.0.0.1", port, 100, 1)
	if !r.OK || r.Stratum != 2 || r.LatencyMs < 0 {
		t.Fatalf("unexpected NTP result: %+v", r)
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

func TestParseTracerouteHops(t *testing.T) {
	got := ParseTracerouteHops(`traceroute to example.com (93.184.216.34), 30 hops max
 1  router.local (192.0.2.1)  1.234 ms
 2  198.51.100.1  8.123 ms
 3  * * *
 4  example.com (93.184.216.34)  20.123 ms`)
	if got != 4 {
		t.Fatalf("ParseTracerouteHops() = %d, want 4", got)
	}
}
