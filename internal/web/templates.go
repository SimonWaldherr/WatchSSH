package web

// css is the shared stylesheet served at /static/style.css.
const css = `
*,*::before,*::after{box-sizing:border-box}
body{margin:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f0f2f5;color:#333;font-size:14px}
a{color:#0066cc;text-decoration:none}a:hover{text-decoration:underline}
header{background:#1c2b4a;color:#fff;padding:0 1.5rem;display:flex;align-items:center;height:52px;gap:2rem}
header h1{font-size:1.1rem;font-weight:700;margin:0;white-space:nowrap}
header nav{display:flex;gap:.25rem}
header nav a{color:#a8bdd9;padding:.35rem .75rem;border-radius:4px;font-size:.85rem;white-space:nowrap}
header nav a:hover,header nav a.active{background:#2d4a7a;color:#fff;text-decoration:none}
main{padding:1.5rem;max-width:1400px;margin:0 auto}
h2{font-size:1.05rem;margin:0 0 1rem;color:#444}
h3{font-size:.95rem;margin:0 0 .75rem;color:#555}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(290px,1fr));gap:1rem;margin-bottom:1.5rem}
.card{background:#fff;border-radius:8px;box-shadow:0 1px 4px rgba(0,0,0,.1);overflow:hidden}
.card-head{padding:.65rem 1rem;display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid #eee}
.card-title{font-weight:600;font-size:.9rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.card-body{padding:.9rem 1rem}
.card-foot{padding:.45rem 1rem;background:#fafafa;border-top:1px solid #eee;font-size:.78rem;color:#888;display:flex;justify-content:space-between;align-items:center}
.badge{display:inline-block;padding:.12rem .45rem;border-radius:10px;font-size:.7rem;font-weight:700;color:#fff;letter-spacing:.02em;white-space:nowrap}
.badge-ok{background:#22863a}.badge-warn{background:#b08800}.badge-error{background:#cb2431}.badge-unknown{background:#888}
.m-row{display:flex;justify-content:space-between;margin-bottom:.3rem;font-size:.82rem}
.m-label{color:#666}.m-val{font-weight:600;font-variant-numeric:tabular-nums}
.pbar-wrap{background:#e9ecef;border-radius:3px;height:5px;margin:.15rem 0 .4rem;overflow:hidden}
.pbar{height:100%;border-radius:3px;background:#22863a;transition:width .3s}
.pbar.warn{background:#b08800}.pbar.error{background:#cb2431}
table{width:100%;border-collapse:collapse;font-size:.84rem}
th{text-align:left;padding:.45rem .75rem;background:#f6f8fa;color:#555;font-weight:600;border-bottom:2px solid #e1e4e8;white-space:nowrap}
td{padding:.4rem .75rem;border-bottom:1px solid #eee;vertical-align:middle}
tr:hover td{background:#fafbfc}
.btn{display:inline-block;padding:.32rem .7rem;border-radius:5px;border:1px solid transparent;cursor:pointer;font-size:.82rem;font-weight:500;text-align:center;line-height:1.4}
.btn-primary{background:#0066cc;border-color:#0055bb;color:#fff}.btn-primary:hover{background:#0055bb;color:#fff;text-decoration:none}
.btn-danger{background:#cb2431;border-color:#a51c26;color:#fff}.btn-danger:hover{background:#a51c26;color:#fff;text-decoration:none}
.btn-secondary{background:#fff;border-color:#d1d5da;color:#444}.btn-secondary:hover{background:#f3f4f6;color:#333;text-decoration:none}
.btn-sm{padding:.18rem .5rem;font-size:.76rem}
.form-wrap{background:#fff;border-radius:8px;box-shadow:0 1px 4px rgba(0,0,0,.1);padding:1.25rem;margin-top:1.25rem}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:.75rem;margin-bottom:.75rem}
.form-row.w3{grid-template-columns:1fr 1fr 1fr}
.form-row.wide{grid-template-columns:1fr}
label{display:block;font-size:.8rem;color:#555;margin-bottom:.25rem;font-weight:500}
input[type=text],input[type=number],input[type=password],input[type=email],select{width:100%;padding:.38rem .6rem;border:1px solid #d1d5da;border-radius:5px;font-size:.85rem;background:#fff}
input:focus,select:focus{outline:none;border-color:#0066cc;box-shadow:0 0 0 2px rgba(0,102,204,.15)}
.form-actions{margin-top:1rem;display:flex;gap:.5rem;align-items:center}
.firing-item{background:#fff;border-left:4px solid #cb2431;border-radius:4px;padding:.65rem 1rem;margin-bottom:.5rem;box-shadow:0 1px 3px rgba(0,0,0,.07)}
.firing-item .msg{font-weight:500;font-size:.88rem}
.firing-item .ts{font-size:.77rem;color:#888;margin-top:.15rem}
.notice{border-radius:5px;padding:.55rem 1rem;font-size:.84rem;margin-bottom:1rem}
.notice-ok{background:#dff0d8;border:1px solid #a3d6a3}
.notice-err{background:#f8d7da;border:1px solid #f5c6cb}
.section{background:#fff;border-radius:8px;box-shadow:0 1px 4px rgba(0,0,0,.1);padding:1rem;margin-bottom:1rem}
.section h3{font-size:.88rem;color:#555;margin:0 0 .65rem;padding-bottom:.45rem;border-bottom:1px solid #eee}
.detail-grid{display:grid;grid-template-columns:1fr 1fr;gap:1rem}
.dot{display:inline-block;width:9px;height:9px;border-radius:50%;vertical-align:middle;margin-right:4px}
.dot-ok{background:#22863a}.dot-err{background:#cb2431}.dot-unk{background:#888}
.tag{display:inline-block;padding:.1rem .4rem;border-radius:3px;font-size:.72rem;font-weight:600;margin-left:.3rem}
.tag-ok{background:#dff0d8;color:#22863a}.tag-err{background:#f8d7da;color:#cb2431}
.empty{color:#888;font-size:.87rem;padding:1rem 0}
@media(max-width:650px){.detail-grid{grid-template-columns:1fr}.form-row,.form-row.w3{grid-template-columns:1fr}}
`

// allTemplates is parsed once at startup into the global template set.
// Each named template renders a complete HTML page using the shared "hdr"
// and "ftr" partials.
const allTemplates = `
{{define "hdr"}}<!DOCTYPE html>
<html lang="en"><head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} — WatchSSH</title>
<link rel="stylesheet" href="/static/style.css">
{{if .Refresh}}<meta http-equiv="refresh" content="30">{{end}}
</head><body>
<header>
  <h1>🖥 WatchSSH</h1>
  <nav>
    <a href="/" {{if eq .Page "dashboard"}}class="active"{{end}}>Dashboard</a>
    <a href="/servers" {{if eq .Page "servers"}}class="active"{{end}}>Servers</a>
    <a href="/alerts" {{if eq .Page "alerts"}}class="active"{{end}}>Alerts</a>
    <a href="/config" {{if eq .Page "config"}}class="active"{{end}}>Configuration</a>
  </nav>
</header>
<main>
{{end}}

{{define "ftr"}}</main>
<script>
// Auto-refresh countdown
(function(){
  if(!document.querySelector('meta[http-equiv=refresh]')) return;
  var sec=30;
  var el=document.getElementById('refresh-count');
  if(!el) return;
  setInterval(function(){el.textContent=(--sec>0?sec:'…');},1000);
})();
</script>
</body></html>{{end}}

{{define "dashboard"}}
{{template "hdr" .}}
<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
  <h2 style="margin:0">Servers ({{len .Servers}})</h2>
  <span style="font-size:.8rem;color:#888">Auto-refresh in <span id="refresh-count">30</span>s</span>
</div>
{{if .Flash}}<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>{{end}}
<div class="grid">
{{range .Servers}}
  <div class="card">
    <div class="card-head">
      <span class="card-title" title="{{.ServerName}}">{{.ServerName}}</span>
      <span class="badge badge-{{serverStatus .}}">{{serverStatusLabel .}}</span>
    </div>
    <div class="card-body">
      {{if .Error}}
        <p style="color:#cb2431;font-size:.84rem;margin:0">{{.Error}}</p>
      {{else}}
        <div class="m-row"><span class="m-label">CPU</span><span class="m-val">{{if .CPU}}{{printf "%.1f" (cpuPct .)}}%{{else}}n/a{{end}}</span></div>
        {{if .CPU}}<div class="pbar-wrap"><div class="pbar {{pbarClass (cpuPct .)}}" style="width:{{clamp (cpuPct .)}}%"></div></div>{{end}}

        <div class="m-row"><span class="m-label">RAM</span><span class="m-val">{{if .Memory}}{{printf "%.1f" (memPct .)}}%{{else}}n/a{{end}}</span></div>
        {{if .Memory}}<div class="pbar-wrap"><div class="pbar {{pbarClass (memPct .)}}" style="width:{{clamp (memPct .)}}%"></div></div>{{end}}

        {{with rootDisk .Disks}}
        <div class="m-row"><span class="m-label">Disk /</span><span class="m-val">{{printf "%.1f" .UsagePercent}}%</span></div>
        <div class="pbar-wrap"><div class="pbar {{pbarClass .UsagePercent}}" style="width:{{clamp .UsagePercent}}%"></div></div>
        {{end}}

        <div class="m-row" style="margin-top:.4rem">
          <span class="m-label">Load (1m)</span><span class="m-val">{{if .Load}}{{printf "%.2f" (loadAvg1 .)}}{{else}}n/a{{end}}</span>
        </div>
        {{if .Connectivity.PingEnabled}}
        <div class="m-row">
          <span class="m-label">Ping</span>
          <span class="m-val">
            {{if .Connectivity.PingOK}}{{printf "%.1f" .Connectivity.PingLatency}} ms{{else}}<span style="color:#cb2431">FAILED</span>{{end}}
          </span>
        </div>
        {{end}}
        {{if .Connectivity.Ports}}
        <div class="m-row">
          <span class="m-label">Ports</span>
          <span class="m-val">
            {{range .Connectivity.Ports}}
              <span class="tag {{if .Open}}tag-ok{{else}}tag-err{{end}}">:{{.Port}}</span>
            {{end}}
          </span>
        </div>
        {{end}}
        {{if hasDockerDiagnostics .}}
        <div class="m-row">
          <span class="m-label">Docker</span>
          <span class="m-val">{{dockerSummary .}}</span>
        </div>
        {{end}}
      {{end}}
    </div>
    <div class="card-foot">
      <span>{{timeAgo .Timestamp}}</span>
      <a href="/server/{{.ServerName}}" class="btn btn-secondary btn-sm">Details</a>
    </div>
  </div>
{{else}}
  <p class="empty">No servers configured yet. <a href="/servers">Add one.</a></p>
{{end}}
</div>

{{if .Firings}}
<h2>Recent Alerts</h2>
{{range .Firings}}
<div class="firing-item">
  <div class="msg">{{.Message}}</div>
  <div class="ts">{{.FiredAt.Format "2006-01-02 15:04:05 MST"}}</div>
</div>
{{end}}
{{end}}
{{template "ftr" .}}
{{end}}

{{define "server-detail"}}
{{template "hdr" .}}
<div style="margin-bottom:1rem;display:flex;align-items:center;gap:1rem">
  <a href="/" class="btn btn-secondary btn-sm">← Back</a>
  <h2 style="margin:0">{{.Metrics.ServerName}}
    {{if .Metrics.Host}}<span style="font-weight:400;color:#888;font-size:.9rem">({{.Metrics.Host}})</span>{{end}}
    <span class="badge badge-{{serverStatus .Metrics}}" style="margin-left:.5rem">{{serverStatusLabel .Metrics}}</span>
  </h2>
</div>

{{if .Metrics.Error}}
  <div class="notice notice-err">Connection error: {{.Metrics.Error}}</div>
{{else}}
<div class="detail-grid">
  <div class="section">
    <h3>System</h3>
    <table><tbody>
      <tr><td class="m-label">Hostname</td><td>{{.Metrics.System.Hostname}}</td></tr>
      <tr><td class="m-label">OS</td><td>{{.Metrics.System.OS}}</td></tr>
      <tr><td class="m-label">Kernel</td><td>{{.Metrics.System.Kernel}}</td></tr>
      <tr><td class="m-label">Arch</td><td>{{.Metrics.System.Arch}}</td></tr>
      <tr><td class="m-label">Uptime</td><td>{{if .Metrics.Load}}{{fmtUptime (uptimeSecs .Metrics)}}{{else}}n/a{{end}}</td></tr>
      <tr><td class="m-label">Checked</td><td>{{.Metrics.Timestamp.Format "2006-01-02 15:04:05"}}</td></tr>
    </tbody></table>
  </div>
  <div class="section">
    <h3>CPU &amp; Load</h3>
    <table><tbody>
      {{if .Metrics.CPU}}<tr><td class="m-label">Usage</td><td>{{printf "%.1f" .Metrics.CPU.UsagePercent}}%</td></tr>
      <tr><td class="m-label">User</td><td>{{printf "%.1f" .Metrics.CPU.UserPercent}}%</td></tr>
      <tr><td class="m-label">System</td><td>{{printf "%.1f" .Metrics.CPU.SystemPercent}}%</td></tr>
      <tr><td class="m-label">I/O Wait</td><td>{{printf "%.1f" .Metrics.CPU.IOWaitPercent}}%</td></tr>{{else}}<tr><td class="m-label">CPU</td><td>n/a</td></tr>{{end}}
      {{if .Metrics.Load}}<tr><td class="m-label">Load 1m</td><td>{{printf "%.2f" .Metrics.Load.Load1}}</td></tr>
      <tr><td class="m-label">Load 5m</td><td>{{printf "%.2f" .Metrics.Load.Load5}}</td></tr>
      <tr><td class="m-label">Load 15m</td><td>{{printf "%.2f" .Metrics.Load.Load15}}</td></tr>{{else}}<tr><td class="m-label">Load</td><td>n/a</td></tr>{{end}}
    </tbody></table>
  </div>
  <div class="section">
    <h3>Memory</h3>
    <table><tbody>
      {{if .Metrics.Memory}}<tr><td class="m-label">Total</td><td>{{fmtBytes .Metrics.Memory.TotalBytes}}</td></tr>
      <tr><td class="m-label">Used</td><td>{{fmtBytes .Metrics.Memory.UsedBytes}} ({{printf "%.1f" .Metrics.Memory.UsagePercent}}%)</td></tr>
      <tr><td class="m-label">Available</td><td>{{fmtBytes .Metrics.Memory.AvailableBytes}}</td></tr>{{else}}<tr><td class="m-label">RAM</td><td>n/a</td></tr>{{end}}
      {{if .Metrics.Swap}}
      <tr><td class="m-label">Swap Total</td><td>{{fmtBytes .Metrics.Swap.TotalBytes}}</td></tr>
      <tr><td class="m-label">Swap Used</td><td>{{fmtBytes .Metrics.Swap.UsedBytes}} ({{printf "%.1f" .Metrics.Swap.Percent}}%)</td></tr>
      {{end}}
    </tbody></table>
  </div>
  <div class="section">
    <h3>Connectivity</h3>
    {{with .Metrics.Connectivity}}
    {{if .PingEnabled}}
    <div class="m-row">
      <span class="m-label">Ping</span>
      <span>
        {{if .PingOK}}<span class="dot dot-ok"></span>OK — {{printf "%.1f" .PingLatency}} ms
        {{else}}<span class="dot dot-err"></span>FAILED{{end}}
      </span>
    </div>
    {{end}}
    {{range .Ports}}
    <div class="m-row">
      <span class="m-label">Port {{.Port}}</span>
      <span>{{if .Open}}<span class="dot dot-ok"></span>Open{{else}}<span class="dot dot-err"></span>Closed{{end}}</span>
    </div>
    {{end}}
    {{range .HTTP}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label" style="word-break:break-all">{{.URL}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>{{.StatusCode}}{{else}}<span class="dot dot-err"></span>{{if .StatusCode}}{{.StatusCode}}{{else}}ERR{{end}}{{end}} — {{printf "%.0f" .LatencyMs}} ms</span>
    </div>
    {{end}}
    {{if and (not .PingEnabled) (not .Ports) (not .HTTP)}}
    <p class="empty">No connectivity checks configured for this server.</p>
    {{end}}
    {{end}}
  </div>
  {{with capabilityRows .Metrics}}
  <div class="section">
    <h3>Collector Status</h3>
    <table>
      <thead><tr><th>Metric</th><th>Status</th><th>Error</th></tr></thead>
      <tbody>
      {{range .}}
      <tr>
        <td><code>{{.Name}}</code></td>
        <td><span class="badge badge-{{statusClass .Status}}">{{.Status}}</span></td>
        <td>{{if .Error}}<span style="font-family:monospace;font-size:.8rem;word-break:break-all">{{.Error}}</span>{{else}}—{{end}}</td>
      </tr>
      {{end}}
      </tbody>
    </table>
  </div>
  {{end}}
</div>

{{if .Metrics.Disks}}
<div class="section">
  <h3>Disk Usage</h3>
  <table>
    <thead><tr><th>Device</th><th>Mount</th><th>Used</th><th>Total</th><th>Usage</th></tr></thead>
    <tbody>
    {{range .Metrics.Disks}}
    <tr>
      <td>{{.Device}}</td>
      <td>{{.MountPoint}}</td>
      <td>{{fmtBytes .UsedBytes}}</td>
      <td>{{fmtBytes .TotalBytes}}</td>
      <td>
        <div style="display:flex;align-items:center;gap:.5rem">
          <div class="pbar-wrap" style="width:80px;margin:0"><div class="pbar {{pbarClass .UsagePercent}}" style="width:{{clamp .UsagePercent}}%"></div></div>
          {{printf "%.1f" .UsagePercent}}%
        </div>
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .Metrics.Network}}
<div class="section">
  <h3>Network Interfaces</h3>
  <table>
    <thead><tr><th>Interface</th><th>Received</th><th>Sent</th><th>Packets In</th><th>Packets Out</th></tr></thead>
    <tbody>
    {{range .Metrics.Network}}
    {{if or .BytesRecv .BytesSent}}
    <tr>
      <td>{{.Interface}}</td>
      <td>{{fmtBytes .BytesRecv}}</td>
      <td>{{fmtBytes .BytesSent}}</td>
      <td>{{.PacketsRecv}}</td>
      <td>{{.PacketsSent}}</td>
    </tr>
    {{end}}
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if hasDockerDiagnostics .Metrics}}
<div class="section">
  <h3>Docker Containers</h3>
  <div class="m-row">
    <span class="m-label">Collector</span>
    <span>{{dockerSummary .Metrics}}</span>
  </div>
  {{with metricError .Metrics "containers"}}
  <div class="notice notice-err" style="margin-top:.75rem">Docker collector error: {{.}}</div>
  {{end}}
  {{if .Metrics.Containers}}
  <table style="margin-top:.75rem">
    <thead><tr><th>Name</th><th>Image</th><th>Status</th><th>CPU%</th><th>Memory</th><th>Network</th><th>Block I/O</th></tr></thead>
    <tbody>
    {{range .Metrics.Containers}}
    <tr>
      <td>{{.Name}}</td>
      <td style="font-family:monospace;font-size:.8rem;word-break:break-all">{{.Image}}</td>
      <td>{{.Status}}</td>
      <td>{{printf "%.1f" .CPUPercent}}</td>
      <td>{{fmtBytes .MemUsedBytes}} / {{fmtBytes .MemLimitBytes}} ({{printf "%.1f" .MemPercent}}%)</td>
      <td>{{fmtBytes .NetRxBytes}} ↓ / {{fmtBytes .NetTxBytes}} ↑</td>
      <td>{{fmtBytes .BlockReadBytes}} read / {{fmtBytes .BlockWriteBytes}} write</td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{else if eq (metricCapability .Metrics "containers") "ok"}}
  <p class="empty">No running containers were found during this poll.</p>
  {{else if not (metricError .Metrics "containers")}}
  <p class="empty">Docker metrics are not available on this target right now.</p>
  {{end}}
</div>
{{end}}

{{if .Metrics.CustomChecks}}
<div class="section">
  <h3>Custom Checks</h3>
  <table>
    <thead><tr><th>Check</th><th>Status</th><th>Output</th></tr></thead>
    <tbody>
    {{range .Metrics.CustomChecks}}
    <tr>
      <td>{{.Name}}</td>
      <td>{{if .OK}}<span class="dot dot-ok"></span>OK{{else}}<span class="dot dot-err"></span>FAILED{{end}}</td>
      <td style="font-family:monospace;font-size:.8rem;word-break:break-all">{{.Output}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .Metrics.Processes}}
<div class="section">
  <h3>Top Processes (by CPU)</h3>
  <table>
    <thead><tr><th>PID</th><th>User</th><th>CPU%</th><th>MEM%</th><th>Command</th></tr></thead>
    <tbody>
    {{range .Metrics.Processes}}
    <tr>
      <td>{{.PID}}</td>
      <td>{{.User}}</td>
      <td>{{printf "%.1f" .CPUPercent}}</td>
      <td>{{printf "%.1f" .MemPercent}}</td>
      <td style="font-family:monospace;font-size:.8rem;word-break:break-all">{{.Command}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}
{{end}}
{{template "ftr" .}}
{{end}}

{{define "servers-manage"}}
{{template "hdr" .}}
<h2>Server Management</h2>
{{if .Flash}}<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>{{end}}

<div class="section">
  <h3>Configured Servers ({{len .Servers}})</h3>
  {{if .Servers}}
  <table>
    <thead><tr><th>Name</th><th>Host</th><th>Port</th><th>User</th><th>Type</th><th>Status</th><th></th></tr></thead>
    <tbody>
    {{range .Servers}}
    <tr>
      <td><a href="/server/{{.ServerName}}">{{.ServerName}}</a></td>
      <td>{{if .Host}}{{.Host}}{{else}}<em>local</em>{{end}}</td>
      <td>{{if not .Host}}—{{else}}{{.Port}}{{end}}</td>
      <td>{{if .Username}}{{.Username}}{{else}}—{{end}}</td>
      <td>{{if not .Host}}local{{else}}SSH{{end}}</td>
      <td><span class="badge badge-{{serverStatus .ServerMetrics}}">{{serverStatusLabel .ServerMetrics}}</span></td>
      <td>
        <form method="post" action="/servers/remove" style="display:inline">
          <input type="hidden" name="name" value="{{.ServerName}}">
          <button type="submit" class="btn btn-danger btn-sm"
            onclick="return confirm('Remove {{.ServerName}}?')">Remove</button>
        </form>
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <p class="empty">No servers configured yet.</p>
  {{end}}
</div>

<div class="form-wrap">
  <h3>Add Server</h3>
  <form method="post" action="/servers/add">
    <div class="form-row">
      <div><label>Name *</label><input type="text" name="name" placeholder="web-01" required></div>
      <div><label>Host / IP *</label><input type="text" name="host" placeholder="192.168.1.10"></div>
    </div>
    <div class="form-row">
      <div><label>SSH Port</label><input type="number" name="port" value="22" min="1" max="65535"></div>
      <div><label>Username</label><input type="text" name="username" placeholder="monitor"></div>
    </div>
    <div class="form-row">
      <div>
        <label>Auth Type</label>
        <select name="auth_type">
          <option value="key">Private Key</option>
          <option value="password">Password</option>
          <option value="agent">SSH Agent</option>
        </select>
      </div>
      <div><label>Key File / Password</label><input type="text" name="auth_credential" placeholder="~/.ssh/id_ed25519"></div>
    </div>
    <div class="form-row">
      <div>
        <label>
          <input type="checkbox" name="local" value="1">
          Monitor this machine locally (no SSH)
        </label>
      </div>
      <div>
        <label>
          <input type="checkbox" name="ping" value="1">
          Enable ping check
        </label>
      </div>
    </div>
    <div class="form-actions">
      <button type="button" class="btn btn-secondary" id="btn-test-conn">Test Connection</button>
      <button type="submit" class="btn btn-primary">Add Server</button>
      <span id="test-conn-result" style="font-size:.83rem;margin-left:.5rem"></span>
    </div>
  </form>
</div>
<script>
(function(){
  var btn = document.getElementById('btn-test-conn');
  var result = document.getElementById('test-conn-result');
  if(!btn) return;
  btn.addEventListener('click', function(){
    var form = btn.closest('form');
    var port = parseInt(form.querySelector('[name=port]').value, 10) || 22;
    var body = JSON.stringify({
      host:       form.querySelector('[name=host]').value.trim(),
      port:       port,
      username:   form.querySelector('[name=username]').value.trim(),
      auth_type:  form.querySelector('[name=auth_type]').value,
      credential: form.querySelector('[name=auth_credential]').value.trim(),
      local:      form.querySelector('[name=local]').checked
    });
    btn.disabled = true;
    result.style.color = '#888';
    result.textContent = 'Testing…';
    fetch('/api/test-connection', {method:'POST', headers:{'Content-Type':'application/json'}, body:body})
      .then(function(r){ return r.json(); })
      .then(function(data){
        result.style.color = data.ok ? '#22863a' : '#cb2431';
        result.textContent = (data.ok ? '✓ ' : '✗ ') + data.message;
      })
      .catch(function(e){ result.style.color='#cb2431'; result.textContent='Request failed: '+e; })
      .finally(function(){ btn.disabled = false; });
  });
})();
</script>
{{template "ftr" .}}
{{end}}

{{define "alerts-page"}}
{{template "hdr" .}}
<h2>Alert Management</h2>
{{if .Flash}}<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>{{end}}

<div class="section">
  <h3>Recent Firings (last {{len .Firings}})</h3>
  {{if .Firings}}
  {{range .Firings}}
  <div class="firing-item">
    <div class="msg">{{.Message}}</div>
    <div class="ts">{{.FiredAt.Format "2006-01-02 15:04:05 MST"}} — server: {{.Server}}</div>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No alerts have fired yet.</p>
  {{end}}
</div>

<div class="section">
  <h3>Alert Rules ({{len .Rules}})</h3>
  {{if .Rules}}
  <table>
    <thead><tr><th>Name</th><th>Metric</th><th>Condition</th><th>Threshold</th><th>Servers</th><th></th></tr></thead>
    <tbody>
    {{range .Rules}}
    <tr>
      <td>{{.Name}}</td>
      <td><code>{{.Metric}}</code></td>
      <td>{{.Operator}}</td>
      <td>{{.Threshold}}</td>
      <td>{{if .Servers}}{{range .Servers}}{{.}} {{end}}{{else}}<em>all</em>{{end}}</td>
      <td>
        <form method="post" action="/alerts/remove" style="display:inline">
          <input type="hidden" name="name" value="{{.Name}}">
          <button type="submit" class="btn btn-danger btn-sm"
            onclick="return confirm('Remove rule {{.Name}}?')">Remove</button>
        </form>
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <p class="empty">No alert rules defined yet.</p>
  {{end}}
</div>

<div class="form-wrap">
  <h3>Add Alert Rule</h3>
  <form method="post" action="/alerts/add">
    <div class="form-row">
      <div><label>Rule Name *</label><input type="text" name="name" placeholder="High CPU" required></div>
      <div>
        <label>Metric *</label>
        <select name="metric" required>
          <option value="cpu_usage">cpu_usage (%)</option>
          <option value="mem_usage">mem_usage (%)</option>
          <option value="swap_usage">swap_usage (%)</option>
          <option value="disk_usage">disk_usage (%)</option>
          <option value="load1">load1</option>
          <option value="load5">load5</option>
          <option value="load15">load15</option>
          <option value="ping_failed">ping_failed</option>
          <option value="ping_latency">ping_latency (ms)</option>
          <option value="port_closed">port_closed</option>
          <option value="http_failed">http_failed</option>
          <option value="cert_expires_days">cert_expires_days (days)</option>
          <option value="custom_failed">custom_failed</option>
        </select>
      </div>
    </div>
    <div class="form-row w3">
      <div>
        <label>Operator</label>
        <select name="operator">
          <option value=">">&gt;</option>
          <option value=">=">&gt;=</option>
          <option value="<">&lt;</option>
          <option value="<=">&lt;=</option>
          <option value="==">==</option>
          <option value="!=">!=</option>
        </select>
      </div>
      <div><label>Threshold</label><input type="number" name="threshold" step="any" placeholder="90"></div>
      <div><label>Mount Point (disk only)</label><input type="text" name="mount_point" placeholder="/"></div>
    </div>
    <div class="form-row">
      <div>
        <label>Limit to Servers <small style="color:#888">(hold Ctrl/⌘ for multi-select; none selected = all servers)</small></label>
        {{if $.ServerNames}}
        <select name="servers" multiple size="4" style="height:auto">
          {{range $.ServerNames}}<option value="{{.}}">{{.}}</option>{{end}}
        </select>
        {{else}}
        <input type="text" name="servers_text" placeholder="web-01, db-01 (empty = all)">
        {{end}}
      </div>
      <div><label>Port (port_closed only)</label><input type="number" name="port" placeholder="80" min="1" max="65535"></div>
    </div>
    <div class="form-actions">
      <button type="submit" class="btn btn-primary">Add Rule</button>
    </div>
  </form>
</div>

{{with .EmailCfg}}
<div class="section" style="margin-top:1.25rem">
  <h3>Email Notification Settings</h3>
  <table><tbody>
    <tr><td class="m-label">SMTP Host</td><td>{{.SMTPHost}}:{{.SMTPPort}}</td></tr>
    <tr><td class="m-label">TLS Mode</td><td>{{.TLSMode}}</td></tr>
    <tr><td class="m-label">From</td><td>{{.From}}</td></tr>
    <tr><td class="m-label">To</td><td>{{range .To}}{{.}} {{end}}</td></tr>
  </tbody></table>
  <p style="margin:.75rem 0 0;font-size:.8rem;color:#888">Email settings are configured in the YAML config file under <code>alerts.email</code>.</p>
</div>
{{end}}
{{template "ftr" .}}
{{end}}

{{define "config-page"}}
{{template "hdr" .}}
<h2>Configuration</h2>
{{if .Flash}}
<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>
{{end}}
<form method="post" action="/config">

  <div class="form-wrap" style="margin-top:0">
    <h3>Polling &amp; Timing</h3>
    <div class="form-row w3">
      <div>
        <label>Poll Interval (seconds)</label>
        <input type="number" name="interval" value="{{.Config.Interval}}" min="5" required>
      </div>
      <div>
        <label>SSH Timeout (seconds)</label>
        <input type="number" name="timeout" value="{{.Config.Timeout}}" min="1" required>
      </div>
      <div>
        <label>Max Concurrent Workers <small style="color:#888">(0 = unlimited)</small></label>
        <input type="number" name="workers" value="{{.Config.Workers}}" min="0">
      </div>
    </div>
  </div>

  <div class="form-wrap">
    <h3>Output</h3>
    <div class="form-row">
      <div>
        <label>Output Type</label>
        <select name="output_type">
          <option value="console" {{if eq .Config.Output.Type "console"}}selected{{end}}>console</option>
          <option value="json" {{if eq .Config.Output.Type "json"}}selected{{end}}>json</option>
        </select>
      </div>
      <div>
        <label>JSON Output File <small style="color:#888">(leave blank for stdout)</small></label>
        <input type="text" name="output_file" value="{{.Config.Output.File}}" placeholder="/var/log/watchssh/metrics.json">
      </div>
    </div>
  </div>

  <div class="form-wrap">
    <h3>Web Dashboard</h3>
    <p style="font-size:.82rem;color:#888;margin:0 0 .75rem">Changes to the listen address require a restart to take effect.</p>
    <div class="form-row">
      <div>
        <label>Enable Web Dashboard</label>
        <select name="web_enabled">
          <option value="1" {{if .Config.Web.Enabled}}selected{{end}}>Enabled</option>
          <option value="0" {{if not .Config.Web.Enabled}}selected{{end}}>Disabled</option>
        </select>
      </div>
      <div>
        <label>Listen Address</label>
        <input type="text" name="web_listen" value="{{.Config.Web.Listen}}" placeholder=":8080">
      </div>
    </div>
  </div>

  <div class="form-wrap">
    <h3>SSH Security</h3>
    <div class="form-row">
      <div>
        <label>Known Hosts File <small style="color:#888">(blank = ~/.ssh/known_hosts)</small></label>
        <input type="text" name="known_hosts_path" value="{{.Config.KnownHostsPath}}" placeholder="~/.ssh/known_hosts">
      </div>
      <div>
        <label>Strict Host Key Checking</label>
        <select name="strict_host_key_checking">
          <option value="true" {{if derefBool .Config.StrictHostKeyChecking true}}selected{{end}}>Enabled (recommended)</option>
          <option value="false" {{if not (derefBool .Config.StrictHostKeyChecking true)}}selected{{end}}>Disabled (insecure)</option>
        </select>
      </div>
    </div>
  </div>

  <div class="form-actions">
    <button type="submit" class="btn btn-primary">Save Configuration</button>
    <a href="/" class="btn btn-secondary">Cancel</a>
  </div>
</form>
{{template "ftr" .}}
{{end}}
`
