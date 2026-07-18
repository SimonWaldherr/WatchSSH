package web

// css is the shared stylesheet served at /static/style.css.
const css = `
:root{--bg:#f4f6f8;--surface:#fff;--surface-alt:#f7f8fa;--text:#27313d;--text-muted:#666;--text-faint:#888;--text-subtle:#71808e;--border:#e5e9ed;--border-strong:#d1d5da;--link:#0066cc;--link-hover:#0055bb;--accent:#1b8a6b;--accent-soft-bg:#edf9f5;--accent-soft-text:#12674f;--accent-ring:rgba(27,138,107,.12);--shadow:rgba(0,0,0,.1);--header-bg:#18283b;--header-text:#fff;--header-nav-text:#a8bdd9;--header-nav-hover-bg:#2d4a7a;--ok-text:#22863a;--ok-soft-bg:#dff0d8;--ok-soft-border:#a3d6a3;--warn-text:#9b6700;--error-text:#cb2431;--error-soft-bg:#f8d7da;--error-soft-border:#f5c6cb;--info-soft-bg:#e7f1fb;--info-soft-border:#b9d8f5;--info-text:#254e77;--input-bg:#fff;--track-bg:#e9ecef;--pill-bg:#eef2f7;--pill-text:#445;--focus-ring:rgba(0,102,204,.15)}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#0d1420;--surface:#161f2c;--surface-alt:#111a27;--text:#dbe4ee;--text-muted:#9aa8b8;--text-faint:#7c8fa1;--text-subtle:#8a9aac;--border:#263242;--border-strong:#374a5e;--link:#5eb0ff;--link-hover:#8cc5ff;--accent:#2fbf94;--accent-soft-bg:#102a23;--accent-soft-text:#6fd8b8;--accent-ring:rgba(47,191,148,.2);--shadow:rgba(0,0,0,.5);--header-bg:#0a121e;--header-text:#fff;--header-nav-text:#9fb3cc;--header-nav-hover-bg:#24405f;--ok-text:#3fb950;--ok-soft-bg:#0f2b1a;--ok-soft-border:#1f5c34;--warn-text:#d29922;--error-text:#f85149;--error-soft-bg:#3c1418;--error-soft-border:#6e2630;--info-soft-bg:#0f2740;--info-soft-border:#1f4a68;--info-text:#9cc7ea;--input-bg:#101823;--track-bg:#202b3a;--pill-bg:#1c2836;--pill-text:#b7c3cf;--focus-ring:rgba(94,176,255,.25)}}
:root[data-theme="dark"]{--bg:#0d1420;--surface:#161f2c;--surface-alt:#111a27;--text:#dbe4ee;--text-muted:#9aa8b8;--text-faint:#7c8fa1;--text-subtle:#8a9aac;--border:#263242;--border-strong:#374a5e;--link:#5eb0ff;--link-hover:#8cc5ff;--accent:#2fbf94;--accent-soft-bg:#102a23;--accent-soft-text:#6fd8b8;--accent-ring:rgba(47,191,148,.2);--shadow:rgba(0,0,0,.5);--header-bg:#0a121e;--header-text:#fff;--header-nav-text:#9fb3cc;--header-nav-hover-bg:#24405f;--ok-text:#3fb950;--ok-soft-bg:#0f2b1a;--ok-soft-border:#1f5c34;--warn-text:#d29922;--error-text:#f85149;--error-soft-bg:#3c1418;--error-soft-border:#6e2630;--info-soft-bg:#0f2740;--info-soft-border:#1f4a68;--info-text:#9cc7ea;--input-bg:#101823;--track-bg:#202b3a;--pill-bg:#1c2836;--pill-text:#b7c3cf;--focus-ring:rgba(94,176,255,.25)}
*,*::before,*::after{box-sizing:border-box}
body{margin:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--text);font-size:14px;line-height:1.45}
a{color:var(--link);text-decoration:none}a:hover{text-decoration:underline}
.skip-link{position:absolute;left:-9999px;top:.5rem;z-index:10;background:var(--surface);color:var(--text);border:2px solid var(--accent);border-radius:4px;padding:.4rem .65rem;font-weight:600}.skip-link:focus{left:.75rem}
header{background:var(--header-bg);color:var(--header-text);padding:0 1.5rem;display:flex;align-items:center;height:56px;gap:2rem;border-bottom:3px solid var(--accent)}
header h1{font-size:1.1rem;font-weight:700;margin:0;white-space:nowrap}
header nav{display:flex;gap:.25rem;flex:1}
header nav a{color:var(--header-nav-text);padding:.35rem .75rem;border-radius:4px;font-size:.85rem;white-space:nowrap}
header nav a:hover,header nav a.active{background:var(--header-nav-hover-bg);color:var(--header-text);text-decoration:none}
.interface-controls{display:flex;align-items:center;gap:.7rem}.mode-picker{display:flex;align-items:center;gap:.4rem;font-size:.76rem;color:var(--header-nav-text);white-space:nowrap}.mode-picker select{width:auto;background:var(--header-nav-hover-bg);border-color:#49617b;color:#fff;padding:.25rem .45rem;font-size:.78rem}
main{padding:1.5rem;max-width:1400px;margin:0 auto}
h2{font-size:1.15rem;margin:0 0 .25rem;color:var(--text)}
h3{font-size:.95rem;margin:0 0 .75rem;color:var(--text-muted)}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(290px,1fr));gap:1rem;margin-bottom:1.5rem}
.card{background:var(--surface);border-radius:8px;box-shadow:0 1px 4px var(--shadow);overflow:hidden}
.card-head{padding:.65rem 1rem;display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid var(--border)}
.card-title{font-weight:600;font-size:.9rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.card-body{padding:.9rem 1rem}
.card-foot{padding:.45rem 1rem;background:var(--surface-alt);border-top:1px solid var(--border);font-size:.78rem;color:var(--text-faint);display:flex;justify-content:space-between;align-items:center}
.badge{display:inline-block;padding:.12rem .45rem;border-radius:10px;font-size:.7rem;font-weight:700;color:#fff;letter-spacing:.02em;white-space:nowrap}
.badge-ok{background:#22863a}.badge-warn{background:#b08800}.badge-error{background:#cb2431}.badge-unknown{background:#888}
.m-row{display:flex;justify-content:space-between;margin-bottom:.3rem;font-size:.82rem}
.m-label{color:var(--text-muted)}.m-val{font-weight:600;font-variant-numeric:tabular-nums}
.pbar-wrap{background:var(--track-bg);border-radius:3px;height:5px;margin:.15rem 0 .4rem;overflow:hidden}
.pbar{height:100%;border-radius:3px;background:#22863a;transition:width .3s}
.pbar.warn{background:#b08800}.pbar.error{background:#cb2431}
table{width:100%;border-collapse:collapse;font-size:.84rem}
th{text-align:left;padding:.45rem .75rem;background:var(--surface-alt);color:var(--text-muted);font-weight:600;border-bottom:2px solid var(--border-strong);white-space:nowrap}
td{padding:.4rem .75rem;border-bottom:1px solid var(--border);vertical-align:middle}
tr:hover td{background:var(--surface-alt)}
.btn{display:inline-block;padding:.32rem .7rem;border-radius:5px;border:1px solid transparent;cursor:pointer;font-size:.82rem;font-weight:500;text-align:center;line-height:1.4}
.btn-primary{background:#0066cc;border-color:#0055bb;color:#fff}.btn-primary:hover{background:#0055bb;color:#fff;text-decoration:none}
.btn-danger{background:#cb2431;border-color:#a51c26;color:#fff}.btn-danger:hover{background:#a51c26;color:#fff;text-decoration:none}
.btn-secondary{background:var(--surface);border-color:var(--border-strong);color:var(--text)}.btn-secondary:hover{background:var(--surface-alt);color:var(--text);text-decoration:none}
.btn-sm{padding:.18rem .5rem;font-size:.76rem}
.form-wrap{background:var(--surface);border-radius:8px;box-shadow:0 1px 4px var(--shadow);padding:1.25rem;margin-top:1.25rem}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:.75rem;margin-bottom:.75rem}
.form-row.w3{grid-template-columns:1fr 1fr 1fr}
.form-row.wide{grid-template-columns:1fr}
.form-grow{grid-column:span 2}
.form-block{border-top:1px solid var(--border);margin-top:1rem;padding-top:1rem}
.form-block h4{font-size:.82rem;color:var(--text-muted);margin:0 0 .75rem}
.probe-details{border-top:1px solid var(--border);margin-top:.8rem;padding-top:.8rem}.probe-details summary{cursor:pointer;color:var(--link);font-size:.82rem;font-weight:600;user-select:none}.probe-details[open] summary{margin-bottom:.85rem}.probe-details summary:hover{color:var(--link-hover)}.m-error{width:100%;font-family:monospace;font-size:.72rem;color:var(--error-text);word-break:break-word;margin-top:.15rem}
.inline-check{display:flex;align-items:center;gap:.35rem;font-size:.82rem;color:var(--text-muted);margin:.45rem 0}
label{display:block;font-size:.8rem;color:var(--text-muted);margin-bottom:.25rem;font-weight:500}
input[type=text],input[type=number],input[type=password],input[type=email],select{width:100%;padding:.38rem .6rem;border:1px solid var(--border-strong);border-radius:5px;font-size:.85rem;background:var(--input-bg);color:var(--text)}
input:focus,select:focus{outline:none;border-color:var(--link);box-shadow:0 0 0 2px var(--focus-ring)}
.form-actions{margin-top:1rem;display:flex;gap:.5rem;align-items:center}
.firing-item{background:var(--surface);border-left:4px solid #cb2431;border-radius:4px;padding:.65rem 1rem;margin-bottom:.5rem;box-shadow:0 1px 3px var(--shadow)}
.firing-item .msg{font-weight:500;font-size:.88rem}
.firing-item .ts{font-size:.77rem;color:var(--text-faint);margin-top:.15rem}
.notice{border-radius:5px;padding:.55rem 1rem;font-size:.84rem;margin-bottom:1rem}
.notice-ok{background:var(--ok-soft-bg);border:1px solid var(--ok-soft-border)}
.notice-err{background:var(--error-soft-bg);border:1px solid var(--error-soft-border)}
.notice-info{background:var(--info-soft-bg);border:1px solid var(--info-soft-border);color:var(--info-text)}
.section{background:var(--surface);border-radius:8px;box-shadow:0 1px 4px var(--shadow);padding:1rem;margin-bottom:1rem}
.section h3{font-size:.88rem;color:var(--text-muted);margin:0 0 .65rem;padding-bottom:.45rem;border-bottom:1px solid var(--border)}
.detail-grid{display:grid;grid-template-columns:1fr 1fr;gap:1rem}
.dot{display:inline-block;width:9px;height:9px;border-radius:50%;vertical-align:middle;margin-right:4px}
.dot-ok{background:var(--ok-text)}.dot-err{background:var(--error-text)}.dot-unk{background:var(--text-faint)}
.tag{display:inline-block;padding:.1rem .4rem;border-radius:3px;font-size:.72rem;font-weight:600;margin-left:.3rem}
.tag-ok{background:var(--ok-soft-bg);color:var(--ok-text)}.tag-err{background:var(--error-soft-bg);color:var(--error-text)}
.pill{display:inline-block;background:var(--pill-bg);color:var(--pill-text);padding:.08rem .38rem;border-radius:3px;font-size:.72rem;margin:.05rem .15rem .05rem 0}
.empty{color:var(--text-faint);font-size:.87rem;padding:1rem 0}
.table-scroll{overflow-x:auto}
.text-ok{color:var(--ok-text)}.text-error{color:var(--error-text)}.text-faint{color:var(--text-faint)}
.page-intro{display:flex;justify-content:space-between;align-items:flex-end;gap:1rem;margin-bottom:1.1rem}.page-intro p{margin:0;color:var(--text-subtle);font-size:.86rem;max-width:58rem}
.setup-steps{display:grid;grid-template-columns:repeat(3,1fr);gap:.5rem;margin:0 0 1rem}.setup-step{border:1px solid var(--border);border-radius:5px;padding:.55rem .65rem;background:var(--surface-alt);font-size:.78rem;color:var(--text-subtle)}.setup-step strong{display:block;color:var(--text);font-size:.8rem}.setup-step.active{border-color:var(--accent);background:var(--accent-soft-bg)}.setup-step.active strong{color:var(--accent-soft-text)}
.profile-note{margin:.5rem 0 0;padding:.55rem .65rem;border-left:3px solid var(--accent);background:var(--accent-soft-bg);color:var(--accent-soft-text);font-size:.8rem}.profile-note code{font-size:.78rem}
.form-section-title{display:flex;align-items:baseline;justify-content:space-between;gap:1rem;margin-bottom:.8rem}.form-section-title h3{margin:0}.form-section-title span{color:var(--text-subtle);font-size:.78rem}
.probe-builder{border-top:1px solid var(--border);padding-top:1rem}.probe-transfer{display:grid;grid-template-columns:1fr 1fr;gap:.75rem;margin-top:1rem;padding-top:1rem;border-top:1px solid var(--border)}.inline-form{display:grid;grid-template-columns:auto minmax(0,1fr) auto auto;align-items:end;gap:.45rem}.inline-form label{margin:0;white-space:nowrap}.inline-form input[type=file]{min-width:0;font-size:.76rem}
.restart-note{font-size:.8rem;color:var(--text-subtle);margin:.2rem 0 0}.restart-note strong{color:var(--warn-text)}
.config-summary{display:grid;grid-template-columns:repeat(4,1fr);gap:.65rem;margin:1rem 0}.summary-item{background:var(--surface-alt);border:1px solid var(--border);border-radius:5px;padding:.6rem .7rem}.summary-item span{display:block;color:var(--text-subtle);font-size:.72rem}.summary-item strong{display:block;margin-top:.08rem;font-size:.88rem;color:var(--text);word-break:break-word}
details.form-block{padding-bottom:.1rem}.form-block summary{cursor:pointer;color:var(--link);font-weight:600;font-size:.84rem;user-select:none}.form-block[open] summary{margin-bottom:.9rem}
body[data-ui-mode="beginner"] .mode-advanced,body[data-ui-mode="beginner"] .mode-expert,body[data-ui-mode="advanced"] .mode-expert{display:none!important}
.health-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:.65rem;margin:1rem 0 1.25rem}.health-filter{appearance:none;width:100%;text-align:left;border:1px solid var(--border);border-radius:6px;background:var(--surface);padding:.65rem .75rem;cursor:pointer;color:var(--text)}.health-filter:hover,.health-filter.active{border-color:var(--accent);box-shadow:0 0 0 2px var(--accent-ring)}.health-filter span{display:block;font-size:.74rem;color:var(--text-subtle)}.health-filter strong{display:block;margin-top:.05rem;font-size:1.2rem;font-variant-numeric:tabular-nums}.health-filter.ok strong{color:var(--ok-text)}.health-filter.warn strong{color:var(--warn-text)}.health-filter.error strong{color:var(--error-text)}.health-filter.unknown strong{color:var(--text-faint)}.server-card[hidden]{display:none}.filter-empty{display:none;padding:1.25rem 0;text-align:center;color:var(--text-subtle)}.filter-empty.visible{display:block}
@media(max-width:760px){header{height:auto;display:grid;grid-template-columns:1fr auto;align-items:center;padding:.65rem 1rem;gap:.5rem}header h1{grid-column:1}.interface-controls{grid-column:2;grid-row:1;gap:.35rem;flex-wrap:wrap;justify-content:flex-end}.mode-picker{gap:.25rem}.mode-picker span{display:none}header nav{grid-column:1/-1;grid-row:2;flex-wrap:nowrap;overflow-x:auto;width:100%;padding-bottom:.1rem;background-image:linear-gradient(to right,var(--header-bg) 40%,transparent),linear-gradient(to left,var(--header-bg) 40%,transparent),linear-gradient(to right,rgba(0,0,0,.35),transparent),linear-gradient(to left,rgba(0,0,0,.35),transparent);background-position:left,right,left,right;background-repeat:no-repeat;background-color:var(--header-bg);background-size:40px 100%,40px 100%,14px 100%,14px 100%;background-attachment:local,local,scroll,scroll}.detail-grid{grid-template-columns:1fr}.form-row,.form-row.w3{grid-template-columns:1fr}.form-grow{grid-column:auto}.form-actions{flex-wrap:wrap}}
@media(max-width:760px){.page-intro{display:block}.page-intro p{margin-top:.35rem}.setup-steps,.config-summary,.probe-transfer{grid-template-columns:1fr}.health-summary{grid-template-columns:1fr 1fr}.inline-form{grid-template-columns:1fr}.inline-form .btn{justify-self:start}}
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
</head><body data-ui-mode="beginner">
<a class="skip-link" href="#main-content" data-i18n="skip_to_content">Skip to content</a>
<header>
  <h1>WatchSSH</h1>
  <nav aria-label="Primary navigation" data-i18n-aria-label="primary_navigation">
    <a href="/" {{if eq .Page "dashboard"}}class="active" aria-current="page"{{end}} data-i18n="dashboard">Dashboard</a>
    <a href="/servers" {{if eq .Page "servers"}}class="active" aria-current="page"{{end}} data-i18n="servers">Servers</a>
    <a href="/alerts" {{if eq .Page "alerts"}}class="active" aria-current="page"{{end}} data-i18n="alerts">Alerts</a>
    <a href="/history" {{if eq .Page "history"}}class="active" aria-current="page"{{end}} data-i18n="history">History</a>
    <a href="/config" {{if eq .Page "config"}}class="active" aria-current="page"{{end}} data-i18n="configuration">Configuration</a>
  </nav>
  <div class="interface-controls">
    <label class="mode-picker"><span data-i18n="mode">Mode</span>
      <select id="ui-mode" aria-label="Configuration complexity" data-i18n-aria-label="configuration_complexity">
        <option value="beginner" data-i18n="beginner">Beginner</option>
        <option value="advanced" data-i18n="advanced">Advanced</option>
        <option value="expert" data-i18n="expert">Expert</option>
      </select>
    </label>
    <label class="mode-picker"><span data-i18n="language">Language</span>
      <select id="ui-language" aria-label="Interface language" data-i18n-aria-label="interface_language">
        <option value="en" data-i18n="english">English</option>
        <option value="de" data-i18n="german">Deutsch</option>
      </select>
    </label>
    <label class="mode-picker"><span data-i18n="theme">Theme</span>
      <select id="ui-theme" aria-label="Color theme" data-i18n-aria-label="color_theme">
        <option value="auto" data-i18n="theme_auto">Auto</option>
        <option value="light" data-i18n="theme_light">Light</option>
        <option value="dark" data-i18n="theme_dark">Dark</option>
      </select>
    </label>
  </div>
</header>
<main id="main-content" tabindex="-1">
{{end}}

{{define "ftr"}}</main>
<script>
(function(){
  var translations={
    en:{skip_to_content:'Skip to content',primary_navigation:'Primary navigation',dashboard:'Dashboard',servers:'Servers',alerts:'Alerts',history:'History',configuration:'Configuration',mode:'Mode',language:'Language',theme:'Theme',configuration_complexity:'Configuration complexity',interface_language:'Interface language',color_theme:'Color theme',beginner:'Beginner',advanced:'Advanced',expert:'Expert',english:'English',german:'Deutsch',theme_auto:'Auto',theme_light:'Light',theme_dark:'Dark',operations_overview:'Operations Overview',add_server:'Add server',manage_alerts:'Manage alerts',server_health_summary:'Server health summary',all_targets:'All targets',healthy:'Healthy',needs_attention:'Needs attention',unavailable:'Unavailable',targets:'Targets',details:'Details',no_targets_match:'No targets match this status filter.',server_management:'Server Management',configured_servers:'Configured Servers',test_connection:'Test Connection',add_alert_rule:'Add Alert Rule',start_with_template:'Start with a template',custom_rule:'Custom rule',add_rule:'Add Rule',remove:'Remove',remove_server:'Remove server',remove_alert_rule:'Remove alert rule'},
    de:{skip_to_content:'Zum Inhalt springen',primary_navigation:'Hauptnavigation',dashboard:'Übersicht',servers:'Server',alerts:'Alerts',history:'Verlauf',configuration:'Konfiguration',mode:'Modus',language:'Sprache',theme:'Design',configuration_complexity:'Konfigurationsumfang',interface_language:'Oberflächensprache',color_theme:'Farbschema',beginner:'Einfach',advanced:'Fortgeschritten',expert:'Expertin/Experte',english:'English',german:'Deutsch',theme_auto:'Automatisch',theme_light:'Hell',theme_dark:'Dunkel',operations_overview:'Betriebsübersicht',add_server:'Server hinzufügen',manage_alerts:'Alerts verwalten',server_health_summary:'Serverzustand',all_targets:'Alle Ziele',healthy:'Gesund',needs_attention:'Aufmerksamkeit nötig',unavailable:'Nicht erreichbar',targets:'Ziele',details:'Details',no_targets_match:'Keine Ziele entsprechen diesem Statusfilter.',server_management:'Serververwaltung',configured_servers:'Konfigurierte Server',test_connection:'Verbindung testen',add_alert_rule:'Alert-Regel hinzufügen',start_with_template:'Mit Vorlage beginnen',custom_rule:'Benutzerdefinierte Regel',add_rule:'Regel hinzufügen',remove:'Entfernen',remove_server:'Server entfernen',remove_alert_rule:'Alert-Regel entfernen'}
  };
  var languageSelect=document.getElementById('ui-language');
  function applyLanguage(language){
    if(!translations[language]) language='en';
    var dictionary=translations[language];
    document.documentElement.lang=language;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var value=dictionary[el.getAttribute('data-i18n')];
      if(value) el.textContent=value;
    });
    document.querySelectorAll('[data-i18n-aria-label]').forEach(function(el){
      var value=dictionary[el.getAttribute('data-i18n-aria-label')];
      if(value) el.setAttribute('aria-label',value);
    });
    document.querySelectorAll('[data-i18n-aria-prefix]').forEach(function(el){
      var prefix=dictionary[el.getAttribute('data-i18n-aria-prefix')];
      var value=el.getAttribute('data-i18n-aria-value');
      if(prefix && value) el.setAttribute('aria-label',prefix+' '+value);
    });
    if(languageSelect) languageSelect.value=language;
  }
  applyLanguage(localStorage.getItem('watchssh-ui-language') || 'en');
  if(languageSelect) languageSelect.addEventListener('change',function(){
    localStorage.setItem('watchssh-ui-language',languageSelect.value);
    applyLanguage(languageSelect.value);
  });
  var modeSelect=document.getElementById('ui-mode');
  function applyMode(mode){
    if(['beginner','advanced','expert'].indexOf(mode)===-1) mode='beginner';
    document.body.setAttribute('data-ui-mode',mode);
    if(modeSelect) modeSelect.value=mode;
  }
  applyMode(localStorage.getItem('watchssh-ui-mode') || 'beginner');
  if(modeSelect) modeSelect.addEventListener('change',function(){
    localStorage.setItem('watchssh-ui-mode',modeSelect.value);
    applyMode(modeSelect.value);
  });
  var themeSelect=document.getElementById('ui-theme');
  function applyTheme(theme){
    if(['auto','light','dark'].indexOf(theme)===-1) theme='auto';
    if(theme==='auto') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme',theme);
    if(themeSelect) themeSelect.value=theme;
  }
  applyTheme(localStorage.getItem('watchssh-ui-theme') || 'auto');
  if(themeSelect) themeSelect.addEventListener('change',function(){
    localStorage.setItem('watchssh-ui-theme',themeSelect.value);
    applyTheme(themeSelect.value);
  });
  var healthFilters=document.querySelectorAll('[data-health-filter]');
  var serverCards=document.querySelectorAll('[data-server-status]');
  var emptyFilter=document.getElementById('server-filter-empty');
  healthFilters.forEach(function(filter){
    filter.addEventListener('click',function(){
      var wanted=filter.getAttribute('data-health-filter');
      healthFilters.forEach(function(item){
        item.classList.remove('active');
        item.setAttribute('aria-pressed','false');
      });
      filter.classList.add('active');
      filter.setAttribute('aria-pressed','true');
      var visible=0;
      serverCards.forEach(function(card){
        card.hidden=wanted !== 'all' && card.getAttribute('data-server-status') !== wanted;
        if(!card.hidden) visible++;
      });
      if(emptyFilter) emptyFilter.classList.toggle('visible',visible===0);
    });
  });
  // Auto-refresh countdown.
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
<div class="page-intro">
  <div><h2 data-i18n="operations_overview">Operations Overview</h2><p>Live agentless checks across {{len .Servers}} configured targets. Select a status to focus the grid.</p></div>
  <div class="form-actions" style="margin:0"><a href="/servers#add-server" class="btn btn-primary" data-i18n="add_server">Add server</a><a href="/alerts" class="btn btn-secondary" data-i18n="manage_alerts">Manage alerts</a></div>
</div>
{{if .Flash}}<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>{{end}}
<div class="health-summary" aria-label="Server health summary" data-i18n-aria-label="server_health_summary">
  <button type="button" class="health-filter active" data-health-filter="all" aria-pressed="true"><span data-i18n="all_targets">All targets</span><strong>{{len .Servers}}</strong></button>
  <button type="button" class="health-filter ok" data-health-filter="ok" aria-pressed="false"><span data-i18n="healthy">Healthy</span><strong>{{.OK}}</strong></button>
  <button type="button" class="health-filter warn" data-health-filter="warn" aria-pressed="false"><span data-i18n="needs_attention">Needs attention</span><strong>{{.Warnings}}</strong></button>
  <button type="button" class="health-filter error" data-health-filter="error" aria-pressed="false"><span data-i18n="unavailable">Unavailable</span><strong>{{.Errors}}</strong></button>
</div>
{{if .Unknown}}<p class="restart-note">{{.Unknown}} target{{if ne .Unknown 1}}s{{end}} waiting for their first result.</p>{{end}}
<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
  <h3 style="margin:0" data-i18n="targets">Targets</h3>
  <span style="font-size:.8rem;color:var(--text-faint)">Auto-refresh in <span id="refresh-count">30</span>s</span>
</div>
<div class="grid">
{{range .Servers}}
  <div class="card server-card" data-server-status="{{serverStatus .}}">
    <div class="card-head">
      <span class="card-title" title="{{.ServerName}}">{{.ServerName}}</span>
      <span class="badge badge-{{serverStatus .}}" role="status">{{serverStatusLabel .}}</span>
    </div>
    <div class="card-body">
      {{if .Error}}
        <p style="color:var(--error-text);font-size:.84rem;margin:0">{{.Error}}</p>
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
            {{if .Connectivity.PingOK}}{{printf "%.1f" .Connectivity.PingLatency}} ms{{else}}<span style="color:var(--error-text)">FAILED</span>{{end}}
          </span>
        </div>
        {{end}}
        {{if .Connectivity.Ports}}
        <div class="m-row">
          <span class="m-label">Ports</span>
          <span class="m-val">
            {{range .Connectivity.Ports}}
              <span class="tag {{if .Open}}tag-ok{{else}}tag-err{{end}}" title="{{if .Host}}{{.Host}}:{{end}}{{.Port}} checked from {{if .Source}}{{.Source}}{{else}}monitor{{end}}">:{{.Port}}</span>
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
      <a href="/server/{{.ServerName}}" class="btn btn-secondary btn-sm" data-i18n="details">Details</a>
    </div>
  </div>
{{else}}
  <p class="empty">No servers configured yet. <a href="/servers">Add one.</a></p>
{{end}}
</div>
<p id="server-filter-empty" class="filter-empty" role="status" data-i18n="no_targets_match">No targets match this status filter.</p>

{{if .Firings}}
<h2>Recent Alerts</h2>
{{range .Firings}}
<div class="firing-item">
  <div class="msg">{{.Message}}</div>
  <div class="ts">{{.FiredAt.Format "2006-01-02 15:04:05 MST"}}</div>
  {{with .Watchdog}}<div class="ts">Watchdog {{.Model}}: {{.Status}}{{if .Severity}} ({{.Severity}}){{end}}{{if .Summary}} - {{.Summary}}{{end}}{{if .Error}} ({{.Error}}){{end}}</div>{{if .DeferredRemediations}}<div class="ts">Watchdog actions deferred below the configured severity: {{range .DeferredRemediations}}{{.}} {{end}}</div>{{end}}{{range .Remediations}}<div class="ts">Watchdog action {{.Name}} on {{.Target}}: {{.Status}}{{if .Error}} ({{.Error}}){{end}}</div>{{end}}{{end}}
  {{range .Remediations}}<div class="ts">Remediation {{.Name}} on {{.Target}}: {{.Status}}{{if .Error}} ({{.Error}}){{end}}</div>{{end}}
</div>
{{end}}
{{end}}
{{template "ftr" .}}
{{end}}

{{define "history-page"}}
{{template "hdr" .}}
<div class="page-intro">
  <div><h2>History</h2><p>Investigate recent measurements and alert evidence from the local tinySQL store.</p></div>
  {{if .StorageEnabled}}<span style="font-size:.8rem;color:var(--text-faint)">Newest records first</span>{{end}}
</div>

{{if not .StorageEnabled}}
<div class="notice notice-err">History storage is disabled. Enable <code>storage.type: tinysql</code> in the configuration and restart WatchSSH.</div>
{{else}}
  {{if .Error}}<div class="notice notice-err">{{.Error}}</div>{{end}}

  <div class="config-summary" aria-label="History summary">
    <div class="summary-item"><span>Metric samples</span><strong>{{len .MetricSamples}} loaded</strong></div>
    <div class="summary-item"><span>Alert firings</span><strong>{{len .AlertFirings}} loaded</strong></div>
    <div class="summary-item"><span>Scope</span><strong>{{if .ServerFilter}}{{.ServerFilter}}{{else}}All targets{{end}}</strong></div>
    <div class="summary-item"><span>Storage</span><strong>tinySQL</strong></div>
  </div>

  <div class="form-wrap" style="margin-top:0">
    <div class="form-section-title"><h3>Scope</h3><span>Uses the server index for targeted history queries.</span></div>
    <form method="get" action="/history">
      <div class="form-row">
        <div>
          <label>Target</label>
          <select name="server">
            <option value="">All targets</option>
            {{range .ServerNames}}<option value="{{.}}" {{if eq . $.ServerFilter}}selected{{end}}>{{.}}</option>{{end}}
          </select>
        </div>
        <div style="display:flex;align-items:end;gap:.5rem">
          <button type="submit" class="btn btn-primary">Apply</button>
          <a href="/history" class="btn btn-secondary">Clear</a>
        </div>
      </div>
    </form>
  </div>

  <div class="section">
    <div class="form-section-title"><h3>Metric Samples</h3><span>Most recent 100 matching records.</span></div>
    {{if .MetricSamples}}
    <table>
      <thead><tr><th>Collected</th><th>Server</th><th>Platform</th><th>Status</th><th>CPU</th><th>RAM</th><th>Disk /</th><th>Load</th><th>Ping</th><th>DNS</th><th>TLS</th><th>Trace</th><th>Board</th></tr></thead>
      <tbody>
      {{range .MetricSamples}}
        <tr>
          <td>{{.CollectedAt}}</td>
          <td>{{.ServerName}}</td>
          <td>{{.Platform}}</td>
          <td>{{if .HasError}}<span class="badge badge-error">error</span>{{else}}<span class="badge badge-ok">ok</span>{{end}}</td>
          <td>{{fmtOptFloat .CPUUsage}}</td>
          <td>{{fmtOptFloat .MemoryUsage}}</td>
          <td>{{fmtOptFloat .DiskRootUsage}}</td>
          <td>{{fmtOptFloat .Load1}}</td>
          <td>{{fmtOptBool .PingOK}}{{if .PingLatencyMS}} ({{fmtOptFloat .PingLatencyMS}} ms){{end}}</td>
          <td>{{fmtOptBool .DNSOK}}</td>
          <td>{{fmtOptFloat .TLSCertMinDays}}</td>
          <td>{{fmtOptFloat .TracerouteHops}}</td>
          <td>{{fmtOptFloat .BoardTemperatureC}}{{if .BoardWiFiRSSIDbm}} / {{fmtOptFloat .BoardWiFiRSSIDbm}} dBm{{end}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No metric history recorded yet.</p>{{end}}
  </div>

  <div class="section">
    <div class="form-section-title"><h3>Alert Firings</h3><span>Most recent 100 records across all targets.</span></div>
    {{if .AlertFirings}}
    <table>
      <thead><tr><th>Fired</th><th>Rule</th><th>Metric</th><th>Server</th><th>Value</th><th>Message</th></tr></thead>
      <tbody>
      {{range .AlertFirings}}
        <tr>
          <td>{{.FiredAt}}</td>
          <td>{{.RuleName}}</td>
          <td>{{.Metric}}</td>
          <td>{{.Server}}</td>
          <td>{{printf "%.2f" .Value}}</td>
          <td>{{.Message}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
    {{else}}<p class="empty">No alert history recorded yet.</p>{{end}}
  </div>
{{end}}
{{template "ftr" .}}
{{end}}

{{define "server-detail"}}
{{template "hdr" .}}
<div style="margin-bottom:1rem;display:flex;align-items:center;gap:1rem">
  <a href="/" class="btn btn-secondary btn-sm">← Back</a>
  <h2 style="margin:0">{{.Metrics.ServerName}}
    {{if .Metrics.Host}}<span style="font-weight:400;color:var(--text-faint);font-size:.9rem">({{.Metrics.Host}})</span>{{end}}
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
      {{if .Metrics.System.CPUCores}}<tr><td class="m-label">CPU Cores</td><td>{{.Metrics.System.CPUCores}}</td></tr>{{end}}
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
      <tr><td class="m-label">Load 15m</td><td>{{printf "%.2f" .Metrics.Load.Load15}}</td></tr>
      {{if .Metrics.Load.TotalProcesses}}<tr><td class="m-label">Runnable / Total Processes</td><td>{{.Metrics.Load.RunningProcesses}} / {{.Metrics.Load.TotalProcesses}}</td></tr>{{end}}
      {{if .Metrics.Load.LastPID}}<tr><td class="m-label">Last PID</td><td>{{.Metrics.Load.LastPID}}</td></tr>{{end}}{{else}}<tr><td class="m-label">Load</td><td>n/a</td></tr>{{end}}
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
  {{if .Metrics.FileDescriptors}}
  <div class="section">
    <h3>File Descriptors</h3>
    <table><tbody>
      <tr><td class="m-label">In Use</td><td>{{fdInUse .Metrics.FileDescriptors}} / {{.Metrics.FileDescriptors.Max}} ({{printf "%.1f" .Metrics.FileDescriptors.UsagePercent}}%)</td></tr>
      <tr><td class="m-label">Allocated</td><td>{{.Metrics.FileDescriptors.Allocated}}</td></tr>
      <tr><td class="m-label">Unused Allocated</td><td>{{.Metrics.FileDescriptors.Unused}}</td></tr>
    </tbody></table>
  </div>
  {{end}}
  {{if .Metrics.Board}}
  <div class="section">
    <h3>Board</h3>
    <table><tbody>
      {{if .Metrics.Board.Model}}<tr><td class="m-label">Model</td><td>{{.Metrics.Board.Model}}</td></tr>{{end}}
      <tr><td class="m-label">Temperature</td><td>{{fmtOptFloat .Metrics.Board.TemperatureC}} °C</td></tr>
      <tr><td class="m-label">CPU Frequency</td><td>{{fmtOptFloat .Metrics.Board.CPUFrequencyMHz}} MHz</td></tr>
      {{if .Metrics.Board.WiFiRSSIDbm}}<tr><td class="m-label">Wi-Fi RSSI</td><td>{{if .Metrics.Board.WiFiInterface}}{{.Metrics.Board.WiFiInterface}}: {{end}}{{fmtOptFloat .Metrics.Board.WiFiRSSIDbm}} dBm</td></tr>{{end}}
      {{if .Metrics.Board.ThrottledHex}}<tr><td class="m-label">Throttled Flags</td><td><code>{{.Metrics.Board.ThrottledHex}}</code></td></tr>{{end}}
      <tr><td class="m-label">Under-voltage Now</td><td>{{if .Metrics.Board.UnderVoltageNow}}yes{{else}}no{{end}}</td></tr>
      <tr><td class="m-label">Throttled Now</td><td>{{if .Metrics.Board.ThrottledNow}}yes{{else}}no{{end}}</td></tr>
      <tr><td class="m-label">Under-voltage Seen</td><td>{{if .Metrics.Board.UnderVoltageSeen}}yes{{else}}no{{end}}</td></tr>
      <tr><td class="m-label">Throttled Seen</td><td>{{if .Metrics.Board.ThrottledSeen}}yes{{else}}no{{end}}</td></tr>
    </tbody></table>
  </div>
  {{end}}
  <div class="section">
    <h3>Connectivity</h3>
    {{with .Metrics.Connectivity}}
    {{if .PingEnabled}}
    <div class="m-row">
      <span class="m-label">Ping</span>
      <span>
        {{if .PingOK}}<span class="dot dot-ok"></span>OK — {{printf "%.1f" .PingLatency}} ms, {{printf "%.1f" .PingLoss}}% loss
        {{else}}<span class="dot dot-err"></span>FAILED{{end}}
      </span>
    </div>
    {{end}}
    {{range .Ports}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label">Port {{if .Host}}{{.Host}}:{{end}}{{.Port}}{{if eq .Source "target"}} (from target){{end}}</span>
      <span>{{if .Open}}<span class="dot dot-ok"></span>Open{{else}}<span class="dot dot-err"></span>Closed{{end}} — {{printf "%.0f" .LatencyMs}} ms</span>
      {{if .Error}}<span class="m-error">{{.Error}}</span>{{end}}
    </div>
    {{end}}
    {{range .Banner}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label">Banner {{.Host}}:{{.Port}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>{{.Banner}}{{else}}<span class="dot dot-err"></span>FAILED{{end}} — {{printf "%.0f" .LatencyMs}} ms</span>
      {{if .Error}}<span class="m-error">{{.Error}}</span>{{end}}
    </div>
    {{end}}
    {{range .HTTP}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label" style="word-break:break-all">{{.Method}} {{.URL}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>{{.StatusCode}}{{else}}<span class="dot dot-err"></span>{{if .StatusCode}}{{.StatusCode}}{{else}}ERR{{end}}{{end}} — {{printf "%.0f" .LatencyMs}} ms</span>
      {{if .Error}}<span class="m-error">{{.Error}}</span>{{end}}
    </div>
    {{end}}
    {{range .DNS}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label">DNS {{.Type}} {{.Host}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>OK{{else}}<span class="dot dot-err"></span>FAILED{{end}} — {{printf "%.0f" .LatencyMs}} ms</span>
    </div>
    {{end}}
    {{range .Traceroute}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label">Traceroute {{.Host}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>{{.Hops}} hops{{else}}<span class="dot dot-err"></span>FAILED{{end}} — {{printf "%.0f" .LatencyMs}} ms</span>
    </div>
    {{end}}
    {{range .TLS}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label">TLS {{.Host}}:{{.Port}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>OK{{else}}<span class="dot dot-err"></span>FAILED{{end}}{{if .CertExpiresDays}} — {{fmtOptFloat .CertExpiresDays}} days{{end}}</span>
    </div>
    {{end}}
    {{range .NTP}}
    <div class="m-row" style="flex-wrap:wrap">
      <span class="m-label">NTP {{.Host}}:{{.Port}}</span>
      <span>{{if .OK}}<span class="dot dot-ok"></span>stratum {{.Stratum}}{{else}}<span class="dot dot-err"></span>FAILED{{end}} — {{printf "%.1f" .OffsetMs}} ms offset, {{printf "%.0f" .LatencyMs}} ms</span>
      {{if .Error}}<span class="m-error">{{.Error}}</span>{{end}}
    </div>
    {{end}}
    {{if and (not .PingEnabled) (not .Ports) (not .Banner) (not .HTTP) (not .DNS) (not .Traceroute) (not .TLS) (not .NTP)}}
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
    <thead><tr><th>Device</th><th>Mount</th><th>Used</th><th>Total</th><th>Usage</th><th>Inodes</th></tr></thead>
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
      <td>{{if .InodesTotal}}{{.InodesUsed}} / {{.InodesTotal}} ({{printf "%.1f" .InodesUsagePercent}}%){{else}}n/a{{end}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .Metrics.Inodes}}
<div class="section">
  <h3>Inode Usage</h3>
  <table>
    <thead><tr><th>Device</th><th>Mount</th><th>Used</th><th>Total</th><th>Usage</th></tr></thead>
    <tbody>
    {{range .Metrics.Inodes}}
    <tr>
      <td>{{.Device}}</td>
      <td>{{.MountPoint}}</td>
      <td>{{.UsedInodes}}</td>
      <td>{{.TotalInodes}}</td>
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
    <thead><tr><th>Interface</th><th>Received</th><th>Sent</th><th>Packets In</th><th>Packets Out</th><th>Errors</th><th>Drops</th></tr></thead>
    <tbody>
    {{range .Metrics.Network}}
    {{if or .BytesRecv .BytesSent}}
    <tr>
      <td>{{.Interface}}</td>
      <td>{{fmtBytes .BytesRecv}}</td>
      <td>{{fmtBytes .BytesSent}}</td>
      <td>{{.PacketsRecv}}</td>
      <td>{{.PacketsSent}}</td>
      <td>{{netErrors .}}</td>
      <td>{{netDrops .}}</td>
    </tr>
    {{end}}
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .Metrics.Users}}
<div class="section">
  <h3>Logged-in Users</h3>
  <table>
    <thead><tr><th>User</th><th>TTY</th><th>Login Time</th><th>Host</th></tr></thead>
    <tbody>
    {{range .Metrics.Users}}
    <tr>
      <td>{{.User}}</td>
      <td>{{.TTY}}</td>
      <td>{{.LoginTime}}</td>
      <td>{{if .Host}}{{.Host}}{{else}}—{{end}}</td>
    </tr>
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
<div class="page-intro">
  <div><h2 data-i18n="server_management">Server Management</h2><p>Build an agentless monitoring target. WatchSSH connects over SSH only when host metrics or a remote custom check are needed.</p></div>
  <a href="#add-server" class="btn btn-primary" data-i18n="add_server">Add server</a>
</div>
<div class="notice notice-info mode-advanced">Advanced mode exposes operational probe settings. Switch to Expert only when you need custom commands or protocol-level tuning.</div>
{{if .Flash}}<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>{{end}}

<div class="section">
  <h3><span data-i18n="configured_servers">Configured Servers</span> ({{len .Servers}})</h3>
  {{if .Servers}}
  <div class="table-scroll">
  <table>
    <thead><tr><th>Name</th><th>Host</th><th>Port</th><th>User</th><th>Type</th><th>Tags</th><th>Checks</th><th>Status</th><th></th></tr></thead>
    <tbody>
    {{range .Servers}}
    <tr>
      <td><a href="/server/{{.ServerName}}">{{.ServerName}}</a></td>
      <td>{{if .Host}}{{.Host}}{{else}}<em>local</em>{{end}}</td>
      <td>{{if not .Host}}—{{else}}{{.Port}}{{end}}</td>
      <td>{{if .Username}}{{.Username}}{{else}}—{{end}}</td>
      <td>{{if not .Host}}local{{else}}SSH{{end}}</td>
      <td>{{range .Tags}}<span class="pill">{{.}}</span>{{else}}—{{end}}</td>
      <td>{{.CheckSummary}}</td>
      <td><span class="badge badge-{{serverStatus .ServerMetrics}}" role="status">{{serverStatusLabel .ServerMetrics}}</span></td>
      <td>
        <form method="post" action="/servers/remove" style="display:inline">
          <input type="hidden" name="name" value="{{.ServerName}}">
          <button type="submit" class="btn btn-danger btn-sm" aria-label="Remove server {{.ServerName}}" data-i18n-aria-prefix="remove_server" data-i18n-aria-value="{{.ServerName}}"
            onclick="return confirm('Remove {{.ServerName}}?')" data-i18n="remove">Remove</button>
        </form>
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
  </div>
  {{else}}
  <p class="empty">No servers configured yet.</p>
{{end}}
</div>

<div class="form-wrap" id="probe-workspace">
  <div class="form-section-title"><h3>Probe Library</h3><span>Add focused checks, manage existing probes, or exchange probe-only bundles.</span></div>
  {{if .ServerNames}}
  <form method="post" action="/probes/add" class="probe-builder">
    <div class="form-row w3">
      <div><label for="probe-server">Target</label><select id="probe-server" name="server">{{range .ServerNames}}<option value="{{.}}">{{.}}</option>{{end}}</select></div>
      <div><label for="probe-kind">Probe type</label><select id="probe-kind" name="kind"><option value="http">HTTP health</option><option value="tcp">TCP port</option><option value="dns">DNS lookup</option><option value="tls">TLS certificate</option><option value="ping">Ping</option><option value="ntp">NTP</option><option value="trace">Traceroute</option><option value="custom">Remote command</option></select></div>
      <div><label for="probe-timeout">Timeout (seconds)</label><input id="probe-timeout" type="number" name="timeout" value="5" min="1"></div>
    </div>
    <div class="form-row w3">
      <div><label for="probe-target">URL or host</label><input id="probe-target" type="text" name="target" placeholder="https://service.example/health or db.internal"></div>
      <div><label for="probe-port">Port</label><input id="probe-port" type="number" name="probe_port" placeholder="80, 443, 5432" min="1" max="65535"></div>
      <div><label for="probe-source">TCP origin</label><select id="probe-source" name="source"><option value="monitor">Monitoring host</option><option value="target">Target network (SSH)</option></select></div>
    </div>
    <div class="form-row mode-advanced">
      <div><label for="probe-method">HTTP method / DNS type</label><input id="probe-method" type="text" name="method" value="GET" placeholder="GET or A"></div>
      <div><label for="probe-status">Expected HTTP status</label><input id="probe-status" type="number" name="expected_status" value="200" min="100" max="599"></div>
    </div>
    <details class="probe-details mode-expert">
      <summary>Optional probe details</summary>
      <div class="form-row w3">
        <div><label>DNS type</label><input type="text" name="dns_type" value="A" placeholder="A, AAAA, MX"></div>
        <div><label>DNS resolver</label><input type="text" name="resolver" placeholder="1.1.1.1"></div>
        <div><label>NTP max offset (ms)</label><input type="number" name="max_offset_ms" min="0" step="0.1"></div>
      </div>
      <div class="form-row">
        <div><label>Remote command name</label><input type="text" name="probe_name" placeholder="service-running"></div>
        <div><label>Remote command</label><input type="text" name="command" placeholder="pgrep -x nginx && echo OK"></div>
      </div>
    </details>
    <div class="form-actions"><button type="submit" class="btn btn-primary">Add probe</button><span class="restart-note">Target-network TCP probes use SSH direct-tcpip and do not need netcat.</span></div>
  </form>

  <div class="probe-transfer">
    <form method="get" action="/probes/export" class="inline-form"><label for="probe-export-server">Export probes</label><select id="probe-export-server" name="server">{{range .ServerNames}}<option value="{{.}}">{{.}}</option>{{end}}</select><select name="format" aria-label="Export format"><option value="yaml">YAML</option><option value="json">JSON</option></select><button type="submit" class="btn btn-secondary">Download</button></form>
    <form method="post" action="/probes/import" enctype="multipart/form-data" class="inline-form"><label for="probe-import-server">Import into</label><select id="probe-import-server" name="server">{{range .ServerNames}}<option value="{{.}}">{{.}}</option>{{end}}</select><input id="probe-bundle" type="file" name="bundle" accept="application/json,application/x-yaml,.json,.yaml,.yml"><button type="submit" class="btn btn-secondary">Import file</button></form>
  </div>
  {{else}}
  <p class="empty">Add a server before creating or importing probes.</p>
  {{end}}
</div>

<div class="section" aria-labelledby="configured-probes-title">
  <div class="form-section-title"><h3 id="configured-probes-title">Configured Probes</h3><span>{{len .Probes}} saved checks across all targets.</span></div>
  {{if .Probes}}
  <div class="table-scroll"><table><thead><tr><th>Target</th><th>Type</th><th>Definition</th><th></th></tr></thead><tbody>
  {{range .Probes}}<tr><td>{{.Server}}</td><td><span class="pill">{{.Name}}</span></td><td><code>{{.Detail}}</code></td><td><form method="post" action="/probes/remove" style="display:inline"><input type="hidden" name="server" value="{{.Server}}"><input type="hidden" name="kind" value="{{.Kind}}"><input type="hidden" name="index" value="{{.Index}}"><button type="submit" class="btn btn-danger btn-sm" aria-label="Remove {{.Name}} probe from {{.Server}}" onclick="return confirm('Remove this probe?')">Remove</button></form></td></tr>{{end}}
  </tbody></table></div>
  {{else}}<p class="empty">No explicit probes yet. System metrics are collected automatically.</p>{{end}}
</div>

<div class="form-wrap" id="add-server">
  <div class="form-section-title"><h3 data-i18n="add_server">Add Server</h3><span>Changes are saved to the active configuration file.</span></div>
  <div class="setup-steps">
    <div class="setup-step active"><strong>1. Choose a profile</strong>Start with suitable checks and tags.</div>
    <div class="setup-step"><strong>2. Connect securely</strong>Use a restricted SSH account where required.</div>
    <div class="setup-step"><strong>3. Verify and save</strong>Test connectivity before enabling monitoring.</div>
  </div>
  <form method="post" action="/servers/add">
    <div class="form-row">
      <div>
        <label for="server-profile">Profile</label>
        <select name="profile" id="server-profile" aria-describedby="profile-note">
          <option value="">Custom</option>
          <option value="web">Web / HTTPS service</option>
          <option value="harp">HARP reverse proxy</option>
          <option value="raspberry-pi">Raspberry Pi / SBC</option>
          <option value="local">Local machine</option>
        </select>
        <div class="profile-note" id="profile-note">Custom starts with host metrics. Add only the probes this target needs.</div>
      </div>
      <div>
        <label for="server-tags">Tags</label>
        <input type="text" id="server-tags" name="tags" placeholder="linux, production, edge">
      </div>
    </div>
    <div class="form-block">
      <h4>Connection</h4>
    <div class="form-row">
      <div><label for="server-name">Name *</label><input type="text" id="server-name" name="name" placeholder="web-01" required></div>
      <div><label for="server-host">Host / IP *</label><input type="text" id="server-host" name="host" placeholder="192.168.1.10"></div>
    </div>
    <div class="form-row mode-advanced">
      <div><label>SSH Port</label><input type="number" name="port" value="22" min="1" max="65535"></div>
      <div></div>
    </div>
    <div class="form-row">
      <div><label for="server-username">SSH Username</label><input type="text" id="server-username" name="username" placeholder="monitor"></div>
      <div><label id="auth-credential-label" for="auth-credential">Private Key File</label><input type="text" id="auth-credential" name="auth_credential" placeholder="~/.ssh/id_ed25519"></div>
    </div>
    <div class="form-row mode-advanced">
      <div>
        <label>Auth Type</label>
        <select name="auth_type">
          <option value="key">Private Key</option>
          <option value="password">Password</option>
          <option value="agent">SSH Agent</option>
        </select>
      </div>
    </div>
    <div class="form-row">
      <div>
        <label class="inline-check">
          <input type="checkbox" id="server-local" name="local" value="1">
          Monitor this machine locally (no SSH)
        </label>
      </div>
      <div>
        <label class="inline-check">
          <input type="checkbox" id="server-ping" name="ping" value="1">
          Enable ping check
        </label>
      </div>
    </div>
    </div>
    <div class="form-row w3 mode-advanced">
      <div><label>Ping Count</label><input type="number" name="ping_count" value="3" min="1" max="10"></div>
      <div><label>Ping Timeout</label><input type="number" name="ping_timeout" value="5" min="1"></div>
      <div>
        <label class="inline-check" style="margin-top:1.35rem">
          <input type="checkbox" name="docker_enabled" value="1">
          Docker metrics
        </label>
      </div>
    </div>

    <div class="form-block">
      <h4>Service &amp; Connectivity Checks</h4>
      <p class="restart-note">Use a URL for application health. TLS, DNS and ports can verify the path to the application independently.</p>
      <div class="form-row">
        <div>
          <label>TCP Ports</label>
          <input type="text" name="ports" placeholder="22, 80, 443">
        </div>
        <div><label>Port Timeout</label><input type="number" name="port_timeout" value="5" min="1"></div>
      </div>
      <div class="form-row w3 mode-advanced">
        <div><label>Banner Hosts</label><input type="text" name="banner_hosts" placeholder="ssh.example.com, smtp.example.com"></div>
        <div><label>Banner Port</label><input type="number" name="banner_port" value="22" min="1" max="65535"></div>
        <div><label>Expected Prefix</label><input type="text" name="banner_expected_prefix" placeholder="SSH-, 220, +PONG"></div>
      </div>
      <div class="form-row mode-advanced"><div><label>Banner Timeout</label><input type="number" name="banner_timeout" value="5" min="1"></div><div></div></div>
      <div class="form-row">
        <div>
          <label>HTTP URLs</label>
          <input type="text" name="http_urls" placeholder="https://example.com/health, https://example.com/readyz">
        </div>
        <div class="form-row" style="margin:0">
          <div><label>Expected Status</label><input type="number" name="http_expected_status" value="200" min="100" max="599"></div>
          <div><label>HTTP Timeout</label><input type="number" name="http_timeout" value="10" min="1"></div>
        </div>
      </div>
      <div class="form-row w3 mode-advanced">
        <div><label>HTTP Method</label><select name="http_method"><option value="GET">GET</option><option value="HEAD">HEAD</option><option value="OPTIONS">OPTIONS</option></select></div>
        <div class="form-grow"><label>Expected Body</label><input type="text" name="http_expected_body" placeholder="optional response substring"></div>
      </div>
      <details class="probe-details mode-expert">
        <summary>Advanced network probes: DNS, TLS, traceroute, NTP and banners</summary>
        <div class="form-row w3">
          <div><label>DNS Hosts</label><input type="text" name="dns_hosts" placeholder="example.com"></div>
          <div><label>DNS Type</label><input type="text" name="dns_type" value="A"></div>
          <div><label>DNS Resolver</label><input type="text" name="dns_server" placeholder="1.1.1.1"></div>
        </div>
        <div class="form-row">
          <div><label>DNS Expected Answer</label><input type="text" name="dns_expected_answer" placeholder="optional substring"></div>
          <div><label>DNS Timeout</label><input type="number" name="dns_timeout" value="5" min="1"></div>
        </div>
        <div class="form-row w3">
          <div><label>TLS Hosts</label><input type="text" name="tls_hosts" placeholder="example.com"></div>
          <div><label>TLS Port</label><input type="number" name="tls_port" value="443" min="1" max="65535"></div>
          <div><label>TLS Server Name</label><input type="text" name="tls_server_name" placeholder="blank = host"></div>
        </div>
        <div class="form-row">
          <div><label>Traceroute Hosts</label><input type="text" name="traceroute_hosts" placeholder="example.com"></div>
          <div class="form-row" style="margin:0">
            <div><label>Max Hops</label><input type="number" name="traceroute_max_hops" value="30" min="1"></div>
            <div><label>Trace Timeout</label><input type="number" name="traceroute_timeout" value="10" min="1"></div>
          </div>
        </div>
        <div class="form-row w3">
          <div><label>NTP Servers</label><input type="text" name="ntp_hosts" placeholder="time.cloudflare.com"></div>
          <div><label>NTP Max Offset (ms)</label><input type="number" name="ntp_max_offset_ms" placeholder="0 = do not validate" min="0" step="0.1"></div>
          <div class="form-row" style="margin:0"><div><label>NTP Port</label><input type="number" name="ntp_port" value="123" min="1" max="65535"></div><div><label>NTP Timeout</label><input type="number" name="ntp_timeout" value="5" min="1"></div></div>
        </div>
      </details>
    </div>

    <details class="form-block mode-expert">
      <summary>Custom remote check</summary>
      <p class="restart-note">Commands run through SSH on this target. Prefer a dedicated read-only monitoring account and a narrowly scoped command.</p>
      <div class="form-row w3">
        <div><label>Name</label><input type="text" name="custom_name" placeholder="service-running"></div>
        <div><label>Command</label><input type="text" name="custom_command" placeholder="pgrep -x nginx && echo OK"></div>
        <div><label>Expected Output</label><input type="text" name="custom_expected_output" placeholder="OK"></div>
      </div>
    </details>
    <div class="form-actions">
      <button type="button" class="btn btn-secondary" id="btn-test-conn" data-i18n="test_connection">Test Connection</button>
      <button type="submit" class="btn btn-primary" data-i18n="add_server">Add Server</button>
      <span id="test-conn-result" role="status" aria-live="polite" style="font-size:.83rem;margin-left:.5rem"></span>
    </div>
  </form>
</div>
<script>
(function(){
  var btn = document.getElementById('btn-test-conn');
  var result = document.getElementById('test-conn-result');
  var profile = document.getElementById('server-profile');
  var profileNote = document.getElementById('profile-note');
  var authType = document.querySelector('[name=auth_type]');
  var credentialLabel = document.getElementById('auth-credential-label');
  var credential = document.getElementById('auth-credential');
  var probeKind = document.getElementById('probe-kind');
  var probeTarget = document.getElementById('probe-target');
  var probePort = document.getElementById('probe-port');
  var probeSource = document.getElementById('probe-source');
  var profileNotes = {
    '': 'Custom starts with host metrics. Add only the probes this target needs.',
    web: 'Adds ports 80/443 plus HTTP health, DNS and TLS checks for the selected host.',
    harp: 'Adds HARP health, readiness and metrics endpoints plus DNS, TLS and ports 80/443.',
    'raspberry-pi': 'Adds Raspberry Pi/SBC tags and a ping check. Enable Docker metrics when applicable.',
    local: 'Runs host checks on the WatchSSH machine. No SSH credential is stored or used.'
  };
  function setIfEmpty(form, name, value){
    var el = form.querySelector('[name='+name+']');
    if(el && !el.value.trim()) el.value = value;
  }
  function appendList(form, name, value){
    var el = form.querySelector('[name='+name+']');
    if(!el || !value) return;
    var items = el.value.split(/[,\n;]/).map(function(v){ return v.trim(); }).filter(Boolean);
    if(items.indexOf(value) === -1) items.push(value);
    el.value = items.join(', ');
  }
  if(profile){
    profile.addEventListener('change', function(){
      var form = profile.closest('form');
      if(profileNote) profileNote.textContent = profileNotes[profile.value] || profileNotes[''];
      var host = form.querySelector('[name=host]').value.trim() || 'example.com';
      var local = form.querySelector('[name=local]');
      if(profile.value === 'local'){
        if(local) local.checked = true;
        appendList(form, 'tags', 'local');
      }
      if(profile.value === 'web'){
        appendList(form, 'tags', 'web');
        appendList(form, 'ports', '80');
        appendList(form, 'ports', '443');
        appendList(form, 'http_urls', 'https://'+host+'/health');
        appendList(form, 'dns_hosts', host);
        appendList(form, 'tls_hosts', host);
      }
      if(profile.value === 'harp'){
        appendList(form, 'tags', 'harp');
        appendList(form, 'tags', 'reverse-proxy');
        appendList(form, 'ports', '80');
        appendList(form, 'ports', '443');
        appendList(form, 'http_urls', 'https://'+host+'/health');
        appendList(form, 'http_urls', 'https://'+host+'/readyz');
        appendList(form, 'http_urls', 'https://'+host+'/metrics');
        appendList(form, 'dns_hosts', host);
        appendList(form, 'tls_hosts', host);
        setIfEmpty(form, 'dns_server', '1.1.1.1');
      }
      if(profile.value === 'raspberry-pi'){
        appendList(form, 'tags', 'raspberry-pi');
        appendList(form, 'tags', 'sbc');
        var ping = form.querySelector('[name=ping]');
        if(ping) ping.checked = true;
      }
    });
  }
  function updateCredentialHint(){
    if(!authType || !credentialLabel || !credential) return;
    if(authType.value === 'password'){
      credentialLabel.textContent = 'Password';
      credential.placeholder = 'password or environment reference';
      return;
    }
    if(authType.value === 'agent'){
      credentialLabel.textContent = 'Credential';
      credential.placeholder = 'not required for SSH agent';
      return;
    }
    credentialLabel.textContent = 'Private Key File';
    credential.placeholder = '~/.ssh/id_ed25519';
  }
  if(authType){ authType.addEventListener('change', updateCredentialHint); updateCredentialHint(); }
  function updateProbePreset(){
    if(!probeKind || !probeTarget || !probePort || !probeSource) return;
    var presets = {
      http:{placeholder:'https://service.example/health',port:'',source:false},
      tcp:{placeholder:'db.internal or service.example',port:'443',source:true},
      dns:{placeholder:'example.com',port:'',source:false},
      tls:{placeholder:'service.example',port:'443',source:false},
      ping:{placeholder:'uses the target host when blank',port:'',source:false},
      ntp:{placeholder:'time.cloudflare.com',port:'123',source:false},
      trace:{placeholder:'example.com',port:'',source:false},
      custom:{placeholder:'not required',port:'',source:false}
    };
    var preset = presets[probeKind.value] || presets.http;
    probeTarget.placeholder = preset.placeholder;
    probePort.value = preset.port;
    probePort.disabled = probeKind.value === 'ping' || probeKind.value === 'dns' || probeKind.value === 'trace' || probeKind.value === 'custom';
    probeSource.disabled = !preset.source;
  }
  if(probeKind){ probeKind.addEventListener('change', updateProbePreset); updateProbePreset(); }
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
    result.className = 'text-faint';
    result.textContent = 'Testing…';
    fetch('/api/test-connection', {method:'POST', headers:{'Content-Type':'application/json'}, body:body})
      .then(function(r){ return r.json(); })
      .then(function(data){
        result.className = data.ok ? 'text-ok' : 'text-error';
        result.textContent = (data.ok ? '✓ ' : '✗ ') + data.message;
      })
      .catch(function(e){ result.className = 'text-error'; result.textContent='Request failed: '+e; })
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
    {{with .Watchdog}}<div class="ts">Watchdog {{.Model}}: {{.Status}}{{if .Severity}} ({{.Severity}}){{end}}{{if .Summary}} - {{.Summary}}{{end}}{{if .Error}} ({{.Error}}){{end}}</div>{{if .DeferredRemediations}}<div class="ts">Watchdog actions deferred below the configured severity: {{range .DeferredRemediations}}{{.}} {{end}}</div>{{end}}{{range .Remediations}}<div class="ts">Watchdog action {{.Name}} on {{.Target}}: {{.Status}}{{if .Error}} ({{.Error}}){{end}}</div>{{end}}{{end}}
    {{range .Remediations}}<div class="ts">Remediation {{.Name}} on {{.Target}}: {{.Status}}{{if .Error}} ({{.Error}}){{end}}</div>{{end}}
  </div>
  {{end}}
  {{else}}
  <p class="empty">No alerts have fired yet.</p>
  {{end}}
</div>

{{with .Watchdog}}
<div class="section">
  <h3>AI Watchdog</h3>
  <table><tbody>
    <tr><td class="m-label">State</td><td>{{if .Enabled}}<span class="dot dot-ok"></span>Enabled{{else}}<span class="dot dot-warn"></span>Disabled{{end}}</td></tr>
    <tr><td class="m-label">Model</td><td><code>{{.Model}}</code></td></tr>
    <tr><td class="m-label">Cooldown</td><td>{{.Cooldown}}s per source server</td></tr>
    <tr><td class="m-label">Action Severity</td><td>{{.MinRemediationSeverity}} or higher</td></tr>
    <tr><td class="m-label">Approved Actions</td><td>{{if .AllowedRemediations}}{{range .AllowedRemediations}}<code>{{.}}</code> {{end}}{{else}}<em>advisory only</em>{{end}}</td></tr>
    <tr><td class="m-label">Identifiers</td><td>{{if .IncludeIdentifiers}}included{{else}}redacted{{end}}</td></tr>
  </tbody></table>
</div>
{{end}}

{{if .Remediations}}
<div class="section">
  <h3>Automatic Remediations ({{len .Remediations}})</h3>
  <table>
    <thead><tr><th>Name</th><th>State</th><th>Mode</th><th>Matches</th><th>Targets</th><th>Cooldown</th><th>Attempt Limit</th></tr></thead>
    <tbody>
    {{range .Remediations}}
    <tr>
      <td>{{.Name}}</td>
      <td>{{if .Enabled}}<span class="dot dot-ok"></span>Enabled{{else}}<span class="dot dot-warn"></span>Disabled{{end}}</td>
      <td><code>{{if .Mode}}{{.Mode}}{{else}}alert{{end}}</code></td>
      <td>{{if .Rules}}{{range .Rules}}{{.}} {{end}}{{else if .Metrics}}{{range .Metrics}}{{.}} {{end}}{{else}}<em>all alerts</em>{{end}}</td>
      <td>{{if .Targets}}{{range .Targets}}{{.}} {{end}}{{else}}<em>alert source</em>{{end}}</td>
      <td>{{.Cooldown}}s</td>
      <td>{{.MaxAttempts}} / {{.Window}}s</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

<div class="section">
  <h3>Alert Rules ({{len .Rules}})</h3>
  {{if .Rules}}
  <table>
    <thead><tr><th>Name</th><th>Metric</th><th>Condition</th><th>Threshold</th><th>Scope</th><th>Servers</th><th></th></tr></thead>
    <tbody>
    {{range .Rules}}
    <tr>
      <td>{{.Name}}</td>
      <td><code>{{.Metric}}</code></td>
      <td>{{.Operator}}</td>
      <td>{{.Threshold}}</td>
      <td>{{if .URL}}<code>{{.URL}}</code>{{else if .MountPoint}}<code>{{.MountPoint}}</code>{{else if .Port}}port {{.Port}}{{else}}<em>any</em>{{end}}</td>
      <td>{{if .Servers}}{{range .Servers}}{{.}} {{end}}{{else}}<em>all</em>{{end}}</td>
      <td>
        <form method="post" action="/alerts/remove" style="display:inline">
          <input type="hidden" name="name" value="{{.Name}}">
          <button type="submit" class="btn btn-danger btn-sm" aria-label="Remove alert rule {{.Name}}" data-i18n-aria-prefix="remove_alert_rule" data-i18n-aria-value="{{.Name}}"
            onclick="return confirm('Remove rule {{.Name}}?')" data-i18n="remove">Remove</button>
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
  <div class="form-section-title"><h3 data-i18n="add_alert_rule">Add Alert Rule</h3><span>Choose a template or configure every condition yourself.</span></div>
  <form method="post" action="/alerts/add">
    <div class="form-row">
      <div>
        <label for="alert-template" data-i18n="start_with_template">Start with a template</label>
        <select id="alert-template" aria-describedby="alert-template-note">
          <option value="" data-i18n="custom_rule">Custom rule</option>
          <option value="cpu">High CPU usage</option>
          <option value="disk">Disk almost full</option>
          <option value="http-failed">HTTP health check failed</option>
          <option value="http-slow">Slow HTTP response</option>
          <option value="tls-expiry">TLS certificate expires soon</option>
          <option value="ping-failed">Host is unreachable</option>
        </select>
        <div class="profile-note" id="alert-template-note">Custom rules expose the complete metric catalog.</div>
      </div>
      <div class="mode-advanced">
        <label>Scope</label>
        <p class="restart-note">Leave detailed scope empty to apply the rule to every compatible target.</p>
      </div>
    </div>
    <div class="form-row">
      <div><label for="alert-name">Rule Name *</label><input type="text" id="alert-name" name="name" placeholder="High CPU" required></div>
      <div>
        <label for="alert-metric">Metric *</label>
        <select id="alert-metric" name="metric" required>
          <optgroup label="System">
          <option value="cpu_usage">cpu_usage (%)</option>
          <option value="mem_usage">mem_usage (%)</option>
          <option value="swap_usage">swap_usage (%)</option>
          <option value="disk_usage">disk_usage (%)</option>
          <option value="disk_inode_usage">disk_inode_usage (%)</option>
          <option value="load1">load1</option>
          <option value="load5">load5</option>
          <option value="load15">load15</option>
          <option value="processes_running">processes_running</option>
          <option value="processes_total">processes_total</option>
          <option value="file_descriptor_usage">file_descriptor_usage (%)</option>
          <option value="network_errors">network_errors</option>
          <option value="network_drops">network_drops</option>
          </optgroup>
          <optgroup label="Connectivity">
          <option value="ping_failed">ping_failed</option>
          <option value="ping_latency">ping_latency (ms)</option>
          <option value="ping_loss">ping_loss (%)</option>
          <option value="port_closed">port_closed</option>
          <option value="port_latency">port_latency (ms)</option>
          <option value="banner_failed">banner_failed</option>
          <option value="banner_latency">banner_latency (ms)</option>
          <option value="http_failed">http_failed</option>
          <option value="http_latency">http_latency (ms)</option>
          <option value="cert_expires_days">cert_expires_days (days)</option>
          <option value="dns_failed">dns_failed</option>
          <option value="dns_latency">dns_latency (ms)</option>
          <option value="traceroute_failed">traceroute_failed</option>
          <option value="traceroute_hops">traceroute_hops</option>
          <option value="tls_failed">tls_failed</option>
          <option value="tls_latency">tls_latency (ms)</option>
          <option value="tls_cert_expires_days">tls_cert_expires_days (days)</option>
          <option value="ntp_failed">ntp_failed</option>
          <option value="ntp_latency">ntp_latency (ms)</option>
          <option value="ntp_offset">ntp_offset (ms)</option>
          <option value="custom_failed">custom_failed</option>
          </optgroup>
          <optgroup label="Board">
          <option value="board_temperature">board_temperature (°C)</option>
          <option value="board_under_voltage">board_under_voltage</option>
          <option value="board_throttled">board_throttled</option>
          <option value="board_wifi_rssi">board_wifi_rssi (dBm)</option>
          </optgroup>
        </select>
      </div>
    </div>
    <div class="form-row w3">
      <div>
        <label for="alert-operator">Operator</label>
        <select id="alert-operator" name="operator">
          <option value=">">&gt;</option>
          <option value=">=">&gt;=</option>
          <option value="<">&lt;</option>
          <option value="<=">&lt;=</option>
          <option value="==">==</option>
          <option value="!=">!=</option>
        </select>
      </div>
      <div><label for="alert-threshold">Threshold</label><input type="number" id="alert-threshold" name="threshold" step="any" placeholder="90" required></div>
      <div class="mode-advanced"><label>Mount Point (disk only)</label><input type="text" name="mount_point" placeholder="/"></div>
    </div>
    <div class="form-row mode-advanced">
      <div>
        <label>Limit to Servers <small style="color:var(--text-faint)">(hold Ctrl/⌘ for multi-select; none selected = all servers)</small></label>
        {{if $.ServerNames}}
        <select name="servers" multiple size="4" style="height:auto">
          {{range $.ServerNames}}<option value="{{.}}">{{.}}</option>{{end}}
        </select>
        {{else}}
        <input type="text" name="servers_text" placeholder="web-01, db-01 (empty = all)">
        {{end}}
      </div>
      <div><label>Port (port_closed only)</label><input type="number" name="port" placeholder="80" min="1" max="65535"></div>
      <div><label>HTTP URL (HTTP only)</label><input type="url" name="url" placeholder="https://example.com/health"></div>
    </div>
    <div class="form-actions">
      <button type="submit" class="btn btn-primary" data-i18n="add_rule">Add Rule</button>
    </div>
  </form>
</div>

<script>
(function(){
  var preset = document.getElementById('alert-template');
  if(!preset) return;
  var form = preset.closest('form');
  var note = document.getElementById('alert-template-note');
  var presets = {
    cpu: {name:'High CPU usage', metric:'cpu_usage', operator:'>=', threshold:'90', note:'Warn when CPU usage reaches 90% or more.'},
    disk: {name:'Disk almost full', metric:'disk_usage', operator:'>=', threshold:'90', mount:'/', note:'Warn when the root filesystem reaches 90% usage.'},
    'http-failed': {name:'HTTP health check failed', metric:'http_failed', operator:'>', threshold:'0', note:'Warn when any configured HTTP health check fails.'},
    'http-slow': {name:'Slow HTTP response', metric:'http_latency', operator:'>', threshold:'2000', note:'Warn when an HTTP response takes more than 2 seconds.'},
    'tls-expiry': {name:'TLS certificate expires soon', metric:'tls_cert_expires_days', operator:'<=', threshold:'21', note:'Warn when a TLS certificate has 21 days or less remaining.'},
    'ping-failed': {name:'Host is unreachable', metric:'ping_failed', operator:'>', threshold:'0', note:'Warn when the configured ping probe cannot reach a target.'}
  };
  function setValue(name, value){
    var input = form.querySelector('[name='+name+']');
    if(input && value !== undefined) input.value = value;
  }
  preset.addEventListener('change', function(){
    var selected = presets[preset.value];
    if(!selected){
      if(note) note.textContent = 'Custom rules expose the complete metric catalog.';
      return;
    }
    setValue('name', selected.name);
    setValue('metric', selected.metric);
    setValue('operator', selected.operator);
    setValue('threshold', selected.threshold);
    setValue('mount_point', selected.mount || '');
    if(note) note.textContent = selected.note;
  });
})();
</script>

{{with .EmailCfg}}
<div class="section" style="margin-top:1.25rem">
  <h3>Email Notification Settings</h3>
  <table><tbody>
    <tr><td class="m-label">SMTP Host</td><td>{{.SMTPHost}}:{{.SMTPPort}}</td></tr>
    <tr><td class="m-label">TLS Mode</td><td>{{.TLSMode}}</td></tr>
    <tr><td class="m-label">From</td><td>{{.From}}</td></tr>
    <tr><td class="m-label">To</td><td>{{range .To}}{{.}} {{end}}</td></tr>
  </tbody></table>
  <p style="margin:.75rem 0 0;font-size:.8rem;color:var(--text-faint)">Email settings are configured in the YAML config file under <code>alerts.email</code>.</p>
</div>
{{end}}
{{template "ftr" .}}
{{end}}

{{define "config-page"}}
{{template "hdr" .}}
<div class="page-intro">
  <div><h2>Configuration</h2><p>Set operational defaults for the central WatchSSH service. Server-specific checks, credentials and targets remain on the Servers page.</p></div>
</div>
{{if .Flash}}
<div class="notice {{if .FlashErr}}notice-err{{else}}notice-ok{{end}}">{{.Flash}}</div>
{{end}}
<form method="post" action="/config">

  <div class="config-summary" aria-label="Current configuration summary">
    <div class="summary-item"><span>Polling</span><strong>Every {{.Config.Interval}} seconds</strong></div>
    <div class="summary-item"><span>Workers</span><strong>{{if .Config.Workers}}{{.Config.Workers}} concurrent{{else}}Automatic{{end}}</strong></div>
    <div class="summary-item"><span>History</span><strong>{{if eq .Config.Storage.Type "tinysql"}}tinySQL{{else}}Not stored{{end}}</strong></div>
    <div class="summary-item"><span>Dashboard</span><strong>{{if .Config.Web.Enabled}}{{.Config.Web.Listen}}{{else}}Disabled{{end}}</strong></div>
  </div>

  <div class="notice notice-info">Use strict host-key checking and restricted SSH accounts for all remote systems. Credentials and alert routing remain intentionally outside this global settings form.</div>

  <div class="form-wrap" style="margin-top:0">
    <div class="form-section-title"><h3>Polling &amp; Timing</h3><span>Applies to all configured targets.</span></div>
    <div class="form-row w3">
      <div>
        <label>Poll Interval (seconds)</label>
        <input type="number" name="interval" value="{{.Config.Interval}}" min="5" required>
      </div>
      <div>
        <label>SSH Timeout (seconds)</label>
        <input type="number" name="timeout" value="{{.Config.Timeout}}" min="1" required>
      </div>
      <div class="mode-advanced">
        <label>Max Concurrent Workers <small style="color:var(--text-faint)">(0 = unlimited)</small></label>
        <input type="number" name="workers" value="{{.Config.Workers}}" min="0">
      </div>
    </div>
  </div>

  <div class="form-wrap mode-advanced">
    <div class="form-section-title"><h3>Output</h3><span>For local logs or an external collector.</span></div>
    <div class="form-row">
      <div>
        <label>Output Type</label>
        <select name="output_type">
          <option value="console" {{if eq .Config.Output.Type "console"}}selected{{end}}>console</option>
          <option value="json" {{if eq .Config.Output.Type "json"}}selected{{end}}>json</option>
        </select>
      </div>
      <div>
        <label>JSON Output File <small style="color:var(--text-faint)">(leave blank for stdout)</small></label>
        <input type="text" name="output_file" value="{{.Config.Output.File}}" placeholder="/var/log/watchssh/metrics.json">
      </div>
    </div>
  </div>

  <div class="form-wrap mode-advanced">
    <div class="form-section-title"><h3>History Storage</h3><span>Persist metrics and alert evidence locally.</span></div>
    <p class="restart-note"><strong>Restart required:</strong> storage changes take effect after WatchSSH restarts.</p>
    <div class="form-row w3">
      <div>
        <label>Storage Type</label>
        <select name="storage_type">
          <option value="none" {{if eq .Config.Storage.Type "none"}}selected{{end}}>none</option>
          <option value="tinysql" {{if eq .Config.Storage.Type "tinysql"}}selected{{end}}>tinysql</option>
        </select>
      </div>
      <div>
        <label>tinySQL Database File</label>
        <input type="text" name="storage_path" value="{{.Config.Storage.Path}}" placeholder="./watchssh.tinysql">
      </div>
      <div>
        <label>Retention Days <small style="color:var(--text-faint)">(0 = disabled)</small></label>
        <input type="number" name="storage_retention_days" value="{{.Config.Storage.RetentionDays}}" min="0">
      </div>
    </div>
    <div class="form-row">
      <div>
        <label>Max Size MB <small style="color:var(--text-faint)">(0 = disabled)</small></label>
        <input type="number" name="storage_max_size_mb" value="{{.Config.Storage.MaxSizeMB}}" min="0">
      </div>
      <div></div>
    </div>
  </div>

  <div class="form-wrap">
    <div class="form-section-title"><h3>Web Dashboard</h3><span>Local operator interface and API endpoints.</span></div>
    <p class="restart-note"><strong>Restart required:</strong> a listen-address change takes effect after WatchSSH restarts. For shared access, configure <code>web.auth</code> with a bcrypt password hash in YAML or place the dashboard behind an authenticated TLS reverse proxy.</p>
    <div class="form-row">
      <div>
        <label>Enable Web Dashboard</label>
        <select name="web_enabled">
          <option value="1" {{if .Config.Web.Enabled}}selected{{end}}>Enabled</option>
          <option value="0" {{if not .Config.Web.Enabled}}selected{{end}}>Disabled</option>
        </select>
      </div>
      <div class="mode-advanced">
        <label>Listen Address</label>
        <input type="text" name="web_listen" value="{{.Config.Web.Listen}}" placeholder=":8080">
      </div>
    </div>
  </div>

  <div class="form-wrap">
    <div class="form-section-title"><h3>SSH Security</h3><span>Global trust policy for remote connections.</span></div>
    <div class="form-row">
      <div class="mode-expert">
        <label>Known Hosts File <small style="color:var(--text-faint)">(blank = ~/.ssh/known_hosts)</small></label>
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
