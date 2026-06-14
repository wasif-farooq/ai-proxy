//go:build e2e

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/* ─── Constants ─────────────────────────────────────────── */

const (
	apiBase   = "http://localhost:8080"
	adminBase = "http://localhost:8081"

	adminEmail    = "admin@test.local"
	adminPassword = "test-password-1234!"
	adminName     = "Test Admin"
	adminRole     = "super_admin"

	jwtSecret = "test-jwt-secret-for-e2e"
)

/* ─── Test Context ──────────────────────────────────────── */

// Suite holds shared state populated incrementally by ordered subtests.
type Suite struct {
	AdminToken   string
	AdminID      string
	ClientID     string
	ClientSecret string
	ProviderUUID string
	AuditLogID   string
}

var suite Suite

// TestMain orchestrates the e2e test lifecycle.
func TestMain(m *testing.M) {
	log.Println("=== E2E Test Suite Setup ===")

	if err := dockerComposeUp(); err != nil {
		log.Fatalf("Failed to start docker compose: %v", err)
	}

	if err := waitForHealth(5 * time.Minute); err != nil {
		dockerComposeDown()
		log.Fatalf("Services not ready: %v", err)
	}

	if err := seedAdminUser(); err != nil {
		dockerComposeDown()
		log.Fatalf("Failed to seed admin user: %v", err)
	}

	log.Println("Setup complete. Running tests...")

	exitCode := m.Run()

	log.Println("=== E2E Test Suite Teardown ===")
	dockerComposeDown()

	os.Exit(exitCode)
}

/* ─── Docker Compose Lifecycle ──────────────────────────── */

func dockerComposeUp() error {
	log.Println("Starting docker compose...")
	cmd := exec.Command("docker", "compose",
		"-f", filepath.Join(projectRoot(), "deployments/docker/docker-compose.dev.yml"),
		"up", "-d", "--build",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w\nOutput: %s", err, output)
	}
	log.Printf("Docker compose started:\n%s", output)
	return nil
}

func dockerComposeDown() {
	log.Println("Stopping docker compose...")
	cmd := exec.Command("docker", "compose",
		"-f", filepath.Join(projectRoot(), "deployments/docker/docker-compose.dev.yml"),
		"down", "--volumes",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Warning: docker compose down failed: %v\nOutput: %s", err, output)
		return
	}
	log.Printf("Docker compose stopped:\n%s", output)
}

// projectRoot walks up from the working directory to find go.mod.
func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("cannot get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatalf("could not find go.mod from %s", dir)
		}
		dir = parent
	}
}

/* ─── Health Check Wait ─────────────────────────────────── */

func waitForHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	interval := 2 * time.Second

	log.Println("Waiting for services to become healthy...")

	for time.Now().Before(deadline) {
		apiOK := ping(apiBase + "/health")
		adminOK := ping(adminBase + "/health")
		dbOK := pgReady()

		if apiOK && adminOK && dbOK {
			log.Println("All services are healthy.")
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("timeout waiting for services after %v", timeout)
}

func ping(url string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func pgReady() bool {
	cmd := exec.Command("docker", "exec", "docker-postgres-1",
		"pg_isready", "-U", "postgres",
	)
	return cmd.Run() == nil
}

/* ─── Seed Admin User ───────────────────────────────────── */

func seedAdminUser() error {
	h := sha256.Sum256([]byte(adminPassword))
	passwordHash := fmt.Sprintf("%x", h[:])

	escapedEmail := strings.ReplaceAll(adminEmail, "'", "''")
	escapedName := strings.ReplaceAll(adminName, "'", "''")

	sql := fmt.Sprintf(
		`INSERT INTO admin_users (email, password_hash, name, role) VALUES ('%s', '%s', '%s', '%s') ON CONFLICT (email) DO NOTHING;`,
		escapedEmail, passwordHash, escapedName, adminRole,
	)

	cmd := exec.Command("docker", "exec", "-i", "docker-postgres-1",
		"psql", "-U", "postgres", "-d", "ai_proxy", "-c", sql,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("seed admin failed: %w\nOutput: %s", err, output)
	}
	log.Printf("Admin user seeded: %s", adminEmail)
	return nil
}

/* ─── HTTP Helpers ──────────────────────────────────────── */

type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Detail  string `json:"detail,omitempty"`
	} `json:"error,omitempty"`
	Meta *struct {
		Total int `json:"total,omitempty"`
		Page  int `json:"page,omitempty"`
		Limit int `json:"limit,omitempty"`
	} `json:"meta,omitempty"`
}

func doRequest(method, url string, body interface{}, headers map[string]string) (*http.Response, *apiResponse, error) {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal body: %w", err)
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	var ar apiResponse
	if resp.Body != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
			// io.EOF is expected for empty bodies (204 No Content)
			return resp, &ar, nil
		}
	}

	return resp, &ar, nil
}

func doRawRequest(method, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func adminHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + suite.AdminToken,
	}
}

// loginAsAdmin authenticates as the seeded admin and stores the token.
func loginAsAdmin(t *testing.T) {
	t.Helper()

	resp, ar, err := doRequest("POST", adminBase+"/api/v1/admin/auth/login", map[string]string{
		"email":    adminEmail,
		"password": adminPassword,
	}, nil)
	if err != nil {
		t.Fatalf("admin login request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("admin login expected 200, got %d. Response: %+v", resp.StatusCode, ar)
	}

	var loginResp struct {
		Token   string `json:"token"`
		AdminID string `json:"admin_id"`
	}
	if err := json.Unmarshal(ar.Data, &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	suite.AdminToken = loginResp.Token
	suite.AdminID = loginResp.AdminID

	if suite.AdminToken == "" {
		t.Fatal("admin login did not return a token")
	}
	t.Logf("Admin logged in: %s (token: %.20s...)", suite.AdminID, suite.AdminToken)
}

// findClientUUID gets the internal UUID of a client by its client_id.
func findClientUUID(t *testing.T, clientID string) string {
	t.Helper()

	if suite.AdminToken == "" {
		loginAsAdmin(t)
	}

	resp, ar, err := doRequest("GET", adminBase+"/api/v1/admin/clients", nil, adminHeaders())
	if err != nil {
		t.Fatalf("list clients failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("list clients expected 200, got %d", resp.StatusCode)
	}

	var clients []struct {
		ID       string `json:"id"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(ar.Data, &clients); err != nil {
		t.Fatalf("unmarshal clients: %v", err)
	}
	for _, c := range clients {
		if c.ClientID == clientID {
			return c.ID
		}
	}
	return ""
}
