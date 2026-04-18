# WatchSSH

[![CI](https://github.com/SimonWaldherr/WatchSSH/actions/workflows/ci.yml/badge.svg)](https://github.com/SimonWaldherr/WatchSSH/actions/workflows/ci.yml)

**Agentless monitoring of Unix-like servers over SSH.**

WatchSSH connects to remote hosts via SSH, runs only standard system tools
that are already present on the target, collects metrics, and reports them
to the console or as JSON. No agent installation required on the monitored
hosts — only an SSH user with read-only access to system utilities.

## Supported Platforms (monitoring targets)

| Platform | OS Detection | Uptime | Load | CPU | RAM | Swap | Disk | Network | Processes |
|----------|:-----------:|:------:|:----:|:---:|:---:|:----:|:----:|:-------:|:---------:|
| Linux    | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| macOS (Darwin) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| FreeBSD  | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ |
| OpenBSD  | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ |
| NetBSD   | ✓ | ✓ | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ | ✓ |

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

If an unknown or unsupported OS is detected, WatchSSH falls back to Linux-style
commands. Metrics that cannot be collected are marked `null` in JSON output and
`n/a` in console output, with the reason recorded in the `capabilities` field.

## CPU Measurement

CPU utilisation requires two measurements to compute a delta. On all platforms,
WatchSSH reads the CPU counters twice with a 1-second sleep between reads. This
means:

- **Each collection cycle takes at least 1 second per host** (the SSH connection
  stays open during the sleep).
- The reported CPU% reflects the activity during that 1-second window.
- Platforms that use `top -l 2` (macOS) achieve this via the tool itself.
- Platforms that use `kern.cp_time` or `/proc/stat` take two readings in the
  same SSH session.

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
| Linux | `uname`, `hostname`, `cat /proc/uptime`, `cat /proc/loadavg`, `cat /proc/meminfo`, `cat /proc/stat` (×2), `df`, `cat /proc/net/dev`, `ps` |
| Linux + Docker | same as above, plus `docker version`, `docker ps`, `docker stats --no-stream` |
| macOS | `uname`, `hostname`, `sysctl`, `vm_stat`, `top`, `df`, `netstat`, `ps` |
| FreeBSD | `uname`, `hostname`, `sysctl`, `swapinfo`, `df`, `netstat`, `ps` |
| OpenBSD | `uname`, `hostname`, `sysctl`, `swapctl`, `df`, `netstat`, `ps` |
| NetBSD | `uname`, `hostname`, `sysctl`, `swapctl`, `df`, `netstat`, `ps` |

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
      "network": "ok"
    }
  }
]
```

**Capability values:**
- `"ok"` — metric collected successfully
- `"unsupported"` — not available on this platform
- `"unavailable"` — temporarily unavailable (e.g. first poll)
- `"error"` — collection failed; see `metric_errors` for details

## Alerting

Configure threshold-based alerts in the `alerts` section of `config.yaml`.
Supported metrics: `cpu_usage`, `mem_usage`, `swap_usage`, `load1`, `load5`,
`load15`, `disk_usage`, `ping_latency`, `ping_failed`, `port_closed`,
`http_failed`, `custom_failed`, `cert_expires_days`.

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
│   └── docker.go       — Optional Docker container collector (Linux only)
├── internal/ssh        — SSH client with strict host key checking
├── internal/check      — Ping, port, and HTTP connectivity checks
└── internal/web        — Embedded web dashboard (HTML/CSS/JS)
```

## Known Limitations / Open Items

- Solaris/Illumos is not yet supported (no backend).
- `iowait_percent` is Linux-only; it is always `0` on BSD/macOS.
- On NetBSD, memory stats use `vm.uvmexp2` which may differ across NetBSD versions.
- Docker observability is Linux-only; enabling `docker.enabled` on non-Linux targets
  results in `capabilities.containers = "unsupported"` rather than an error.
- The web UI's server-detail page shows platform/capabilities but has no
  history/graphing capability.
- The JSON schema version is "2"; changes that remove or rename fields will
  increment the version.
- Process RSS bytes are available only on platforms where `ps` reports them in
  KB (Linux, BSD, macOS). On others the field may be 0.

## License

MIT
