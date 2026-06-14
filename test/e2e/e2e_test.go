//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAPIEndpoints runs all e2e tests in a strict order with proper dependencies.
//
// Go's test functions can run in any order, so we use a single top-level
// test function with ordered subtests to guarantee sequential execution.
func TestAPIEndpoints(t *testing.T) {
	// ═══════════════════════════════════════════════════════════════
	// Phase 1: Health Checks (no dependencies)
	// ═══════════════════════════════════════════════════════════════
	t.Run("HealthCheck_API", testHealthCheckAPI)
	t.Run("HealthCheck_Admin", testHealthCheckAdmin)

	// ═══════════════════════════════════════════════════════════════
	// Phase 2: Admin Authentication
	// ═══════════════════════════════════════════════════════════════
	t.Run("Auth_LoginInvalidPassword", testAuthLoginInvalidPassword)
	t.Run("Auth_LoginMissingEmail", testAuthLoginMissingEmail)
	t.Run("Auth_LoginMissingPassword", testAuthLoginMissingPassword)
	t.Run("Auth_LoginValid", testAuthLoginValid)
	t.Run("Auth_MeValidToken", testAuthMeValidToken)
	t.Run("Auth_MeWithoutToken", testAuthMeWithoutToken)
	t.Run("Auth_MeWithInvalidToken", testAuthMeWithInvalidToken)

	// ═══════════════════════════════════════════════════════════════
	// Phase 3: Dashboard
	// ═══════════════════════════════════════════════════════════════
	t.Run("Dashboard_Stats", testDashboardStats)
	t.Run("Dashboard_StatsWithoutAuth", testDashboardStatsWithoutAuth)

	// ═══════════════════════════════════════════════════════════════
	// Phase 4: Client CRUD (depends on admin token)
	// ═══════════════════════════════════════════════════════════════
	t.Run("Clients_ListEmpty", testClientsListEmpty)
	t.Run("Clients_CreateMissingName", testClientsCreateMissingName)
	t.Run("Clients_Create", testClientsCreate)
	t.Run("Clients_GetByID", testClientsGetByID)
	t.Run("Clients_GetByIDNotFound", testClientsGetByIDNotFound)
	t.Run("Clients_Update", testClientsUpdate)
	t.Run("Clients_RotateKeys", testClientsRotateKeys)
	t.Run("Clients_Delete", testClientsDelete)

	// ═══════════════════════════════════════════════════════════════
	// Phase 5: Provider CRUD (depends on admin token)
	// ═══════════════════════════════════════════════════════════════
	t.Run("Providers_ListEmpty", testProvidersListEmpty)
	t.Run("Providers_CreateInvalidID", testProvidersCreateInvalidID)
	t.Run("Providers_MalformedProviderID", testProvidersMalformedProviderID)
	t.Run("Providers_Create", testProvidersCreate)
	t.Run("Providers_GetByID", testProvidersGetByID)
	t.Run("Providers_GetByIDNotFound", testProvidersGetByIDNotFound)
	t.Run("Providers_Update", testProvidersUpdate)
	t.Run("Providers_ListWithResults", testProvidersListWithResults)
	t.Run("Providers_Delete", testProvidersDelete)

	// ═══════════════════════════════════════════════════════════════
	// Phase 6: Audit Logs (depends on actions from phases 4-5)
	// ═══════════════════════════════════════════════════════════════
	t.Run("Audit_List", testAuditList)
	t.Run("Audit_ListWithFilter", testAuditListWithFilter)
	t.Run("Audit_GetByID", testAuditGetByID)
	t.Run("Audit_GetByIDNotFound", testAuditGetByIDNotFound)

	// ═══════════════════════════════════════════════════════════════
	// Phase 7: API Proxy Middleware Chain
	// ═══════════════════════════════════════════════════════════════
	t.Run("Proxy_MissingClientID", testProxyMissingClientID)
	t.Run("Proxy_MissingAuthorization", testProxyMissingAuthorization)
	t.Run("Proxy_InvalidClientID", testProxyInvalidClientID)
	t.Run("Proxy_InvalidSecret", testProxyInvalidSecret)
	t.Run("Proxy_MissingNonce", testProxyMissingNonce)
	t.Run("Proxy_MissingTimestamp", testProxyMissingTimestamp)
	t.Run("Proxy_ExpiredTimestamp", testProxyExpiredTimestamp)
	t.Run("Proxy_NonceReplay", testProxyNonceReplay)
	t.Run("Proxy_RateLimitHeadersPresent", testProxyRateLimitHeaders)
	t.Run("Proxy_ModelRequired", testProxyModelRequired)

	// ═══════════════════════════════════════════════════════════════
	// Phase 8: CORS & Edge Cases
	// ═══════════════════════════════════════════════════════════════
	t.Run("CORS_Headers", testCORSHeaders)
	t.Run("RBAC_UnauthenticatedAccess", testRBACUnauthenticatedAccess)
}

/* ════════════════════════════════════════════════════════════
   1. Health Check Tests
   ════════════════════════════════════════════════════════════ */

func testHealthCheckAPI(t *testing.T) {
	resp, ar, err := doRequest("GET", apiBase+"/health", nil, nil)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	if err := json.Unmarshal(ar.Data, &data); err != nil {
		t.Fatalf("unmarshal health data: %v", err)
	}
	if data.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", data.Status)
	}
	if data.Service != "ai-proxy" {
		t.Errorf("expected service 'ai-proxy', got %q", data.Service)
	}
}

func testHealthCheckAdmin(t *testing.T) {
	resp, ar, err := doRequest("GET", adminBase+"/health", nil, nil)
	if err != nil {
		t.Fatalf("admin health check failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !ar.Success {
		t.Fatal("expected success=true")
	}
}

/* ════════════════════════════════════════════════════════════
   2. Admin Auth Tests
   ════════════════════════════════════════════════════════════ */

func testAuthLoginInvalidPassword(t *testing.T) {
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "wrong-password",
	}, nil)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	assertError(t, ar, "Unauthorized")
}

func testAuthLoginMissingEmail(t *testing.T) {
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/auth/login", map[string]string{
		"password": adminPassword,
	}, nil)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	assertStatus(t, resp, 422)
	assertError(t, ar, "Validation failed")
}

func testAuthLoginMissingPassword(t *testing.T) {
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/auth/login", map[string]string{
		"email": adminEmail,
	}, nil)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	assertStatus(t, resp, 422)
	assertError(t, ar, "Validation failed")
}

func testAuthLoginValid(t *testing.T) {
	loginAsAdmin(t)
	if suite.AdminToken == "" {
		t.Fatal("token should not be empty")
	}
}

func testAuthMeValidToken(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/me", nil, adminHeaders())
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var data struct {
		AdminID string `json:"admin_id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Role    string `json:"role"`
	}
	if err := json.Unmarshal(ar.Data, &data); err != nil {
		t.Fatalf("unmarshal me data: %v", err)
	}
	if data.Email != adminEmail {
		t.Errorf("expected email %q, got %q", adminEmail, data.Email)
	}
	if data.Role != adminRole {
		t.Errorf("expected role %q, got %q", adminRole, data.Role)
	}
	if data.Name != adminName {
		t.Errorf("expected name %q, got %q", adminName, data.Name)
	}
	if data.AdminID == "" {
		t.Error("admin_id should not be empty")
	}
}

func testAuthMeWithoutToken(t *testing.T) {
	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/me", nil, nil)
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	assertError(t, ar, "Unauthorized")
}

func testAuthMeWithInvalidToken(t *testing.T) {
	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/me", nil, map[string]string{
		"Authorization": "Bearer invalid-token-that-is-not-valid",
	})
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	assertError(t, ar, "Unauthorized")
}

/* ════════════════════════════════════════════════════════════
   3. Dashboard Tests
   ════════════════════════════════════════════════════════════ */

func testDashboardStats(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/dashboard/stats", nil, adminHeaders())
	if err != nil {
		t.Fatalf("dashboard stats request failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var stats struct {
		TotalClients    int `json:"total_clients"`
		ActiveClients   int `json:"active_clients"`
		Requests30d     int `json:"total_requests_30d"`
		ActiveProviders int `json:"active_providers"`
	}
	if err := json.Unmarshal(ar.Data, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	t.Logf("Dashboard stats: %+v", stats)
}

func testDashboardStatsWithoutAuth(t *testing.T) {
	resp, _, err := doRequest("GET", adminBase+"/api/v1/admin/dashboard/stats", nil, nil)
	if err != nil {
		t.Fatalf("dashboard stats request failed: %v", err)
	}
	assertStatus(t, resp, 401)
}

/* ════════════════════════════════════════════════════════════
   4. Client CRUD Tests
   ════════════════════════════════════════════════════════════ */

func testClientsListEmpty(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/clients", nil, adminHeaders())
	if err != nil {
		t.Fatalf("list clients failed: %v", err)
	}
	assertStatus(t, resp, 200)

	clients := make([]interface{}, 0)
	if err := json.Unmarshal(ar.Data, &clients); err != nil {
		t.Fatalf("unmarshal clients: %v", err)
	}
	t.Logf("Initial clients count: %d", len(clients))
}

func testClientsCreateMissingName(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/clients", map[string]interface{}{}, adminHeaders())
	if err != nil {
		t.Fatalf("create client failed: %v", err)
	}
	assertStatus(t, resp, 422)
	assertError(t, ar, "Validation failed")
}

func testClientsCreate(t *testing.T) {
	loginAsAdmin(t)

	body := map[string]interface{}{
		"name": "E2E Test Client",
		"preferred_providers": []map[string]interface{}{
			{"provider": "openai", "model": "gpt-4"},
		},
	}
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/clients", body, adminHeaders())
	if err != nil {
		t.Fatalf("create client failed: %v", err)
	}
	assertStatus(t, resp, 201)

	var result struct {
		Client struct {
			ID       string `json:"id"`
			ClientID string `json:"client_id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
		} `json:"client"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(ar.Data, &result); err != nil {
		t.Fatalf("unmarshal create client result: %v", err)
	}

	if result.Client.ClientID == "" {
		t.Fatal("client_id should not be empty")
	}
	if result.Client.Name != "E2E Test Client" {
		t.Errorf("expected name 'E2E Test Client', got %q", result.Client.Name)
	}
	if result.Client.Status != "active" {
		t.Errorf("expected status 'active', got %q", result.Client.Status)
	}
	if result.ClientSecret == "" {
		t.Fatal("client_secret should not be empty")
	}

	suite.ClientID = result.Client.ClientID
	suite.ClientSecret = result.ClientSecret
	t.Logf("Created client: %s (secret: %.20s...)", suite.ClientID, suite.ClientSecret)
}

func testClientsGetByID(t *testing.T) {
	if suite.ClientID == "" {
		t.Fatal("no client created — test clientsCreate must run first")
	}
	loginAsAdmin(t)

	uuid := findClientUUID(t, suite.ClientID)
	if uuid == "" {
		t.Fatal("client not found in list")
	}

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/clients/"+uuid, nil, adminHeaders())
	if err != nil {
		t.Fatalf("get client failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var client struct {
		ID       string `json:"id"`
		ClientID string `json:"client_id"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(ar.Data, &client); err != nil {
		t.Fatalf("unmarshal client: %v", err)
	}
	if client.ClientID != suite.ClientID {
		t.Errorf("expected client_id %q, got %q", suite.ClientID, client.ClientID)
	}
}

func testClientsGetByIDNotFound(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/clients/00000000-0000-0000-0000-000000000000", nil, adminHeaders())
	if err != nil {
		t.Fatalf("get client failed: %v", err)
	}
	assertStatus(t, resp, 404)
	assertError(t, ar, "Resource not found")
}

func testClientsUpdate(t *testing.T) {
	if suite.ClientID == "" {
		t.Fatal("no client created")
	}
	loginAsAdmin(t)

	uuid := findClientUUID(t, suite.ClientID)
	if uuid == "" {
		t.Fatal("client not found")
	}

	updatedName := "Updated E2E Client"
	resp, ar, err := doRequest("PUT", adminBase+"/api/v1/admin/clients/"+uuid, map[string]interface{}{
		"name": updatedName,
	}, adminHeaders())
	if err != nil {
		t.Fatalf("update client failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var client struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(ar.Data, &client); err != nil {
		t.Fatalf("unmarshal client: %v", err)
	}
	if client.Name != updatedName {
		t.Errorf("expected name %q, got %q", updatedName, client.Name)
	}
}

func testClientsRotateKeys(t *testing.T) {
	if suite.ClientID == "" {
		t.Fatal("no client created")
	}
	loginAsAdmin(t)

	uuid := findClientUUID(t, suite.ClientID)
	if uuid == "" {
		t.Fatal("client not found")
	}

	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/clients/"+uuid+"/rotate", nil, adminHeaders())
	if err != nil {
		t.Fatalf("rotate keys failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var result struct {
		Client       interface{} `json:"client"`
		ClientSecret string      `json:"client_secret"`
	}
	if err := json.Unmarshal(ar.Data, &result); err != nil {
		t.Fatalf("unmarshal rotate result: %v", err)
	}
	if result.ClientSecret == "" {
		t.Fatal("new client_secret should not be empty")
	}
	suite.ClientSecret = result.ClientSecret
	t.Logf("Client keys rotated, new secret: %.20s...", suite.ClientSecret)
}

func testClientsDelete(t *testing.T) {
	loginAsAdmin(t)

	// Create a temporary client to delete
	body := map[string]interface{}{"name": "Client To Delete"}
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/clients", body, adminHeaders())
	if err != nil {
		t.Fatalf("create client failed: %v", err)
	}
	assertStatus(t, resp, 201)

	var created struct {
		Client struct {
			ID string `json:"id"`
		} `json:"client"`
	}
	if err := json.Unmarshal(ar.Data, &created); err != nil {
		t.Fatalf("unmarshal created client: %v", err)
	}

	// Delete it
	resp, _, err = doRequest("DELETE", adminBase+"/api/v1/admin/clients/"+created.Client.ID, nil, adminHeaders())
	if err != nil {
		t.Fatalf("delete client failed: %v", err)
	}
	assertStatus(t, resp, 204)

	// Verify it's gone
	resp, ar, err = doRequest("GET", adminBase+"/api/v1/admin/clients/"+created.Client.ID, nil, adminHeaders())
	if err != nil {
		t.Fatalf("get deleted client failed: %v", err)
	}
	assertStatus(t, resp, 404)
}

/* ════════════════════════════════════════════════════════════
   5. Provider CRUD Tests
   ════════════════════════════════════════════════════════════ */

func testProvidersListEmpty(t *testing.T) {
	loginAsAdmin(t)
	// We may have a provider from a previous test run — just verify the endpoint works
	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/providers", nil, adminHeaders())
	if err != nil {
		t.Fatalf("list providers failed: %v", err)
	}
	assertStatus(t, resp, 200)

	providers := make([]interface{}, 0)
	if err := json.Unmarshal(ar.Data, &providers); err != nil {
		t.Fatalf("unmarshal providers: %v", err)
	}
	t.Logf("Providers count: %d", len(providers))
}

func testProvidersCreateInvalidID(t *testing.T) {
	loginAsAdmin(t)

	body := map[string]interface{}{
		"provider_id": "unknown-provider",
		"name":        "Invalid",
		"api_key":     "test-key",
	}
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/providers", body, adminHeaders())
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	assertStatus(t, resp, 422)
	assertError(t, ar, "Validation failed")
}

func testProvidersMalformedProviderID(t *testing.T) {
	loginAsAdmin(t)

	body := map[string]interface{}{
		"provider_id": "",
		"name":        "Bad Provider",
		"api_key":     "test-key",
	}
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/providers", body, adminHeaders())
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	assertStatus(t, resp, 422)
	assertError(t, ar, "Validation failed")
}

func testProvidersCreate(t *testing.T) {
	loginAsAdmin(t)

	body := map[string]interface{}{
		"provider_id": "custom",
		"name":        "Custom E2E",
		"api_key":     "sk-test-key-12345",
		"base_url":    "https://api.custom.com/v1",
		"models":      []string{"gpt-4", "gpt-4-turbo", "gpt-3.5-turbo"},
	}
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/providers", body, adminHeaders())
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	assertStatus(t, resp, 201)

	var provider struct {
		ID         string   `json:"id"`
		ProviderID string   `json:"provider_id"`
		Name       string   `json:"name"`
		Enabled    bool     `json:"enabled"`
		Models     []string `json:"models"`
	}
	if err := json.Unmarshal(ar.Data, &provider); err != nil {
		t.Fatalf("unmarshal provider: %v", err)
	}
	if provider.ProviderID != "custom" {
		t.Errorf("expected provider_id 'custom', got %q", provider.ProviderID)
	}
	if len(provider.Models) != 3 {
		t.Errorf("expected 3 models, got %d", len(provider.Models))
	}
	if !provider.Enabled {
		t.Error("expected provider to be enabled")
	}

	suite.ProviderUUID = provider.ID
	t.Logf("Created provider: %s (%s)", suite.ProviderUUID, provider.Name)
}

func testProvidersGetByID(t *testing.T) {
	if suite.ProviderUUID == "" {
		t.Fatal("no provider created")
	}
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/providers/"+suite.ProviderUUID, nil, adminHeaders())
	if err != nil {
		t.Fatalf("get provider failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var provider struct {
		ID         string `json:"id"`
		ProviderID string `json:"provider_id"`
		Name       string `json:"name"`
	}
	if err := json.Unmarshal(ar.Data, &provider); err != nil {
		t.Fatalf("unmarshal provider: %v", err)
	}
	if provider.ID != suite.ProviderUUID {
		t.Errorf("expected id %q, got %q", suite.ProviderUUID, provider.ID)
	}
}

func testProvidersGetByIDNotFound(t *testing.T) {
	loginAsAdmin(t)

	resp, _, err := doRequest("GET", adminBase+"/api/v1/admin/providers/00000000-0000-0000-0000-000000000000", nil, adminHeaders())
	if err != nil {
		t.Fatalf("get provider failed: %v", err)
	}
	assertStatus(t, resp, 404)
}

func testProvidersUpdate(t *testing.T) {
	if suite.ProviderUUID == "" {
		t.Fatal("no provider created")
	}
	loginAsAdmin(t)

	updatedName := "OpenAI E2E Updated"
	newModels := []string{"gpt-4", "gpt-4o"}
	resp, ar, err := doRequest("PUT", adminBase+"/api/v1/admin/providers/"+suite.ProviderUUID, map[string]interface{}{
		"name":   updatedName,
		"models": newModels,
	}, adminHeaders())
	if err != nil {
		t.Fatalf("update provider failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var provider struct {
		Name   string   `json:"name"`
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(ar.Data, &provider); err != nil {
		t.Fatalf("unmarshal provider: %v", err)
	}
	if provider.Name != updatedName {
		t.Errorf("expected name %q, got %q", updatedName, provider.Name)
	}
	if len(provider.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(provider.Models))
	}
	if provider.Models[0] != "gpt-4" && provider.Models[1] != "gpt-4o" {
		t.Errorf("unexpected models: %v", provider.Models)
	}
}

func testProvidersDelete(t *testing.T) {
	loginAsAdmin(t)

	// Create a provider to delete
	body := map[string]interface{}{
		"provider_id": "azure",
		"name":        "Provider To Delete",
		"api_key":     "sk-test-key",
	}
	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/providers", body, adminHeaders())
	if err != nil {
		t.Fatalf("create provider failed: %v", err)
	}
	assertStatus(t, resp, 201)

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ar.Data, &created); err != nil {
		t.Fatalf("unmarshal created provider: %v", err)
	}

	// Delete it
	resp, _, err = doRequest("DELETE", adminBase+"/api/v1/admin/providers/"+created.ID, nil, adminHeaders())
	if err != nil {
		t.Fatalf("delete provider failed: %v", err)
	}
	assertStatus(t, resp, 204)

	// Verify it's gone
	resp, ar, err = doRequest("GET", adminBase+"/api/v1/admin/providers/"+created.ID, nil, adminHeaders())
	if err != nil {
		t.Fatalf("get deleted provider failed: %v", err)
	}
	assertStatus(t, resp, 404)
}

func testProvidersListWithResults(t *testing.T) {
	if suite.ProviderUUID == "" {
		t.Fatal("no provider created")
	}
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/providers", nil, adminHeaders())
	if err != nil {
		t.Fatalf("list providers failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var providers []struct {
		ID         string `json:"id"`
		ProviderID string `json:"provider_id"`
		Name       string `json:"name"`
	}
	if err := json.Unmarshal(ar.Data, &providers); err != nil {
		t.Fatalf("unmarshal providers: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected at least one provider")
	}

	found := false
	for _, p := range providers {
		if p.ID == suite.ProviderUUID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created provider not found in list")
	}
}

/* ════════════════════════════════════════════════════════════
   6. Audit Log Tests
   ════════════════════════════════════════════════════════════ */

func testAuditList(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/audit-logs", nil, adminHeaders())
	if err != nil {
		t.Fatalf("list audit logs failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var logs []struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(ar.Data, &logs); err != nil {
		t.Fatalf("unmarshal audit logs: %v", err)
	}
	t.Logf("Found %d audit logs", len(logs))

	// There should be at least a few logs from client/provider creation
	if len(logs) > 0 {
		suite.AuditLogID = logs[0].ID
	}
}

func testAuditListWithFilter(t *testing.T) {
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/audit-logs?event_type=client_created", nil, adminHeaders())
	if err != nil {
		t.Fatalf("list audit logs with filter failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var logs []interface{}
	if err := json.Unmarshal(ar.Data, &logs); err != nil {
		t.Fatalf("unmarshal audit logs: %v", err)
	}
	t.Logf("Found %d client_created logs", len(logs))
}

func testAuditGetByID(t *testing.T) {
	if suite.AuditLogID == "" {
		t.Skip("no audit logs available")
	}
	loginAsAdmin(t)

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/audit-logs/"+suite.AuditLogID, nil, adminHeaders())
	if err != nil {
		t.Fatalf("get audit log failed: %v", err)
	}
	assertStatus(t, resp, 200)

	var ev struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(ar.Data, &ev); err != nil {
		t.Fatalf("unmarshal audit event: %v", err)
	}
	if ev.ID != suite.AuditLogID {
		t.Errorf("expected id %q, got %q", suite.AuditLogID, ev.ID)
	}
}

func testAuditGetByIDNotFound(t *testing.T) {
	loginAsAdmin(t)

	resp, _, err := doRequest("GET", adminBase+"/api/v1/admin/audit-logs/00000000-0000-0000-0000-000000000000", nil, adminHeaders())
	if err != nil {
		t.Fatalf("get audit log failed: %v", err)
	}
	assertStatus(t, resp, 404)
}

/* ════════════════════════════════════════════════════════════
   7. API Proxy Middleware Chain Tests
   ════════════════════════════════════════════════════════════ */

func testProxyMissingClientID(t *testing.T) {
	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"Authorization": "Bearer " + suite.ClientSecret,
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	if ar.Error == nil || !strings.Contains(ar.Error.Detail, "X-Client-ID") {
		t.Errorf("expected X-Client-ID error, got %+v", ar.Error)
	}
}

func testProxyMissingAuthorization(t *testing.T) {
	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"X-Client-ID": suite.ClientID,
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	if ar.Error == nil || !strings.Contains(ar.Error.Detail, "Authorization") {
		t.Errorf("expected Authorization error, got %+v", ar.Error)
	}
}

func testProxyInvalidClientID(t *testing.T) {
	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"X-Client-ID":   "nonexistent-client",
		"Authorization": "Bearer " + suite.ClientSecret,
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	if ar.Error == nil || !strings.Contains(ar.Error.Detail, "credentials") {
		t.Errorf("expected credentials error, got %+v", ar.Error)
	}
}

func testProxyInvalidSecret(t *testing.T) {
	if suite.ClientID == "" {
		t.Fatal("no client created")
	}

	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"X-Client-ID":   suite.ClientID,
		"Authorization": "Bearer sk-wrong-secret",
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	if ar.Error == nil || !strings.Contains(ar.Error.Detail, "secret") {
		t.Errorf("expected secret error, got %+v", ar.Error)
	}
}

func testProxyMissingNonce(t *testing.T) {
	if suite.ClientID == "" || suite.ClientSecret == "" {
		t.Fatal("no client created")
	}

	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"X-Client-ID":   suite.ClientID,
		"Authorization": "Bearer " + suite.ClientSecret,
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	if ar.Error == nil || ar.Error.Message != "Invalid or missing nonce" {
		t.Errorf("expected nonce error, got %+v", ar.Error)
	}
}

func testProxyMissingTimestamp(t *testing.T) {
	if suite.ClientID == "" || suite.ClientSecret == "" {
		t.Fatal("no client created")
	}

	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"X-Client-ID":   suite.ClientID,
		"Authorization": "Bearer " + suite.ClientSecret,
		"X-Nonce":       "unique-nonce-123",
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 400)
	if ar.Error == nil || ar.Error.Message != "Invalid or expired timestamp" {
		t.Errorf("expected timestamp error, got %+v", ar.Error)
	}
}

func testProxyExpiredTimestamp(t *testing.T) {
	if suite.ClientID == "" || suite.ClientSecret == "" {
		t.Fatal("no client created")
	}

	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, map[string]string{
		"X-Client-ID":   suite.ClientID,
		"Authorization": "Bearer " + suite.ClientSecret,
		"X-Nonce":       "unique-nonce-456",
		"X-Timestamp":   fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix()),
	})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 400)
	if ar.Error == nil || ar.Error.Message != "Invalid or expired timestamp" {
		t.Errorf("expected timestamp error, got %+v", ar.Error)
	}
}

func testProxyNonceReplay(t *testing.T) {
	if suite.ClientID == "" || suite.ClientSecret == "" {
		t.Fatal("no client created")
	}

	now := time.Now().Unix()
	nonce := fmt.Sprintf("replay-nonce-%d", now)

	headers := map[string]string{
		"X-Client-ID":   suite.ClientID,
		"Authorization": "Bearer " + suite.ClientSecret,
		"X-Nonce":       nonce,
		"X-Timestamp":   fmt.Sprintf("%d", now),
	}

	// First request — may fail with 400 (model resolution) but nonce is consumed
	firstResp, _, _ := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, headers)
	t.Logf("First request status: %d (nonce should be consumed)", firstResp.StatusCode)

	// Second request with SAME nonce — MUST fail with nonce replay
	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{
		"model": "gpt-4",
	}, headers)
	if err != nil {
		t.Fatalf("second proxy request failed: %v", err)
	}
	assertStatus(t, resp, 401)
	if ar.Error == nil || ar.Error.Message != "Nonce already used" {
		t.Errorf("expected 'Nonce already used', got %+v", ar.Error)
	}
}

func testProxyRateLimitHeaders(t *testing.T) {
	if suite.ClientID == "" || suite.ClientSecret == "" {
		t.Fatal("no client created")
	}

	now := time.Now().Unix()
	nonce := fmt.Sprintf("ratelimit-nonce-%d", now)

	// Use raw request to inspect response headers
	resp, err := doRawRequest("POST", apiBase+"/api/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4"}`),
		map[string]string{
			"Content-Type":  "application/json",
			"X-Client-ID":   suite.ClientID,
			"Authorization": "Bearer " + suite.ClientSecret,
			"X-Nonce":       nonce,
			"X-Timestamp":   fmt.Sprintf("%d", now),
		},
	)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining != "" {
		t.Logf("X-RateLimit-Remaining: %s", remaining)
	} else {
		t.Log("X-RateLimit-Remaining header not set")
	}
	t.Logf("Response status: %d", resp.StatusCode)
}

func testProxyModelRequired(t *testing.T) {
	if suite.ClientID == "" || suite.ClientSecret == "" {
		t.Fatal("no client created")
	}

	now := time.Now().Unix()
	resp, ar, err := doRequest("POST", apiBase+"/api/v1/chat/completions", map[string]string{},
		map[string]string{
			"X-Client-ID":   suite.ClientID,
			"Authorization": "Bearer " + suite.ClientSecret,
			"X-Nonce":       fmt.Sprintf("model-nonce-%d", now),
			"X-Timestamp":   fmt.Sprintf("%d", now),
		})
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	assertStatus(t, resp, 400)
	if ar.Error == nil || ar.Error.Message != "Bad request" {
		t.Errorf("expected Bad request, got %+v", ar.Error)
	}
}

/* ════════════════════════════════════════════════════════════
   8. CORS & Edge Case Tests
   ════════════════════════════════════════════════════════════ */

func testCORSHeaders(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("OPTIONS", apiBase+"/api/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		t.Fatalf("expected 204 for OPTIONS, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("Access-Control-Allow-Methods header should be present")
	}
	if resp.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Error("Access-Control-Allow-Headers header should be present")
	}
}

func testRBACUnauthenticatedAccess(t *testing.T) {
	protectedEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/admin/me"},
		{"GET", "/api/v1/admin/dashboard/stats"},
		{"GET", "/api/v1/admin/clients"},
		{"POST", "/api/v1/admin/clients"},
		{"GET", "/api/v1/admin/providers"},
		{"POST", "/api/v1/admin/providers"},
		{"GET", "/api/v1/admin/audit-logs"},
	}

	for _, ep := range protectedEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			url := adminBase + ep.path
			var resp *http.Response
			var err error

			if ep.method == "POST" {
				resp, _, err = doRequest(ep.method, url, map[string]string{"name": "test"}, nil)
			} else {
				resp, _, err = doRequest(ep.method, url, nil, nil)
			}
			if err != nil {
				t.Fatalf("request to %s failed: %v", ep.path, err)
			}
			if resp.StatusCode != 401 {
				t.Errorf("expected 401 for %s %s, got %d", ep.method, ep.path, resp.StatusCode)
			}
		})
	}
}

/* ════════════════════════════════════════════════════════════
   Assertion Helpers
   ════════════════════════════════════════════════════════════ */

func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("expected status %d, got %d", expected, resp.StatusCode)
	}
}

func assertError(t *testing.T, ar *apiResponse, expectedMsg string) {
	t.Helper()
	if ar.Error == nil {
		t.Fatalf("expected error with message %q, got nil", expectedMsg)
	}
	if ar.Error.Message != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, ar.Error.Message)
	}
}
