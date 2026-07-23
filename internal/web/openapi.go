package web

import (
	"net/http"
)

// openAPISpec documents the stable, versioned API without making the dashboard
// depend on a third-party Swagger UI bundle. Consumers can load this document
// in their preferred OpenAPI or Swagger tooling.
const openAPISpec = `{
  "openapi": "3.1.0",
  "info": {
    "title": "WatchSSH API",
    "version": "v1",
    "description": "Agentless monitoring data and probe results collected by WatchSSH. Unversioned /api paths are compatibility aliases; new integrations should use /api/v1."
  },
  "paths": {
    "/healthz": {"get": {"summary": "Liveness probe", "responses": {"200": {"description": "WatchSSH process is alive"}}}},
    "/livez": {"get": {"summary": "Kubernetes-compatible liveness alias", "responses": {"200": {"description": "WatchSSH process is alive"}}}},
    "/readyz": {"get": {"summary": "Readiness probe", "responses": {"200": {"description": "Initial monitoring data is available"}, "503": {"description": "WatchSSH is not ready"}}}},
    "/metrics": {"get": {"summary": "Prometheus metrics", "responses": {"200": {"description": "Prometheus text exposition"}}}},
    "/api/v1/metrics": {"get": {"summary": "Current server metrics", "responses": {"200": {"description": "Current metrics", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/ServerMetrics"}}}}}, "401": {"$ref": "#/components/responses/Unauthorized"}}}},
    "/api/v1/probes": {"get": {"summary": "Current connectivity probe results", "parameters": [{"$ref": "#/components/parameters/Server"}], "responses": {"200": {"description": "Probe results", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/ProbeResult"}}}}}, "401": {"$ref": "#/components/responses/Unauthorized"}}}},
    "/api/v1/inventory": {"get": {"summary": "Non-secret asset and SSH inventory", "responses": {"200": {"description": "Configured assets and observed facts", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/AssetRecord"}}}}}, "401": {"$ref": "#/components/responses/Unauthorized"}}}},
    "/api/v1/security/findings": {"get": {"summary": "SSH posture and observed TLS findings", "responses": {"200": {"description": "Current security findings", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/SecurityFinding"}}}}}, "401": {"$ref": "#/components/responses/Unauthorized"}}}},
    "/api/v1/history/metrics": {"get": {"summary": "Persisted metric samples", "parameters": [{"$ref": "#/components/parameters/Server"}, {"$ref": "#/components/parameters/Limit"}], "responses": {"200": {"description": "Metric samples"}, "401": {"$ref": "#/components/responses/Unauthorized"}, "503": {"$ref": "#/components/responses/Problem"}}}},
    "/api/v1/history/alerts": {"get": {"summary": "Persisted alert firings", "parameters": [{"$ref": "#/components/parameters/Limit"}], "responses": {"200": {"description": "Alert firings"}, "401": {"$ref": "#/components/responses/Unauthorized"}, "503": {"$ref": "#/components/responses/Problem"}}}},
    "/api/v1/history/summary": {"get": {"summary": "Per-server history summary", "parameters": [{"$ref": "#/components/parameters/Server"}, {"$ref": "#/components/parameters/Limit"}], "responses": {"200": {"description": "History summary"}, "401": {"$ref": "#/components/responses/Unauthorized"}, "503": {"$ref": "#/components/responses/Problem"}}}},
    "/api/v1/test-connection": {"post": {"summary": "Test an SSH connection", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ConnectionTestRequest"}}}}, "responses": {"200": {"description": "Connection outcome"}, "401": {"$ref": "#/components/responses/Unauthorized"}, "405": {"$ref": "#/components/responses/Problem"}}}}
  },
  "components": {
    "parameters": {
      "Server": {"name": "server", "in": "query", "schema": {"type": "string"}, "description": "Exact WatchSSH server name."},
      "Limit": {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 500, "default": 100}}
    },
    "responses": {
      "Unauthorized": {"description": "HTTP Basic authentication is required."},
      "Problem": {"description": "RFC 9457 problem response", "content": {"application/problem+json": {"schema": {"$ref": "#/components/schemas/Problem"}}}}
    },
    "schemas": {
      "ServerMetrics": {"type": "object", "required": ["server_name", "timestamp"], "properties": {"server_name": {"type": "string"}, "host": {"type": "string"}, "timestamp": {"type": "string", "format": "date-time"}, "platform": {"type": "string"}, "error": {"type": "string"}}},
      "ProbeResult": {"type": "object", "required": ["server", "connectivity"], "properties": {"server": {"type": "string"}, "host": {"type": "string"}, "connectivity": {"type": "object"}}},
      "AssetRecord": {"type": "object", "required": ["name", "host"], "properties": {"name": {"type": "string"}, "host": {"type": "string"}, "platform": {"type": "string"}, "architecture": {"type": "string"}, "tags": {"type": "array", "items": {"type": "string"}}, "depends_on": {"type": "array", "items": {"type": "string"}}, "proxy_jump": {"type": "boolean"}, "local": {"type": "boolean"}}},
      "SecurityFinding": {"type": "object", "required": ["server", "severity", "code", "summary"], "properties": {"server": {"type": "string"}, "severity": {"type": "string", "enum": ["info", "warning"]}, "code": {"type": "string"}, "summary": {"type": "string"}}},
      "ConnectionTestRequest": {"type": "object", "properties": {"host": {"type": "string"}, "port": {"type": "integer", "default": 22}, "username": {"type": "string"}, "auth_type": {"type": "string", "enum": ["key", "password", "agent"]}, "credential": {"type": "string", "writeOnly": true}, "local": {"type": "boolean"}}},
      "Problem": {"type": "object", "required": ["type", "title", "status", "detail", "instance"], "properties": {"type": {"type": "string", "format": "uri-reference"}, "title": {"type": "string"}, "status": {"type": "integer"}, "detail": {"type": "string"}, "instance": {"type": "string"}, "request_id": {"type": "string"}}}
    }
  }
}`

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(openAPISpec))
}
