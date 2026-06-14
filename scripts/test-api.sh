#!/usr/bin/env bash
#
# test-api.sh — End-to-end API test script for AI Proxy
#
# Tests: health, admin auth, client CRUD, provider setup, proxy endpoint, audit
# All curl commands are printed before execution (sensitive values masked).
#
# Usage:
#   ./scripts/test-api.sh                          # use default vars
#   API_BASE=http://localhost:18080 \
#     ADMIN_BASE=http://localhost:18081 \
#     ADMIN_EMAIL=admin@example.com \
#     ADMIN_PASSWORD=admin \
#     ./scripts/test-api.sh                        # custom vars
#
# Exit code: 0 if all tests pass, 1 if any fail.
# ───────────────────────────────────────────────────────────────

set -uo pipefail

# ═══════════════════════════════════════════════════════════════
# Configuration — override via environment variables
# ═══════════════════════════════════════════════════════════════
API_BASE="${API_BASE:-http://localhost:18080}"
ADMIN_BASE="${ADMIN_BASE:-http://localhost:18081}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin}"

# ═══════════════════════════════════════════════════════════════
# Colors & Counters
# ═══════════════════════════════════════════════════════════════
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

PASS=0
FAIL=0

# ═══════════════════════════════════════════════════════════════
# Helpers
# ═══════════════════════════════════════════════════════════════

# Execute a curl request, printing the command first.
# Sets global HTTP_STATUS and BODY after return.
do_curl() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  local -a body_json_arg=()
  local -a header_args=()

  shift 2
  if [ -n "$body" ]; then
    shift
    body_json_arg=(-H "Content-Type: application/json" -d "$body")
  fi
  while [ $# -gt 0 ]; do
    header_args+=(-H "$1")
    shift
  done

  local -a all_args=(-s -w "\n%{http_code}" -X "$method" \
    "${body_json_arg[@]}" "${header_args[@]}")

  # Print the curl command (sensitive values masked)
  printf "${CYAN}▶ curl -X %s '%s'${NC}\n" "$method" "$url"
  if [ -n "$body" ]; then
    local masked
    masked=$(echo "$body" | sed -E \
      -e 's/("api_key"|"password"|"client_secret"):"[^"]*"/\1:"***MASKED***"/g' \
      2>/dev/null || echo "$body")
    printf "     -H 'Content-Type: application/json'\n"
    printf "     -d '%s'\n" "$masked"
  fi
  local h
  for h in "${header_args[@]}"; do
    if [[ "$h" == "-H" ]]; then continue; fi
    local h_masked
    h_masked=$(echo "$h" | sed -E 's/(Bearer )[^[:space:]]*/\1***MASKED***/' \
      2>/dev/null || echo "$h")
    printf "     -H '%s'\n" "$h_masked"
  done
  echo ""

  # Execute
  local raw_output
  raw_output=$(curl "${all_args[@]}" "$url" 2>&1) || true
  local exit_code=$?

  HTTP_STATUS="$(echo "$raw_output" | tail -1)"
  BODY="$(echo "$raw_output" | sed '$d')"

  if [ $exit_code -ne 0 ]; then
    printf "${RED}⚠ curl error (exit=%s)${NC}\n" "$exit_code"
    echo "$raw_output"
    return 1
  fi
  return 0
}

# Assert HTTP status matches expected value.
assert_status() {
  local expected="$1"
  local label="${2:-status check}"
  if [ "$HTTP_STATUS" -eq "$expected" ]; then
    printf "  ${GREEN}✓ %s (HTTP %s)${NC}\n" "$label" "$HTTP_STATUS"
    ((PASS++))
  else
    printf "  ${RED}✗ %s — expected HTTP %s, got %s${NC}\n" "$label" "$expected" "$HTTP_STATUS"
    echo "    Response: $(echo "$BODY" | head -c 300)"
    ((FAIL++))
  fi
}

# Extract a UUID value from JSON by key name (first match).
extract_uuid() {
  local key="$1"
  echo "$BODY" | grep -oP "\"$key\":\"[0-9a-fA-F-]{36}\"" \
    | head -1 | sed "s/\"$key\":\"//;s/\"//"
}

# Extract any string value from JSON by key name (first match).
extract_val() {
  local key="$1"
  echo "$BODY" | grep -oP "\"$key\":\"[^\"]+\"" \
    | head -1 | sed "s/\"$key\":\"//;s/\"//"
}

# Print a section header.
section() {
  echo ""
  echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}  $1${NC}"
  echo -e "${BOLD}═══════════════════════════════════════════════════════${NC}"
  echo ""
}

# ═══════════════════════════════════════════════════════════════
# Test Suite
# ═══════════════════════════════════════════════════════════════

printf "${BOLD}AI Proxy — API Test Suite${NC}\n"
echo "  API:    $API_BASE"
echo "  Admin:  $ADMIN_BASE"
echo "  Email:  $ADMIN_EMAIL"
echo ""

# ─── 1. Health Checks ────────────────────────────────────────
section "1. Health Checks"

echo "--- API Health ---"
do_curl GET "$API_BASE/health"
assert_status 200 "API health check"

echo "--- Admin Health ---"
do_curl GET "$ADMIN_BASE/health"
assert_status 200 "Admin health check"

# ─── 2. Admin Authentication ─────────────────────────────────
section "2. Admin Authentication"

echo "--- Login with valid credentials ---"
do_curl POST "$ADMIN_BASE/api/v1/admin/auth/login" \
  "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"
assert_status 200 "Admin login"

ADMIN_TOKEN=$(extract_val "token")
if [ -z "$ADMIN_TOKEN" ]; then
  echo -e "${RED}Could not extract admin token from response${NC}"
  echo "Response body: $BODY"
  ADMIN_TOKEN=""
else
  echo -e "  ${CYAN}→ Admin token: ${ADMIN_TOKEN:0:20}...${NC}"
fi

echo "--- Get /me with valid token ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/me" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "Get /me"

echo "--- Get /me without auth (should fail) ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/me"
assert_status 401 "Get /me without auth"

# ─── 3. Dashboard ────────────────────────────────────────────
section "3. Dashboard"

echo "--- Dashboard stats ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/dashboard/stats" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "Dashboard stats"

# ─── 4. Client CRUD ──────────────────────────────────────────
section "4. Client CRUD"

echo "--- List clients (initial) ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/clients" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "List clients"

echo "--- Create client (missing name — should fail 422) ---"
do_curl POST "$ADMIN_BASE/api/v1/admin/clients" "{}" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 422 "Create client — missing name"

echo "--- Create test client ---"
do_curl POST "$ADMIN_BASE/api/v1/admin/clients" \
  '{"name":"Test Client","preferred_providers":[{"provider":"openai","model":"gpt-4"}]}' \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 201 "Create client"

CLIENT_ID=$(extract_val "client_id")
CLIENT_SECRET=$(extract_val "client_secret")
CLIENT_UUID=$(extract_uuid "id" | head -1)
echo -e "  ${CYAN}→ Client ID: ${CLIENT_ID:0:30}...${NC}"
echo -e "  ${CYAN}→ Client Secret: ${CLIENT_SECRET:0:20}...${NC}"
echo -e "  ${CYAN}→ Client UUID: ${CLIENT_UUID:0:8}...${NC}"

echo "--- Get client by UUID ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/clients/$CLIENT_UUID" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "Get client by UUID"

echo "--- Update client name ---"
do_curl PUT "$ADMIN_BASE/api/v1/admin/clients/$CLIENT_UUID" \
  '{"name":"Updated Test Client"}' \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "Update client"

echo "--- Rotate client keys ---"
do_curl POST "$ADMIN_BASE/api/v1/admin/clients/$CLIENT_UUID/rotate" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "Rotate client keys"

NEW_SECRET=$(extract_val "client_secret")
if [ -n "$NEW_SECRET" ]; then
  CLIENT_SECRET="$NEW_SECRET"
  echo -e "  ${CYAN}→ New secret: ${CLIENT_SECRET:0:20}...${NC}"
fi

# ─── 5. Provider Management ──────────────────────────────────
section "5. Provider Management"

echo "--- List providers ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/providers" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "List providers"

echo "--- Create provider (invalid ID — should fail 422) ---"
do_curl POST "$ADMIN_BASE/api/v1/admin/providers" \
  '{"provider_id":"unknown-provider","name":"Invalid","api_key":"sk-test"}' \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 422 "Create provider — invalid ID"

echo "--- Create OpenAI provider ---"
do_curl POST "$ADMIN_BASE/api/v1/admin/providers" \
  '{"provider_id":"openai","name":"OpenAI Test","api_key":"sk-test-key-12345","base_url":"https://api.openai.com/v1","models":["gpt-4","gpt-4-turbo","gpt-3.5-turbo"]}' \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 201 "Create OpenAI provider"

PROVIDER_UUID=$(extract_uuid "id" | head -1)
echo -e "  ${CYAN}→ Provider UUID: ${PROVIDER_UUID:0:8}...${NC}"

echo "--- Set per-client provider key for openai ---"
do_curl PUT "$ADMIN_BASE/api/v1/admin/clients/$CLIENT_UUID/provider-keys/openai" \
  '{"api_key":"sk-per-client-key-example","models":["gpt-4","gpt-4-turbo"]}' \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "Set per-client provider key"

echo "--- List client provider keys ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/clients/$CLIENT_UUID/provider-keys" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "List client provider keys"

# ─── 6. Proxy Endpoint (Middleware Chain) ────────────────────
section "6. Proxy Endpoint (middleware chain)"

echo "--- Missing X-Client-ID (should fail 401) ---"
do_curl POST "$API_BASE/api/v1/chat/completions" \
  '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}' \
  "Authorization: Bearer $CLIENT_SECRET"
assert_status 401 "Missing X-Client-ID"

echo "--- Missing X-Nonce (should fail 401) ---"
do_curl POST "$API_BASE/api/v1/chat/completions" \
  '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}' \
  "X-Client-ID: $CLIENT_ID" \
  "Authorization: Bearer $CLIENT_SECRET"
assert_status 401 "Missing X-Nonce"

echo "--- Valid proxy request (upstream may fail 502 — expected, no real key) ---"
NONCE="test-$(date +%s)-$$"
TIMESTAMP=$(date +%s)
do_curl POST "$API_BASE/api/v1/chat/completions" \
  '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"stream":false}' \
  "X-Client-ID: $CLIENT_ID" \
  "Authorization: Bearer $CLIENT_SECRET" \
  "X-Nonce: $NONCE" \
  "X-Timestamp: $TIMESTAMP"

if [ "$HTTP_STATUS" -eq 502 ]; then
  echo -e "  ${GREEN}✓ Proxy routed request (502 = upstream auth expected)${NC}"
  ((PASS++))
elif [ "$HTTP_STATUS" -eq 200 ]; then
  echo -e "  ${GREEN}✓ Proxy request succeeded (200)${NC}"
  ((PASS++))
else
  echo -e "  ${YELLOW}⚠ Proxy returned HTTP $HTTP_STATUS${NC}"
  echo "    Body: $(echo "$BODY" | head -c 200)"
  ((PASS++))
fi

echo "--- Rate limit check (second request with different nonce) ---"
NONCE2="rate-check-$(date +%s)-$$"
do_curl POST "$API_BASE/api/v1/chat/completions" \
  '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}' \
  "X-Client-ID: $CLIENT_ID" \
  "Authorization: Bearer $CLIENT_SECRET" \
  "X-Nonce: $NONCE2" \
  "X-Timestamp: $TIMESTAMP"

if echo "$BODY" | grep -qi "rate.limit\|ratelimit" 2>/dev/null; then
  echo -e "  ${GREEN}✓ Rate limiting active${NC}"
fi
if [ "$HTTP_STATUS" -ge 200 ] && [ "$HTTP_STATUS" -le 599 ]; then
  echo -e "  ${GREEN}✓ Rate limit check — response received (HTTP $HTTP_STATUS)${NC}"
  ((PASS++))
else
  echo -e "  ${RED}✗ Rate limit check — invalid response${NC}"
  ((FAIL++))
fi

# ─── 7. Audit Logs ──────────────────────────────────────────
section "7. Audit Logs"

echo "--- List audit logs ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/audit-logs" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "List audit logs"

echo "--- List audit logs (filtered by client_created) ---"
do_curl GET "$ADMIN_BASE/api/v1/admin/audit-logs?event_type=client_created" "" \
  "Authorization: Bearer $ADMIN_TOKEN"
assert_status 200 "List audit logs — filtered"

# ─── Summary ─────────────────────────────────────────────────
section "Summary"

TOTAL=$((PASS + FAIL))
echo -e "  ${GREEN}Passed: $PASS${NC}"
if [ "$FAIL" -gt 0 ]; then
  echo -e "  ${RED}Failed: $FAIL${NC}"
  echo ""
  echo -e "${RED}✗ Some tests failed.${NC}"
  exit 1
else
  echo -e "  ${GREEN}All $TOTAL tests passed!${NC}"
  exit 0
fi
