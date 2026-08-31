# WatchSSH Operator Guide

WatchSSH is a central, agentless monitoring service. It connects to targets
over SSH only when it needs host metrics or a remote custom check; HTTP, DNS,
TLS, NTP, traceroute, and monitor-originated TCP probes run from the WatchSSH
machine. Target systems do not need a WatchSSH agent.

## 1. Start Safely

Build and validate the project:

```bash
make check
cp config.example.yaml config.yaml
make run CONFIG=config.yaml
```

For a single collection cycle, use `make once CONFIG=config.yaml`. With no
configured targets, WatchSSH provides a local diagnostic profile so the UI can
be used to add the first server.

Console output refreshes in place when WatchSSH is attached to an interactive
terminal. Redirect it to a file or pipe it to another process when an
append-only history is required; JSON output is always append-only.

Run WatchSSH under a dedicated account. Use a restricted SSH account on remote
hosts, strict host-key verification, and secret sources rather than plaintext
credentials in configuration files.

WatchSSH supports non-default SSH ports. In the web form, enter the port in
**SSH Port** or use `host:port`, such as `ssh.example.test:50622`. For IPv6
with a port, use brackets: `[2001:db8::1]:2200`.

## 2. Add a Target

Use the Servers page to create a target, select a profile, test the connection,
and save it. A minimal YAML equivalent is:

```yaml
known_hosts_path: /etc/watchssh/known_hosts
strict_host_key_checking: true

servers:
  - name: app-01
    host: 10.20.0.11
    username: monitor
    auth:
      type: key
      key_file: /etc/watchssh/id_ed25519
    tags: [production, application]
```

Use `auth.private_key`, `auth.password_source`, or Vault KV sources where
appropriate. Avoid disabling host-key validation. The Security Checks view
flags insecure host-key policy and password-based SSH authentication so these
decisions remain visible.

Set `tool_inventory: true` for a target when you want WatchSSH to record which
standard tools are available. It performs one read-only POSIX-shell command
using `command -v`; it does not install packages, require `which`, or change
the target. The inventory covers core tools such as `df`, `du`, `ps`, `awk`,
and `stat`, plus optional operational tools including `ss`, `ip`, `systemctl`,
`journalctl`, `curl`, `openssl`, and `docker`. The Probe Library also provides
typed SSH checks for files (`test` + `stat`), directories (`du` and optional
bounded `find`), and log pattern counts (`tail` + `grep`). These probes require
absolute paths and return only metadata or counts, never file or log contents.

For a small automatic target audit, explicitly enable it per server:

```yaml
audit:
  enabled: true
  max_entries: 200
```

On Linux, WatchSSH reads account names and UIDs through `getent passwd` (with a
read-only `/etc/passwd` fallback), then package names through `dpkg-query`,
`rpm`, or `apk` when available. The result is bounded independently for users
and packages. It does not collect password hashes, home-directory contents,
package metadata, or credentials. Treat account and package inventories as
sensitive operational data and protect dashboard/API access accordingly.

The Server Detail page also provides **Run audit** for an immediate, on-demand
read-only audit. WatchSSH keeps the latest 20 audit snapshots for each target
while it runs and compares every result to its predecessor. The UI lists added
and removed account names/UIDs and package names. Restarting WatchSSH resets
this in-memory audit history; enable tinySQL for durable metric and alert
history, but do not treat it as an audit-evidence archive.

## 3. Add Probes

The Probe Library provides a fast path for individual checks and portable
JSON/YAML bundles for repeatable setups. Use separate probes when one failure
must mean one clear operational outcome:

| Need | Probe |
| --- | --- |
| Is the host reachable? | Ping |
| Is a TCP service listening? | TCP port |
| Can the application answer its health endpoint? | HTTP |
| Does a name resolve through the expected resolver? | DNS |
| Is the TLS endpoint valid and not near expiry? | TLS |
| Is the target network able to reach a private dependency? | TCP with `source: target` |
| Is time synchronized? | NTP |
| Does the SSH protocol and optional pinned host key respond? | SSH handshake |
| Does a known remote command return an expected value? | Custom SSH command |
| Does an important file exist, have the expected size, and remain fresh? | File metadata (`test` + `stat`) |
| Is a cache, spool, or upload directory within a size or file-count budget? | Directory usage (`du` + bounded `find`) |
| Are recent log lines free of a known error pattern? | Log pattern count (`tail` + `grep`) |
| Is a service, process, or listening port present on a Unix target? | Service, process, or listener probe |
| Does a local Unix socket or required service account exist? | Unix socket (`test -S`) or service account (`id -u`) |

Exported bundles include only `checks`. They never include authentication,
private keys, passwords, or server tags. They can include endpoints and a
pinned SSH host-key fingerprint because those are part of a probe definition.
Review imported custom commands before enabling them.

The file, directory, and log probes are designed for the normal remote shell
toolset rather than a WatchSSH agent. They use only metadata or aggregate
counts: WatchSSH does not store file contents, directory listings, or log
lines. File and directory paths must be absolute. Give each probe a narrow
purpose and a short timeout; a directory file-count check stops after the
configured limit is exceeded.

For Unix compatibility, service checks prefer `systemctl` and fall back to
`service`; process checks prefer `pgrep` and fall back to `ps` plus `grep`;
listener checks prefer `ss` and fall back to `netstat`. If neither tool is
available, the result clearly reports an unsupported probe instead of
silently treating the target as healthy.

An SSH handshake is intentionally credential-free: it performs key exchange,
records the server host-key fingerprint, and can compare it with an optional
`SHA256:...` fingerprint. It validates the SSH endpoint without logging in or
running a command. Socket and service-account probes run through the normal
authenticated SSH connection and only use POSIX `test -S` and `id -u`.

## 4. Model Dependencies

Declare only real upstream dependencies. If an upstream target is unreachable,
WatchSSH suppresses alert routing for dependent targets, preventing a single
network outage from producing a cascade of downstream incidents.

```yaml
servers:
  - name: router-01
    host: 10.20.0.1
    username: monitor
    auth: {type: key, key_file: /etc/watchssh/id_ed25519}

  - name: app-01
    host: 10.20.0.11
    username: monitor
    depends_on: [router-01]
    auth: {type: key, key_file: /etc/watchssh/id_ed25519}
```

Dependencies must name existing servers and cannot contain duplicates, self
references, or cycles. They suppress routing only for an unavailable upstream;
they do not silently hide unrelated application failures.

## 5. Create Alerts and Record Changes

Create focused alert rules for conditions that need a response. Use the Alerts
page to record deployments, maintenance, restarts, and configuration changes.
The change timeline gives operators context when an alert follows a known
change.

Use a maintenance window or disable only the relevant alert routes during a
planned outage. Do not broadly disable SSH host-key checks or all monitoring
to silence expected work.

## 6. Use Runbooks and AI Advice

Deterministic remediation is configured separately and must be explicitly
enabled. Keep every command narrow, target-specific, rate-limited, and backed
by a least-privilege SSH or `sudoers` policy.

A recovery policy links one scoped alert rule to one fixed command. For a
classic Apache host, use `pidof: apache2` in a named process probe, create an
alert rule for `process_failed` scoped to that probe, and attach a remediation
with `/etc/init.d/apache2 restart`. Add `verify_command: pidof apache2 >/dev/null`
and a short `verify_delay` so a successful restart means Apache is running,
not merely that the init script returned zero. The same pattern applies to
`systemctl restart`, `service ... restart`, `docker restart`, Compose, or an
application-owned recovery command. Recovery is disabled until
`enabled: true` is explicitly set and is bounded by its cooldown and attempt
budget.

The optional `alerts.watchdog` integration is an AI advisor, not an execution
engine. It sends a bounded, reduced probe snapshot to an OpenAI-compatible API
such as LM Studio and returns a summary, severity, and possible runbook names.
It has no shell or tool access. WatchSSH never executes an AI recommendation.

Each recommendation creates a Runbook Review item. An operator must inspect
the telemetry, acknowledge or decline the recommendation, and execute an
approved runbook through the established operational process. Review state is
an audit aid; it does not authorize commands.

## 7. Schedule Local Artifact Jobs

Use a scheduled job when WatchSSH should perform work on its own host and then
publish a resulting file to an existing SSH target. A Mac mini can, for
example, download an OSM extract, run a local transformation script, and
upload the resulting `.pbf` file to a Hetzner server. This is deliberately a
configuration-owned workflow, not a dashboard shell.

```yaml
servers:
  - name: hetzner-osm
    host: osm.example.net
    port: 50622
    username: watchssh
    auth:
      type: key
      key_file: /Users/watchssh/.ssh/hetzner_osm_ed25519

jobs:
  - name: update-bavaria-osm
    enabled: true
    schedule: "15 3 * * 1"
    timeout: 7200
    working_directory: /Users/watchssh/osm
    command: ./refresh-bavaria.sh
    uploads:
      - server: hetzner-osm
        source: /Users/watchssh/osm/out/bavaria.osm.pbf
        destination: /srv/www/osm/bavaria.osm.pbf
        create_directories: true
```

`command` runs locally through `sh -c`; it receives no shell input from the
web UI. `schedule` accepts five-field cron expressions with wildcards, lists,
ranges, and steps, plus descriptors such as `@daily`. Jobs are disabled until
`enabled: true` is set, run for at most `timeout` seconds, and do not execute
in `--once` mode. Set `run_on_start: true` only for idempotent jobs that should
run immediately after a restart.

Each `source` must be an absolute, regular local file. Each `destination` must
be an absolute path on a named non-local target. The upload uses that target's
configured SSH port, host-key verification, credential source, and optional
bastion. The Unix target needs `scp`, `mkdir`, and `mv`; no separate WatchSSH
agent or local transfer utility is involved. Artifacts are written to a random
sibling `.partial` name and atomically renamed on success, preserving the
previously published artifact on failure. The Jobs page shows the current
configuration and the most recent 100 in-memory results, including transfer
sizes and failures. It neither reveals command text nor retains command output.

Keep local scripts versioned, non-interactive, and idempotent. Verify download
checksums in the script before publishing and use a dedicated restricted SSH
account whose write access is limited to the target directory.

## 8. Use Inventory and Security Checks

The Servers page shows a non-secret asset inventory: target name, observed OS,
architecture, tags, SSH/bastion use, declared dependencies, and last-seen time.
Use it to identify unknown targets, stale assets, and unsupported platforms.

Security Checks currently identify:

- disabled SSH host-key verification;
- password or keyboard-interactive SSH authentication;
- failed TLS probes;
- TLS certificates with 30 days or less remaining.

Treat these as operational findings. Confirm ownership and impact before a
change, then record the work in Change Correlation.

## 9. Integrate and Operate

Keep the dashboard private or put it behind TLS and authentication. Health
endpoints remain public by design:

| Endpoint | Use |
| --- | --- |
| `/healthz`, `/livez` | Process liveness |
| `/readyz` | First monitoring data is available |
| `/metrics` | Prometheus scrape target |
| `/openapi.json` | OpenAPI 3.1 description |
| `/api/v1/inventory` | Asset and SSH inventory |
| `/api/v1/security/findings` | Security posture and observed TLS findings |

Use `/api/v1/...` for new integrations. See [Operations and API Reference](operations.md)
for authentication, reverse-proxy, Prometheus, and API-contract details.

## Incident Workflow

1. Confirm WatchSSH is ready with `/readyz`.
2. Identify the first failed upstream target and inspect its probes.
3. Check declared dependencies before treating downstream failures separately.
4. Review recent changes, alert history, and security findings.
5. Follow the appropriate runbook; review AI suggestions as evidence, never as authority.
6. Record the change or remediation, then confirm recovery with the relevant probes.
7. Improve probes, dependencies, and runbooks after the incident.
