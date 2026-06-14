package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the data-access contract for audit events.
type Repository interface {
	// InsertBatch inserts multiple audit events in a single transaction.
	InsertBatch(ctx context.Context, events []*AuditEvent) error
	// List returns a paginated, optionally filtered list of events.
	List(ctx context.Context, filter AuditFilter) (*AuditList, error)
	// GetByID retrieves a single event by its UUID.
	GetByID(ctx context.Context, id string) (*AuditEvent, error)
}

// PostgresRepository implements Repository backed by a pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new audit repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const auditColumns = `id, event_type, severity, client_id, admin_id, actor_type,
	request_id, ip_address, user_agent,
	action, resource, resource_id,
	provider_id, model, status_code, latency_ms,
	nonce_valid, token_type,
	before_state, after_state,
	timestamp`

// InsertBatch inserts events in a batch using a single multi-row INSERT.
// If any event fails, the entire batch is rolled back.
func (r *PostgresRepository) InsertBatch(ctx context.Context, events []*AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range events {
		batch.Queue(`
			INSERT INTO audit_logs (
				event_type, severity, client_id, admin_id, actor_type,
				request_id, ip_address, user_agent,
				action, resource, resource_id,
				provider_id, model, status_code, latency_ms,
				nonce_valid, token_type,
				before_state, after_state,
				timestamp
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8,
				$9, $10, $11,
				$12, $13, $14, $15,
				$16, $17,
				$18, $19,
				$20
			)`,
			string(e.EventType), string(e.Severity),
			e.ClientID, e.AdminID, string(e.ActorType),
			e.RequestID, e.IPAddress, e.UserAgent,
			e.Action, e.Resource, e.ResourceID,
			e.ProviderID, e.Model, e.StatusCode, e.LatencyMs,
			e.NonceValid, e.TokenType,
			e.BeforeState, e.AfterState,
			e.Timestamp,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := range events {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert audit event %d: %w", i, err)
		}
	}

	return nil
}

// scanEvent scans a single row into an AuditEvent.
func scanEvent(row pgx.Row) (*AuditEvent, error) {
	var e AuditEvent
	var beforeRaw, afterRaw []byte

	err := row.Scan(
		&e.ID, &e.EventType, &e.Severity,
		&e.ClientID, &e.AdminID, &e.ActorType,
		&e.RequestID, &e.IPAddress, &e.UserAgent,
		&e.Action, &e.Resource, &e.ResourceID,
		&e.ProviderID, &e.Model, &e.StatusCode, &e.LatencyMs,
		&e.NonceValid, &e.TokenType,
		&beforeRaw, &afterRaw,
		&e.Timestamp,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan audit event: %w", err)
	}

	if len(beforeRaw) > 0 {
		raw := json.RawMessage(beforeRaw)
		e.BeforeState = &raw
	}
	if len(afterRaw) > 0 {
		raw := json.RawMessage(afterRaw)
		e.AfterState = &raw
	}

	return &e, nil
}

// List returns paginated audit events with optional filtering.
func (r *PostgresRepository) List(ctx context.Context, filter AuditFilter) (*AuditList, error) {
	args := []any{}
	where := ""
	paramIdx := 0

	addParam := func(v any) string {
		paramIdx++
		args = append(args, v)
		return fmt.Sprintf("$%d", paramIdx)
	}

	clauses := []string{}
	if filter.EventType != nil {
		clauses = append(clauses, "event_type = "+addParam(string(*filter.EventType)))
	}
	if filter.Severity != nil {
		clauses = append(clauses, "severity = "+addParam(string(*filter.Severity)))
	}
	if filter.ClientID != nil {
		clauses = append(clauses, "client_id = "+addParam(*filter.ClientID))
	}
	if filter.AdminID != nil {
		clauses = append(clauses, "admin_id = "+addParam(*filter.AdminID))
	}

	if len(clauses) > 0 {
		where = " WHERE " + clauses[0]
		for _, c := range clauses[1:] {
			where += " AND " + c
		}
	}

	// Count
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs"+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count audit events: %w", err)
	}

	// Fetch
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	limitIdx := len(args) - 1

	rows, err := r.pool.Query(ctx,
		`SELECT `+auditColumns+` FROM audit_logs`+where+
			` ORDER BY timestamp DESC LIMIT $`+itoa(limitIdx)+
			` OFFSET $`+itoa(limitIdx+1),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	var events []AuditEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		if e != nil {
			events = append(events, *e)
		}
	}
	if events == nil {
		events = []AuditEvent{}
	}

	return &AuditList{Events: events, Total: total}, nil
}

// GetByID retrieves a single audit event by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*AuditEvent, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+auditColumns+` FROM audit_logs WHERE id = $1`, id,
	)
	return scanEvent(row)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
