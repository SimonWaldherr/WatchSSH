# Operations and API Reference

WatchSSH is designed to run as one central, agentless monitoring service. This
document defines its HTTP contracts for load balancers, orchestrators, reverse
proxies, dashboards, and API consumers.

## Endpoint Contract

| Endpoint | Access | Purpose | Success response |
| --- | --- | --- | --- |
| `GET /healthz` | public | Process liveness. It does not depend on storage, SSH, or a completed poll. | `200 ok` |
| `GET /livez` | public | Kubernetes-compatible alias of `/healthz`. | `200 ok` |
| `GET /readyz` | public | Readiness. It becomes ready after WatchSSH has data for configured targets. | `200` or `503` JSON |
| `GET /metrics` | authenticated when `web.auth` is set | Current Prometheus text exposition. | `200 text/plain` |
| `GET /openapi.json` | authenticated when `web.auth` is set | OpenAPI 3.1 description of the stable HTTP API. | `200 application/vnd.oai.openapi+json` |
| `GET /api/v1/...` | authenticated when `web.auth` is set | Versioned programmatic API. | JSON or problem JSON |

Health endpoints and JSON API responses set `Cache-Control: no-store`. Health
endpoints deliberately remain public so systemd, Docker, Kubernetes, and load
balancers can check WatchSSH without receiving dashboard credentials.

## API Versioning

New integrations must use `/api/v1`. The available endpoints are:

```text
GET  /api/v1/metrics
GET  /api/v1/probes?server=<name>
GET  /api/v1/history/metrics?server=<name>&limit=100
GET  /api/v1/history/alerts?limit=100
GET  /api/v1/history/summary?server=<name>&limit=500
POST /api/v1/test-connection
```

`limit` is bounded to 500. The unversioned `/api/...` routes are maintained as
compatibility aliases; they should not be selected for new integrations.

The OpenAPI document is intentionally served without a bundled Swagger UI.
This keeps WatchSSH self-contained and avoids loading third-party browser code.
Import `https://watchssh.example/openapi.json` into Swagger UI, Redoc, Bruno,
Postman, Insomnia, or a code generator instead.

API errors use [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem
details with `application/problem+json`, including `type`, `title`, `status`,
`detail`, `instance`, and `request_id`. Every HTTP response includes an
`X-Request-ID`; reverse proxies should preserve or log this value.

## Prometheus

`/metrics` exports only the current in-memory monitoring state. Persistent
history stays in tinySQL and is available through the history API.

Alongside target and probe metrics, WatchSSH exports service metrics:

```text
watchssh_build_info
watchssh_process_start_time_seconds
watchssh_ready
watchssh_configured_servers
watchssh_collected_servers
watchssh_last_collection_timestamp_seconds
```

Labels are deliberately bounded. In particular, HTTP probe URLs are not used
as Prometheus labels because they can contain sensitive query parameters and
cause unbounded cardinality. HTTP probes use a stable per-server `probe` index
and method label instead.

## Reverse Proxy and TLS

Bind WatchSSH to loopback where possible, then terminate TLS and authenticate
users at a reverse proxy. Preserve `X-Request-ID` and do not cache health, API,
or metrics responses. When `web.auth` is configured, Basic authentication
protects the dashboard, OpenAPI document, API, and Prometheus endpoint; the
three health endpoints remain public by design.

Use a dedicated monitoring service account, strict SSH host key checking, and
least-privilege remote commands. Do not place SSH keys, passwords, bearer
tokens, or full HTTP probe URLs in logs, labels, or dashboards with broad
access.

## Bastions and Private Network Probes

Use `proxy_jump` for a single explicit SSH bastion. WatchSSH validates the
bastion and destination host keys independently and opens the target session
with SSH `direct-tcpip`; no local `ProxyCommand` is evaluated. Bastion and
target credentials may use the same secret sources as normal servers.

SSH keepalives are opt-in per server. Set `keepalive_interval` in seconds and
optionally `keepalive_count_max` (default `3`) for networks that remove quiet
connections through NAT or firewall state expiry.

TCP probes normally originate at the WatchSSH host. For dependencies visible
only from a monitored host, declare `checks.ports[].source: target` together
with its `host` and `port`. WatchSSH opens an SSH `direct-tcpip` channel from
that target to the dependency, so no `nc`, `socat`, shell command, or target
side package is required.

## Deployment Checklist

1. Run the service under a dedicated non-login user.
2. Store configuration and secrets outside the application directory.
3. Keep the HTTP listener private or behind TLS and authentication.
4. Point a load balancer liveness check to `/healthz` or `/livez`.
5. Point a traffic/readiness check to `/readyz`.
6. Scrape `/metrics` with the same credentials or trusted network policy used
   for the dashboard.
7. Import `/openapi.json` into the API tooling used by your team.
8. Alert when `watchssh_ready` is `0`, a target's `watchssh_up` is `0`, or the
   latest collection timestamp becomes stale.
