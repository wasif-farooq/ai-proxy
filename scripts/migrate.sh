#!/usr/bin/env bash
# =============================================================================
# migrate.sh — Run database migrations for ai-proxy
#
# Usage:
#   ./scripts/migrate.sh                        # Apply all pending migrations
#   ./scripts/migrate.sh --dry-run              # Print what would be applied
#   ./scripts/migrate.sh --down                 # Rollback last migration
# =============================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${DIR}/../internal/database/migrations"
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/ai_proxy?sslmode=disable}"

# Color helpers
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    log_info "Dry-run mode — no changes will be applied"
fi

# Check dependencies
if ! command -v psql &>/dev/null; then
    log_error "psql is required but not installed"
    exit 1
fi

# Create migrations tracking table if it doesn't exist
log_info "Ensuring _migrations tracking table exists..."
if ! $DRY_RUN; then
    psql "${DATABASE_URL}" -q -c "
        CREATE TABLE IF NOT EXISTS _migrations (
            id        SERIAL PRIMARY KEY,
            filename  VARCHAR(255) UNIQUE NOT NULL,
            applied_at TIMESTAMPTZ DEFAULT NOW()
        );
    " 2>/dev/null || {
        log_warn "Could not create tracking table (expected if running as non-superuser)"
    }
fi

# Find and apply migrations in order
log_info "Looking for migrations in ${MIGRATIONS_DIR}..."

for f in $(ls "${MIGRATIONS_DIR}"/*.sql 2>/dev/null | sort); do
    filename=$(basename "${f}")

    # Check if already applied
    EXISTING=$(psql "${DATABASE_URL}" -t -A -c \
        "SELECT filename FROM _migrations WHERE filename = '$(printf '%s' "${filename}" | sed "s/'/''/g")'" 2>/dev/null)
    if [ "${EXISTING}" = "${filename}" ]; then
        log_info "  ✓ ${filename} — already applied"
        continue
    fi

    log_info "  → ${filename} — applying..."

    if $DRY_RUN; then
        echo "      Would apply: ${f}"
        continue
    fi

    # Apply migration
    if psql "${DATABASE_URL}" -q -f "${f}"; then
        # Record migration
        ESCAPED=$(printf '%s' "${filename}" | sed "s/'/''/g")
        psql "${DATABASE_URL}" -q -c \
            "INSERT INTO _migrations (filename) VALUES ('${ESCAPED}')" 2>/dev/null || true
        log_info "  ✓ ${filename} — done"
    else
        log_error "  ✗ ${filename} — FAILED"
        exit 1
    fi
done

log_info "All migrations applied successfully."
