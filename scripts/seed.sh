#!/usr/bin/env bash
# =============================================================================
# seed.sh — Seed the database with initial admin user and sample data
#
# Usage:
#   ./scripts/seed.sh                          # Interactive (prompts for email/password)
#   EMAIL=admin@example.com PASSWORD=secret ./scripts/seed.sh  # Non-interactive
#   ./scripts/seed.sh --help                   # Show usage
# =============================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/ai_proxy?sslmode=disable}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# ─── Parse input ───────────────────────────────────────────
if [[ "${1:-}" == "--help" ]]; then
    echo "Usage:"
    echo "  ./scripts/seed.sh                          # Interactive"
    echo "  EMAIL=admin@example.com PASSWORD=secret ./scripts/seed.sh"
    echo ""
    echo "Environment variables:"
    echo "  DATABASE_URL   PostgreSQL connection string"
    echo "  EMAIL          Admin email (non-interactive mode)"
    echo "  PASSWORD       Admin password (non-interactive mode)"
    echo "  NAME           Admin display name (default: 'Super Admin')"
    echo "  ROLE           Admin role (default: 'super_admin')"
    exit 0
fi

# ─── Collect credentials ──────────────────────────────────
if [[ -n "${EMAIL:-}" && -n "${PASSWORD:-}" ]]; then
    EMAIL="${EMAIL}"
    PASSWORD="${PASSWORD}"
    NAME="${NAME:-Super Admin}"
    ROLE="${ROLE:-super_admin}"
else
    echo ""
    echo "Create your initial admin user"
    echo "──────────────────────────────"
    read -rp "Email:    " EMAIL
    read -rsp "Password: " PASSWORD
    echo ""
    read -rp "Name:     " NAME
    NAME="${NAME:-Super Admin}"
    ROLE="super_admin"
fi

if [[ -z "${EMAIL}" || -z "${PASSWORD}" ]]; then
    log_error "Email and password are required"
    exit 1
fi

# ─── Generate password hash ───────────────────────────────
log_info "Generating password hash..."
PASSWORD_HASH=$(printf '%s' "${PASSWORD}" | sha256sum | cut -d' ' -f1)

# ─── Check if admin exists ────────────────────────────────
ESCAPED_EMAIL=$(printf '%s' "${EMAIL}" | sed "s/'/''/g")
EXISTING=$(psql "${DATABASE_URL}" -t -A -c \
    "SELECT id FROM admin_users WHERE email = '${ESCAPED_EMAIL}'" 2>/dev/null || true)

if [[ -n "${EXISTING}" ]]; then
    log_warn "Admin with email '${EMAIL}' already exists (id: ${EXISTING})"
    exit 0
fi

# ─── Insert admin user ────────────────────────────────────
ESCAPED_NAME=$(printf '%s' "${NAME}" | sed "s/'/''/g")
psql "${DATABASE_URL}" -q -c "
    INSERT INTO admin_users (email, password_hash, name, role)
    VALUES ('${ESCAPED_EMAIL}', '${PASSWORD_HASH}', '${ESCAPED_NAME}', '${ROLE}');
"

log_info "Admin user created successfully:"
echo "  Email: ${EMAIL}"
echo "  Name:  ${NAME}"
echo "  Role:  ${ROLE}"
echo ""
log_warn "Remember to change your password after first login!"
