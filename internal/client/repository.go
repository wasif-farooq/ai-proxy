package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the data-access contract for clients.
type Repository interface {
	Create(ctx context.Context, input CreateClientInput) (*Client, error)
	GetByID(ctx context.Context, id string) (*Client, error)
	GetByClientID(ctx context.Context, clientID string) (*Client, error)
	List(ctx context.Context, filter ClientFilter) (*ClientList, error)
	Update(ctx context.Context, id string, input UpdateClientInput) (*Client, error)
	UpdateStatus(ctx context.Context, id string, status ClientStatus) (*Client, error)
	RotateKeys(ctx context.Context, id string, secretHash, encKey, encSecret string) (*Client, error)
	Delete(ctx context.Context, id string) error
}

// PostgresRepository implements Repository backed by a pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new repository from a connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const (
	clientColumns = `id, client_id, client_secret_hash, name, status,
	                 encryption_key, encryption_secret, preferred_providers,
	                 created_at, updated_at, last_rotated_at`
)

// scanClient scans a row into a Client struct.
func scanClient(row pgx.Row) (*Client, error) {
	var c Client
	var providersJSON []byte
	var lastRotated *time.Time

	err := row.Scan(
		&c.ID, &c.ClientID, &c.ClientSecretHash, &c.Name, &c.Status,
		&c.EncryptionKey, &c.EncryptionSecret, &providersJSON,
		&c.CreatedAt, &c.UpdatedAt, &lastRotated,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan client: %w", err)
	}

	if len(providersJSON) > 0 {
		// Try new format first (array of objects), fall back to old format (array of strings)
		if err := json.Unmarshal(providersJSON, &c.PreferredProviders); err != nil {
			// Old format: array of provider ID strings
			var oldFormat []string
			if err2 := json.Unmarshal(providersJSON, &oldFormat); err2 != nil {
				return nil, fmt.Errorf("unmarshal providers: %w", err)
			}
			c.PreferredProviders = make([]ClientPreferredRoute, len(oldFormat))
			for i, p := range oldFormat {
				c.PreferredProviders[i] = ClientPreferredRoute{Provider: p}
			}
		}
	}
	c.LastRotatedAt = lastRotated
	return &c, nil
}

// Create inserts a new client and returns the full entity.
func (r *PostgresRepository) Create(ctx context.Context, input CreateClientInput) (*Client, error) {
	providersJSON, _ := json.Marshal(input.PreferredProviders)
	if input.PreferredProviders == nil {
		providersJSON = []byte("[]")
	}

	row := r.pool.QueryRow(ctx,
		`INSERT INTO clients (client_id, client_secret_hash, name, encryption_key, encryption_secret, preferred_providers)
		 VALUES (gen_random_uuid()::text, $1, $2, $3, $4, $5)
		 RETURNING `+clientColumns,
		input.ClientSecret, input.Name, input.EncryptionKey, input.EncryptionSecret, providersJSON,
	)
	return scanClient(row)
}

// GetByID retrieves a client by its UUID primary key.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Client, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+clientColumns+` FROM clients WHERE id = $1`, id,
	)
	return scanClient(row)
}

// GetByClientID retrieves a client by its unique client_id string.
func (r *PostgresRepository) GetByClientID(ctx context.Context, clientID string) (*Client, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+clientColumns+` FROM clients WHERE client_id = $1`, clientID,
	)
	return scanClient(row)
}

// List returns a paginated, optionally filtered list of clients.
func (r *PostgresRepository) List(ctx context.Context, filter ClientFilter) (*ClientList, error) {
	args := []any{}
	where := ""

	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where = " WHERE status = $1"
	}

	// Count
	var total int
	countQuery := "SELECT COUNT(*) FROM clients" + where
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count clients: %w", err)
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
	paramIdx := len(args) - 1

	rows, err := r.pool.Query(ctx,
		`SELECT `+clientColumns+` FROM clients`+where+
			` ORDER BY created_at DESC LIMIT $`+itoa(paramIdx)+
			` OFFSET $`+itoa(paramIdx+1),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	var clients []Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, fmt.Errorf("scan client row: %w", err)
		}
		if c != nil {
			clients = append(clients, *c)
		}
	}

	if clients == nil {
		clients = []Client{}
	}

	return &ClientList{Clients: clients, Total: total}, nil
}

// Update applies partial updates to a client.
func (r *PostgresRepository) Update(ctx context.Context, id string, input UpdateClientInput) (*Client, error) {
	// Build SET clause dynamically
	sets := ""
	args := []any{}
	argIdx := 1

	if input.Name != nil {
		sets += fmt.Sprintf("name = $%d, ", argIdx)
		args = append(args, *input.Name)
		argIdx++
	}
	if input.Status != nil {
		sets += fmt.Sprintf("status = $%d, ", argIdx)
		args = append(args, string(*input.Status))
		argIdx++
	}
	if input.PreferredProviders != nil {
		providersJSON, _ := json.Marshal(*input.PreferredProviders)
		sets += fmt.Sprintf("preferred_providers = $%d, ", argIdx)
		args = append(args, providersJSON)
		argIdx++
	}

	if sets == "" {
		return r.GetByID(ctx, id)
	}

	sets += "updated_at = NOW()"
	args = append(args, id)

	row := r.pool.QueryRow(ctx,
		`UPDATE clients SET `+sets+` WHERE id = $`+itoa(argIdx)+
			` RETURNING `+clientColumns,
		args...,
	)
	return scanClient(row)
}

// UpdateStatus is a convenience wrapper for status-only updates.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id string, status ClientStatus) (*Client, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE clients SET status = $1, updated_at = NOW() WHERE id = $2 RETURNING `+clientColumns,
		string(status), id,
	)
	return scanClient(row)
}

// RotateKeys updates the client secret hash and encryption material.
func (r *PostgresRepository) RotateKeys(ctx context.Context, id string, secretHash, encKey, encSecret string) (*Client, error) {
	now := time.Now()
	row := r.pool.QueryRow(ctx,
		`UPDATE clients
		 SET client_secret_hash = $1, encryption_key = $2, encryption_secret = $3,
		     last_rotated_at = $4, updated_at = $5
		 WHERE id = $6
		 RETURNING `+clientColumns,
		secretHash, encKey, encSecret, now, now, id,
	)
	return scanClient(row)
}

// Delete permanently removes a client.
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM clients WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete client: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("client not found")
	}
	return nil
}

// itoa is a shortcut for strconv.Itoa.
func itoa(i int) string {
	return strconv.Itoa(i)
}
