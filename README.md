# WatchSSH

[![CI](https://github.com/SimonWaldherr/WatchSSH/actions/workflows/ci.yml/badge.svg)](https://github.com/SimonWaldherr/WatchSSH/actions/workflows/ci.yml)

**Agentless monitoring of Unix-like servers over SSH.**

WatchSSH connects to remote hosts via SSH, runs only standard system tools
that are already present on the target, collects metrics, and reports them
to the console or as JSON. No agent installation required on the monitored
hosts — only an SSH user with read-only access to system utilities.

When no servers are configured, WatchSSH automatically falls back to a built-in
`localhost` diagnostic profile so it is immediately useful on a single machine
and can surface local Docker containers when available.

## Supported Platforms (monitoring targets)

| Platform | OS Detection | Uptime | Load | CPU | RAM | Swap | Disk | Inodes | Network | Users | Processes |
|----------|:-----------:|:------:|:----:|:---:|:---:|:----:|:----:|:------:|:-------:|:-----:|:---------:|
| Linux    | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| macOS (Darwin) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| FreeBSD  | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ | ✓ | ✓ |
| OpenBSD  | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ | ✓ | ✓ |
| NetBSD   | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ | ✓ | ✓ |
| DragonFlyBSD / MidnightBSD | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ | ✓ | ✓ |
| Solaris / illumos / AIX / HP-UX | ✓ | partial | ✓ | n/a | partial | partial | ✓ | ✓ | partial | ✓ | ✓ |
| Windows over OpenSSH | ✓ | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |

\* Swap is reported as `null`/`n/a` when not configured on the host (not an error).

### Platform-specific notes

- **Linux**: Uses `/proc/uptime`, `/proc/loadavg`, `/proc/meminfo`,
  `/proc/stat` (two samples), `/proc/net/dev`, and `df -B1 -P` for disk.
- **macOS (Darwin)**: Uses `sysctl`, `vm_stat`, `top -l 2 -n 0 -s 1` (two
  samples with 1-second interval), `netstat -ibn`, and `df -kP`.
- **FreeBSD**: Uses `sysctl kern.cp_time` (two samples), `sysctl vm.stats.*`,
  `swapinfo -k`, `netstat -ibn`, and `df -kP`.
- **OpenBSD**: Uses `sysctl kern.cp_time` (key=value format), `sysctl
  vm.uvmexp`, `swapctl -s -k`, and `netstat -ibn`.
- **NetBSD**: Similar to FreeBSD; uses `sysctl vm.uvmexp2` for memory.
- **DragonFlyBSD / MidnightBSD**: Use the FreeBSD-compatible collector path.
- **Solaris / illumos / AIX / HP-UX**: Use a conservative generic Unix
  collector based on `uname`, `hostname`, `getconf`, `uptime`, `df`, `who`,
  `ps`, `netstat`, and platform-specific best-effort commands such as
  `prtconf` and `swap -s` where available.
- **Windows over OpenSSH**: Detected explicitly so it does not fall through to
  Linux commands. System identity is reported, while Unix system metrics are
  marked unsupported. Connectivity probes still run from the WatchSSH host.
- **Standard Unix tool modules**: All platform collectors also try `df -iP`
  for inode usage and `who` for active login sessions. Failures are reported
  via `capabilities.inodes` and `capabilities.logged_users`.

If an unknown Unix-like OS is detected, WatchSSH falls back to a generic Unix
collector first. Metrics that cannot be collected are marked `null` in JSON
output and `n/a` in console output, with the reason recorded in the
`capabilities` field.

### Additional Metrics

WatchSSH also collects these additive metrics when the target platform exposes
them through standard tools:

- CPU core count (`system.cpu_cores`)
- Linux process scheduler counts from `/proc/loadavg`
  (`load.running_processes`, `load.total_processes`, `load.last_pid`)
- Filesystem inode usage per mount (`disks[].inodes_*`)
- Network receive/transmit errors and drops per interface (`network[].errors_*`,
  `network[].drops_*`)
- Linux/macOS file descriptor pressure from `/proc/sys/fs/file-nr` or `sysctl`
  (`file_descriptors`)
- Linux Raspberry Pi / SBC board diagnostics when exposed by the host:
  model, thermal zone temperature, CPU frequency, `vcgencmd get_throttled`
  flags, and Wi-Fi RSSI from `/proc/net/wireless` (`board`)

Capability keys for these metrics are `cpu_cores`, `disk_inodes`, and
`file_descriptors`. Board diagnostics use the `board` capability key.

## CPU Measurement

CPU utilisation requires two measurements to compute a delta. On platforms with
known CPU counters, WatchSSH reads the CPU counters twice with a 1-second sleep
between reads. This means:

- **Each collection cycle takes at least 1 second per host** (the SSH connection
  stays open during the sleep).
- The reported CPU% reflects the activity during that 1-second window.
- Platforms that use `top -l 2` (macOS) achieve this via the tool itself.
- Platforms that use `kern.cp_time` or `/proc/stat` take two readings in the
  same SSH session.
- Generic Unix and Windows targets mark CPU utilisation as unsupported until a
  reliable platform-specific counter is available.

There is no "first-poll warming up" issue because WatchSSH uses two reads per
cycle, not a running-average from a background sampler.

I/O wait (`iowait_percent`) is only available on Linux. It is 0 on other
platforms but the field is always present in JSON output.

## Docker Observability (Linux only)

WatchSSH can optionally collect Docker container metrics on Linux hosts. Enable
it per-server in the configuration:

```yaml
servers:
  - name: docker-host
    host: 192.0.2.10
    username: monitor
    docker:
      enabled: true   # Default: false. Linux only.
```

When enabled, WatchSSH runs two commands on the target:

1. `docker ps --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}'` — discovers
   running containers.
2. `docker stats --no-stream --format '...'` — collects CPU, memory, network I/O,
   and block I/O for each container in a single invocation.

**Prerequisites on the target host:**

- Docker must be installed and the daemon must be running.
- The monitoring user must be in the `docker` group (or have equivalent access):
  ```bash
  sudo usermod -aG docker monitor
  ```

**Capability flags** when Docker is not available:

| Situation | `capabilities.containers` value |
|-----------|-------------------------------|
| Docker enabled, running, containers found | `"ok"` |
| Docker enabled but daemon not reachable | `"unavailable"` |
| Docker enabled on non-Linux platform | `"unsupported"` |
| Docker disabled in config | field absent |

Container data appears under `containers` in JSON output and in a dedicated
section in the console output. Containers with no running instances result in
an empty array and capability `"ok"`.

## Bounded Worker Pool

By default, WatchSSH spawns one goroutine per configured server. For large
deployments, cap concurrency with the global `workers` setting:

```yaml
workers: 10   # Poll at most 10 servers at a time. Default: 0 (unlimited).
```

Each goroutine is governed by `context.Context` timeouts — a single slow or
failing host will not block other polling goroutines beyond the configured
`timeout`.

## Installation

```bash
go install github.com/SimonWaldherr/WatchSSH@latest
```

Or build from source:

```bash
git clone https://github.com/SimonWaldherr/WatchSSH
cd WatchSSH
go build -o watchssh .
```

## Quick Start

```bash
./watchssh -once
# No config yet? WatchSSH inspects localhost and local Docker automatically.

cp config.example.yaml config.yaml
# Edit config.yaml to add your servers
./watchssh -config config.yaml
```

For a single collection cycle:

```bash
./watchssh -config config.yaml -once
```

For a targeted single cycle (only matching servers and tags):

```bash
./watchssh -config config.yaml -once -servers web-01,db-01 -tags linux,production
```

For JSON output:

```bash
./watchssh -config config.yaml -once 2>/dev/null   # (set output.type: json in config)
```

To keep a local history of metric samples and alert firings:

```yaml
storage:
  type: tinysql
  path: ./watchssh.tinysql
  retention_days: 30
  max_size_mb: 512
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `config.yaml` | Path to YAML configuration file |
| `-once` | false | Run one collection cycle and exit |
| `-servers` | `""` | Comma-separated server names to monitor |
| `-tags` | `""` | Comma-separated server tags to monitor |
| `-verbose` | false | Enable verbose logging (file names, line numbers) |
| `-version` | false | Print version and exit |

When both `-servers` and `-tags` are provided, a server must match both filters.

## Configuration

See [`config.example.yaml`](config.example.yaml) for a fully commented example.

### Host Key Verification (Security)

WatchSSH performs strict SSH host key verification by default. Before adding a
new host to your config, verify its fingerprint out-of-band:

```bash
# Step 1 – connect interactively and note the fingerprint shown by ssh
ssh monitor@192.0.2.10

# Step 2 – once you've verified it, add to known_hosts
ssh-keyscan -H 192.0.2.10 >> ~/.ssh/known_hosts
```

**Never** add host keys blindly from untrusted sources. A wrong host key means
you may be sending credentials and commands to an attacker's machine (MITM).

If WatchSSH encounters an unknown or changed host key, it will fail with a clear
error message:

```
host key setup: cannot load known_hosts from "…":
  → Add the host key with: ssh-keyscan -H <host> >> ~/.ssh/known_hosts
  → Verify the fingerprint out-of-band before adding it.
```

To allow a specific host without key checking (for testing only):

```yaml
servers:
  - name: test-vm
    host: 192.168.1.100
    username: monitor
    insecure_ignore_host_key: true   # DANGEROUS — do not use in production
```

### Authentication

WatchSSH supports three authentication methods:

| Method | Config | Notes |
|--------|--------|-------|
| Private key | `auth.type: key` | Default. Uses `~/.ssh/id_rsa` if no `key_file` set |
| SSH agent | `auth.type: agent` | Requires `SSH_AUTH_SOCK` to be set |
| Password | `auth.type: password` | Least secure; prefer key-based auth |

### Security Best Practices

1. **Dedicated monitoring user** — Create a read-only `monitor` user on each
   target. Do not use root.
2. **Key-based authentication** — Prefer SSH keys over passwords.
3. **Verify host keys** — Always verify fingerprints out-of-band before adding
   to `known_hosts`.
4. **Least privilege** — The monitoring user only needs to run the commands
   listed below. Use `sudo` restrictions or dedicated paths if needed.
5. **Keep known_hosts up to date** — Rotate host keys when servers are
   reprovisioned.

Commands run on target hosts:

| Platform | Commands |
|----------|----------|
| Linux | `uname`, `hostname`, `getconf`/`nproc`, `cat /proc/uptime`, `cat /proc/loadavg`, `cat /proc/meminfo`, `cat /proc/stat` (×2), `df`, `cat /proc/net/dev`, `cat /proc/sys/fs/file-nr`, `ps` |
| macOS | `uname`, `hostname`, `getconf`/`sysctl`, `sysctl kern.boottime`, `sysctl vm.loadavg`, `sysctl hw.memsize`, `sysctl kern.num_files`, `sysctl kern.maxfiles`, `vm_stat`, `sysctl vm.swapusage`, `top`, `df`, `netstat`, `ps`, `who` |
| Linux + Docker | same as above, plus `docker version`, `docker ps`, `docker stats --no-stream` |
| FreeBSD | `uname`, `hostname`, `sysctl`, `swapinfo`, `df`, `netstat`, `ps` |
| OpenBSD | `uname`, `hostname`, `sysctl`, `swapctl`, `df`, `netstat`, `ps` |
| NetBSD | `uname`, `hostname`, `sysctl`, `swapctl`, `df`, `netstat`, `ps` |
| DragonFlyBSD / MidnightBSD | FreeBSD-compatible path: `uname`, `hostname`, `sysctl`, `swapinfo`, `df`, `netstat`, `ps` |
| Solaris / illumos / AIX / HP-UX / unknown Unix | generic path: `uname`, `hostname`, `getconf`, `uptime`, `prtconf`, `swap`, `df`, `netstat`, `ps`, `who` |
| Windows over OpenSSH | `cmd /c ver`, `hostname`; Unix system metrics are marked unsupported |

## Output

### Console

```
┌──────────────────────────────────────────────────────────────────────┐
│ Server : web-01  (192.0.2.10)                                        │
│ Time   : 2026-04-18T07:23:36Z                                        │
│ OS     : Linux                                                       │
├──────────────────────────────────────────────────────────────────────┤
│ OS     : Linux  Kernel: 6.1.0-21-amd64  Arch: x86_64                │
│ Host   : web-01   Uptime: 14d 3h12m5s                               │
├──────────────────────────────────────────────────────────────────────┤
│ Load   : 0.42  0.35  0.28  (1/5/15 min)                             │
│ CPU    : 8.2%  (user 5.1%  sys 3.1%  iowait 0.2%  idle 91.6%)      │
├──────────────────────────────────────────────────────────────────────┤
│ RAM    : 5.2 GiB / 16.0 GiB  (32.5%)                               │
├──────────────────────────────────────────────────────────────────────┤
...
```

### JSON

The JSON schema includes a `schema_version` field and uses `null` for
unavailable or unsupported metrics:

```json
[
  {
    "server_name": "web-01",
    "schema_version": "2",
    "platform": "Linux",
    "cpu": {
      "usage_percent": 8.2,
      "user_percent": 5.1,
      "system_percent": 3.1,
      "idle_percent": 91.6,
      "iowait_percent": 0.2
    },
    "memory": { ... },
    "swap": null,
    "capabilities": {
      "cpu": "ok",
      "memory": "ok",
      "swap": "unsupported",
      "load": "ok",
      "disks": "ok",
      "disk_inodes": "ok",
      "cpu_cores": "ok",
      "inodes": "ok",
      "logged_users": "ok",
      "network": "ok",
      "file_descriptors": "ok"
    }
  }
]
```

**Capability values:**
- `"ok"` — metric collected successfully
- `"unsupported"` — not available on this platform
- `"unavailable"` — temporarily unavailable (e.g. first poll)
- `"error"` — collection failed; see `metric_errors` for details

## History Storage

WatchSSH is stateless by default: the web UI keeps only the latest live values
in memory, and `output` controls console or JSON export. Optional embedded
history storage can be enabled with:

```yaml
storage:
  type: tinysql
  path: ./watchssh.tinysql
  retention_days: 30
  max_size_mb: 512
```

When enabled, WatchSSH writes each collected server sample to `metric_samples`
and each newly-triggered alert to `alert_firings`. The tables include compact
query columns such as timestamp, server name, platform, CPU usage, memory usage,
root disk usage, load, ping status, alert metric, and error status, plus the
full JSON payload for forward-compatible analysis.

`retention_days` removes older records. `max_size_mb` trims the oldest history
records after the database file grows beyond the configured size.

The web dashboard exposes the stored data at `/history`. JSON consumers can use:

- `GET /api/history/metrics?server=<name>&limit=100`
- `GET /api/history/alerts?limit=100`
- `GET /api/history/summary?server=<name>&limit=500`
- `GET /api/probes?server=<name>`

WatchSSH also exposes current live values in Prometheus text format at
`GET /metrics`. This endpoint reports current state only; persisted history
remains in the configured history store.

## Network Probes

RIPE Atlas is useful because it standardises small, repeatable measurements
from well-known vantage points: ping, DNS, traceroute, TLS/HTTP, and timing.
WatchSSH adopts the same style for local agentless probes run from the
monitoring host:

```yaml
checks:
  http:
    - url: https://example.com/health
      method: GET
      expected_status: 200
      expected_body: '"status":"ready"' # optional response substring
  dns:
    - name: public-dns
      host: example.com
      type: A
      server: 1.1.1.1
      expected_answer: 93.184.216.
      timeout: 5
  traceroute:
    - name: path-to-edge
      host: example.com
      max_hops: 30
      timeout: 10
  tls:
    - name: public-cert
      host: example.com
      port: 443
      server_name: example.com
      timeout: 5
  ntp:
    - name: cloudflare-time
      host: time.cloudflare.com
      max_offset_ms: 100 # optional: fail on excessive clock drift
      timeout: 5
```

These probes are included in JSON output, `/history`, `/api/history/summary`,
and `/metrics`. WatchSSH does not try to become a distributed RIPE Atlas
replacement; it keeps the single-monitoring-host model and makes probe results
consistent enough for alerts and exports.

## HARP Integration

For [HARP](https://github.com/SimonWaldherr/HARP), WatchSSH should start with
the operational checks that are cheap and already supported: health endpoint,
readiness endpoint, public `/metrics` reachability, DNS resolution, and TLS
certificate expiry. Example:

```yaml
servers:
  - name: harp-proxy
    host: harp.example.com
    username: monitor
    checks:
      http:
        - url: https://harp.example.com/health
          expected_status: 200
        - url: https://harp.example.com/readyz
          expected_status: 200
        - url: https://harp.example.com/metrics
          expected_status: 200
      dns:
        - name: harp-public-dns
          host: harp.example.com
          type: A
      tls:
        - name: harp-public-cert
          host: harp.example.com
          port: 443
          server_name: harp.example.com
```

The `/metrics` check currently verifies that HARP's metrics endpoint is up and
responds with the expected status. Parsing selected HARP counters into native
WatchSSH metrics can be added later once the HARP metric names are stable.

## Alerting

Configure threshold-based alerts in the `alerts` section of `config.yaml`.
Supported metrics: `cpu_usage`, `mem_usage`, `swap_usage`, `load1`, `load5`,
`load15`, `disk_usage`, `disk_inode_usage`, `processes_running`,
`processes_total`, `file_descriptor_usage`, `network_errors`, `network_drops`,
`ping_latency`, `ping_failed`, `port_closed`, `port_latency`, `http_failed`,
`http_latency`, `dns_failed`, `dns_latency`, `traceroute_failed`,
`traceroute_hops`, `tls_failed`, `ntp_failed`, `ntp_latency`, `ntp_offset`,
`custom_failed`, `cert_expires_days`, `tls_cert_expires_days`,
`board_temperature`, `board_under_voltage`, `board_throttled`,
`board_wifi_rssi`.

Email notifications via SMTP (with STARTTLS or TLS) are supported.

Optional guarded alert actions are also supported via `alerts.action`:
- command is executed directly (no shell)
- executable must be listed in `allowed_executables`
- command receives firings JSON on stdin

## Web Dashboard

Enable the built-in web UI:

```yaml
web:
  enabled: true
  listen: ":8080"
```

Then open `http://localhost:8080` in your browser.

If your config has an empty `servers:` list, the UI and CLI still show a
temporary `localhost` diagnostic target so you can inspect the host running
WatchSSH before scaling out to more systems.

The server form can now create common profiles directly from the UI:
custom SSH targets, web/HTTPS services, HARP reverse proxies, Raspberry Pi/SBC
hosts, and local monitoring targets. It also supports tags, Docker collection,
ping, TCP port, HTTP response content, DNS, TLS, traceroute, NTP, and one custom
command check without editing YAML by hand. Frequent TCP and HTTP settings stay
visible; less common network probes are grouped in an expandable section.

Health endpoints for automation:

- `GET /healthz` → liveness (`200 ok`)
- `GET /readyz` → readiness (`200` when first metrics are available, otherwise `503`)

## Architecture

```
main.go
├── internal/config     — YAML config loading and validation
├── internal/monitor    — Polling loop, data model (ServerMetrics), output
│   ├── collectors.go   — Low-level parsers (kept for test coverage)
│   ├── metrics.go      — Canonical data model
│   ├── monitor.go      — Polling loop + platform dispatch (applySnapshot)
│   ├── output.go       — Console and JSON renderers
│   └── alert.go        — Alert rule evaluation + email delivery
├── internal/platform   — OS detection + platform-specific collectors
│   ├── platform.go     — Family, Collector interface, Detect(), New()
│   ├── common.go       — Shared parsers (df, ps, sysctl boottime/loadavg)
│   ├── linux.go        — /proc/* based collector
│   ├── darwin.go       — sysctl/vm_stat/top based collector
│   ├── freebsd.go      — sysctl kern.cp_time based collector
│   ├── openbsd.go      — OpenBSD-specific sysctl collector
│   ├── netbsd.go       — NetBSD-specific sysctl collector
│   ├── generic_unix.go — Conservative collector for Solaris/illumos/AIX/HP-UX/unknown Unix
│   ├── windows.go      — Windows-over-OpenSSH detection with unsupported Unix metrics
│   └── docker.go       — Optional Docker container collector (Linux only)
├── internal/ssh        — SSH client with strict host key checking
├── internal/check      — Ping, TCP, HTTP, DNS, traceroute, TLS, and NTP probes
└── internal/web        — Embedded web dashboard (HTML/CSS/JS)
```

## Known Limitations / Open Items

- Generic Unix targets provide best-effort metrics; unsupported counters are
  surfaced through `capabilities` rather than guessed.
- `iowait_percent` is Linux-only; it is always `0` on BSD/macOS.
- On NetBSD, memory stats use `vm.uvmexp2` which may differ across NetBSD versions.
- Docker observability is Linux-only; enabling `docker.enabled` on non-Linux targets
  results in `capabilities.containers = "unsupported"` rather than an error.
- File descriptor pressure is currently Linux/macOS-only; other targets report
  `capabilities.file_descriptors = "unsupported"`.
- The web UI's server-detail page shows platform/capabilities but has no
  history/graphing capability.
- The JSON schema version is "2"; changes that remove or rename fields will
  increment the version.
- Process RSS bytes are available only on platforms where `ps` reports them in
  KB (Linux, BSD, macOS). On others the field may be 0.

## License

MIT
