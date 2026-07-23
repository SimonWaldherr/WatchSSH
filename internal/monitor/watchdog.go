package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// WatchdogManager limits calls to an external model endpoint per source
// server. It deliberately keeps no conversational state and never exposes a
// tool interface to the model.
type WatchdogManager struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func NewWatchdogManager() *WatchdogManager {
	return &WatchdogManager{last: make(map[string]time.Time)}
}

func (wm *WatchdogManager) allow(cfg config.WatchdogConfig, server string, now time.Time) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if last := wm.last[server]; !last.IsZero() && now.Sub(last) < time.Duration(cfg.Cooldown)*time.Second {
		return false
	}
	wm.last[server] = now
	return true
}

type watchdogInput struct {
	SchemaVersion    string                `json:"schema_version"`
	ObservedAt       time.Time             `json:"observed_at"`
	Alert            watchdogAlert         `json:"alert"`
	Probe            watchdogProbeSnapshot `json:"probe_snapshot"`
	AvailableActions []watchdogAction      `json:"available_actions"`
}

type watchdogAlert struct {
	Rule   string  `json:"rule"`
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
}

// watchdogProbeSnapshot intentionally excludes process lists, logged-in users,
// custom command output, SSH credentials, and raw banners. Identifiers are
// pseudonymous unless include_identifiers is explicitly enabled.
type watchdogProbeSnapshot struct {
	ServerRef        string          `json:"server_ref"`
	Platform         string          `json:"platform,omitempty"`
	CollectionError  string          `json:"collection_error,omitempty"`
	CPUUsage         *float64        `json:"cpu_usage_percent,omitempty"`
	MemoryUsage      *float64        `json:"memory_usage_percent,omitempty"`
	Load1            *float64        `json:"load1,omitempty"`
	Probes           []watchdogProbe `json:"probes,omitempty"`
	ProbeCount       int             `json:"probe_count"`
	FailedProbeCount int             `json:"failed_probe_count"`
	FailedProbeTypes []string        `json:"failed_probe_types,omitempty"`
}

type watchdogProbe struct {
	Type            string   `json:"type"`
	Reference       string   `json:"reference"`
	OK              bool     `json:"ok"`
	LatencyMs       float64  `json:"latency_ms,omitempty"`
	StatusCode      int      `json:"status_code,omitempty"`
	PacketLoss      float64  `json:"packet_loss_percent,omitempty"`
	Hops            int      `json:"hops,omitempty"`
	CertExpiresDays *float64 `json:"cert_expires_days,omitempty"`
	OffsetMs        float64  `json:"offset_ms,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type watchdogAction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type chatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens"`
	ResponseFormat any           `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type watchdogDecision struct {
	Summary      string   `json:"summary"`
	Severity     string   `json:"severity"`
	Remediations []string `json:"remediations"`
}

func (m *Monitor) runWatchdog(cfg *config.Config, metrics []ServerMetrics, firings []Firing) {
	watchdog := cfg.Alerts.Watchdog
	if watchdog == nil || !watchdog.Enabled || len(firings) == 0 {
		return
	}
	if m.watchdogMgr == nil {
		m.watchdogMgr = NewWatchdogManager()
	}
	byServer := make(map[string]ServerMetrics, len(metrics))
	for _, metric := range metrics {
		byServer[metric.ServerName] = metric
	}
	runbooks := watchdogRunbooks(*watchdog, cfg.Alerts.Remediations)

	for index := range firings {
		firing := &firings[index]
		startedAt := time.Now()
		result := WatchdogResult{Model: watchdog.Model, StartedAt: startedAt}
		if !m.watchdogMgr.allow(*watchdog, firing.Server, startedAt) {
			result.Status = "skipped_cooldown"
			firing.Watchdog = &result
			continue
		}
		metric, exists := byServer[firing.Server]
		if !exists {
			result.Status = "failed"
			result.Error = "no current probe snapshot exists for the firing server"
			result.DurationMs = durationMilliseconds(startedAt)
			firing.Watchdog = &result
			continue
		}

		decision, err := requestWatchdogDecision(*watchdog, metric, *firing, runbooks)
		result.DurationMs = durationMilliseconds(startedAt)
		if err != nil {
			result.Status = "failed"
			result.Error = abbreviateWatchdogText(err.Error(), 1024)
			firing.Watchdog = &result
			continue
		}
		result.Status = "analyzed"
		result.Severity = decision.Severity
		result.Summary = abbreviateWatchdogText(decision.Summary, 1024)
		seen := make(map[string]struct{}, len(decision.Remediations))
		for _, name := range decision.Remediations {
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			remediation, allowed := runbooks[name]
			if !allowed {
				result.RejectedRemediations = append(result.RejectedRemediations, name)
				continue
			}
			// AI output is intentionally advisory. A human must review the
			// recommendation and invoke the runbook through normal operations.
			result.RecommendedRemediations = append(result.RecommendedRemediations, remediation.Name)
		}
		firing.Watchdog = &result
	}
}

func watchdogRunbooks(cfg config.WatchdogConfig, remediations []config.RemediationConfig) map[string]config.RemediationConfig {
	all := make(map[string]config.RemediationConfig, len(remediations))
	for _, remediation := range remediations {
		all[remediation.Name] = remediation
	}
	runbooks := make(map[string]config.RemediationConfig, len(cfg.AllowedRemediations))
	for _, name := range cfg.AllowedRemediations {
		if remediation, exists := all[name]; exists && remediation.Enabled && remediation.Mode == "watchdog" {
			runbooks[name] = remediation
		}
	}
	return runbooks
}

func requestWatchdogDecision(cfg config.WatchdogConfig, metric ServerMetrics, firing Firing, runbooks map[string]config.RemediationConfig) (watchdogDecision, error) {
	input := watchdogInput{
		SchemaVersion: "1",
		ObservedAt:    metric.Timestamp,
		Alert: watchdogAlert{
			Rule: firing.RuleName, Metric: firing.Metric, Value: firing.Value,
		},
		Probe: buildWatchdogProbeSnapshot(metric, cfg.IncludeIdentifiers),
	}
	runbookNames := make([]string, 0, len(runbooks))
	for name := range runbooks {
		runbookNames = append(runbookNames, name)
	}
	sort.Strings(runbookNames)
	for _, name := range runbookNames {
		remediation := runbooks[name]
		input.AvailableActions = append(input.AvailableActions, watchdogAction{Name: remediation.Name, Description: remediation.Description})
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return watchdogDecision{}, fmt.Errorf("encoding watchdog input: %w", err)
	}
	if len(inputJSON) > cfg.MaxInputBytes {
		return watchdogDecision{}, fmt.Errorf("watchdog input is %d bytes, exceeding max_input_bytes %d", len(inputJSON), cfg.MaxInputBytes)
	}

	request := chatCompletionRequest{
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: watchdogSystemPrompt(cfg.SystemPrompt)},
			{Role: "user", Content: "Analyze this WatchSSH alert and probe snapshot. Return only the requested JSON decision.\n" + string(inputJSON)},
		},
	}
	if cfg.ResponseFormat == "json_schema" {
		request.ResponseFormat = watchdogJSONSchema()
	} else {
		request.ResponseFormat = map[string]string{"type": "json_object"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return watchdogDecision{}, fmt.Errorf("encoding watchdog request: %w", err)
	}
	endpoint, err := watchdogEndpoint(cfg)
	if err != nil {
		return watchdogDecision{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return watchdogDecision{}, fmt.Errorf("creating watchdog request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("User-Agent", "WatchSSH/2 watchdog")
	if cfg.APIKeyEnv != "" {
		apiKey, ok := os.LookupEnv(cfg.APIKeyEnv)
		if !ok || apiKey == "" {
			return watchdogDecision{}, fmt.Errorf("watchdog API key environment variable %q is not set", cfg.APIKeyEnv)
		}
		httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := (&http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(httpRequest)
	if err != nil {
		return watchdogDecision{}, fmt.Errorf("calling watchdog API: %w", err)
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return watchdogDecision{}, fmt.Errorf("reading watchdog response: %w", readErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return watchdogDecision{}, fmt.Errorf("watchdog API returned %s: %s", response.Status, abbreviateWatchdogText(string(responseBody), 1024))
	}
	var completion chatCompletionResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return watchdogDecision{}, fmt.Errorf("decoding watchdog completion: %w", err)
	}
	if len(completion.Choices) == 0 || strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
		return watchdogDecision{}, fmt.Errorf("watchdog completion did not contain a message")
	}
	var decision watchdogDecision
	if err := json.Unmarshal([]byte(stripJSONCodeFence(completion.Choices[0].Message.Content)), &decision); err != nil {
		return watchdogDecision{}, fmt.Errorf("decoding watchdog decision: %w", err)
	}
	if err := validateWatchdogDecision(decision); err != nil {
		return watchdogDecision{}, err
	}
	return decision, nil
}

func buildWatchdogProbeSnapshot(metric ServerMetrics, includeIdentifiers bool) watchdogProbeSnapshot {
	snapshot := watchdogProbeSnapshot{
		ServerRef:       watchdogIdentifier(metric.ServerName, "server", includeIdentifiers),
		Platform:        metric.Platform,
		CollectionError: watchdogError(metric.Error, includeIdentifiers),
	}
	if metric.CPU != nil {
		value := metric.CPU.UsagePercent
		snapshot.CPUUsage = &value
	}
	if metric.Memory != nil {
		value := metric.Memory.UsagePercent
		snapshot.MemoryUsage = &value
	}
	if metric.Load != nil {
		value := metric.Load.Load1
		snapshot.Load1 = &value
	}
	if metric.Connectivity.PingEnabled {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{
			Type: "ping", Reference: "ping", OK: metric.Connectivity.PingOK,
			LatencyMs: metric.Connectivity.PingLatency, PacketLoss: metric.Connectivity.PingLoss,
		})
	}
	for index, probe := range metric.Connectivity.Ports {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "tcp", Reference: watchdogIdentifier(fmt.Sprintf("port-%d", probe.Port), fmt.Sprintf("tcp-%d", index+1), includeIdentifiers), OK: probe.Open, LatencyMs: probe.LatencyMs, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.Connectivity.Banner {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "banner", Reference: watchdogIdentifier(probe.Name, fmt.Sprintf("banner-%d", index+1), includeIdentifiers), OK: probe.OK, LatencyMs: probe.LatencyMs, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.Connectivity.HTTP {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "http", Reference: watchdogIdentifier(probe.URL, fmt.Sprintf("http-%d", index+1), includeIdentifiers), OK: probe.OK, LatencyMs: probe.LatencyMs, StatusCode: probe.StatusCode, CertExpiresDays: probe.CertExpiresDays, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.Connectivity.DNS {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "dns", Reference: watchdogIdentifier(probe.Name, fmt.Sprintf("dns-%d", index+1), includeIdentifiers), OK: probe.OK, LatencyMs: probe.LatencyMs, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.Connectivity.Traceroute {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "traceroute", Reference: watchdogIdentifier(probe.Name, fmt.Sprintf("traceroute-%d", index+1), includeIdentifiers), OK: probe.OK, LatencyMs: probe.LatencyMs, Hops: probe.Hops, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.Connectivity.TLS {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "tls", Reference: watchdogIdentifier(probe.Name, fmt.Sprintf("tls-%d", index+1), includeIdentifiers), OK: probe.OK, LatencyMs: probe.LatencyMs, CertExpiresDays: probe.CertExpiresDays, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.Connectivity.NTP {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "ntp", Reference: watchdogIdentifier(probe.Name, fmt.Sprintf("ntp-%d", index+1), includeIdentifiers), OK: probe.OK, LatencyMs: probe.LatencyMs, OffsetMs: probe.OffsetMs, Error: watchdogError(probe.Error, includeIdentifiers)})
	}
	for index, probe := range metric.CustomChecks {
		snapshot.Probes = append(snapshot.Probes, watchdogProbe{Type: "custom", Reference: watchdogIdentifier(probe.Name, fmt.Sprintf("custom-%d", index+1), includeIdentifiers), OK: probe.OK})
	}
	snapshot.ProbeCount = len(snapshot.Probes)
	failedTypes := make(map[string]struct{})
	for _, probe := range snapshot.Probes {
		if probe.OK {
			continue
		}
		snapshot.FailedProbeCount++
		failedTypes[probe.Type] = struct{}{}
	}
	for probeType := range failedTypes {
		snapshot.FailedProbeTypes = append(snapshot.FailedProbeTypes, probeType)
	}
	sort.Strings(snapshot.FailedProbeTypes)
	return snapshot
}

func watchdogSystemPrompt(extra string) string {
	prompt := "You are a conservative WatchSSH operations advisor. Analyze only the provided telemetry. Return a JSON object with summary (max 500 characters), severity (info, warning, or critical), and remediations (an array of recommended runbook names). Recommendations are advisory and require human approval; never imply that an action was executed. Recommend a runbook only when telemetry strongly supports it. The reported severity must reflect the evidence, not the desired action. You may use only names listed in available_actions. Never invent commands, endpoints, or tool calls. An empty remediations array is preferred when uncertain."
	if strings.TrimSpace(extra) != "" {
		prompt += "\nAdditional operator policy:\n" + extra
	}
	return prompt
}

func watchdogJSONSchema() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "watchssh_watchdog_decision",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"summary":      map[string]any{"type": "string", "maxLength": 500},
					"severity":     map[string]any{"type": "string", "enum": []string{"info", "warning", "critical"}},
					"remediations": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 8},
				},
				"required": []string{"summary", "severity", "remediations"},
			},
		},
	}
}

func watchdogEndpoint(cfg config.WatchdogConfig) (string, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if cfg.BaseURLEnv != "" {
		baseURL = strings.TrimSpace(os.Getenv(cfg.BaseURLEnv))
		if baseURL == "" {
			return "", fmt.Errorf("watchdog base URL environment variable %q is not set", cfg.BaseURLEnv)
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL, nil
	}
	return baseURL + "/chat/completions", nil
}

func validateWatchdogDecision(decision watchdogDecision) error {
	if strings.TrimSpace(decision.Summary) == "" {
		return fmt.Errorf("watchdog decision summary is required")
	}
	if decision.Severity != "info" && decision.Severity != "warning" && decision.Severity != "critical" {
		return fmt.Errorf("watchdog decision severity %q is invalid", decision.Severity)
	}
	return nil
}

func watchdogIdentifier(value, fallback string, include bool) string {
	if include && strings.TrimSpace(value) != "" {
		return abbreviateWatchdogText(value, 512)
	}
	return fallback
}

func watchdogError(value string, include bool) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if !include {
		return "probe failed"
	}
	return abbreviateWatchdogText(value, 512)
}

func abbreviateWatchdogText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit-3] + "..."
}

func stripJSONCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSpace(content)
	return strings.TrimSpace(strings.TrimSuffix(content, "```"))
}

func durationMilliseconds(startedAt time.Time) float64 {
	return float64(time.Since(startedAt).Microseconds()) / 1000
}
