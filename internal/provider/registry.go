package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ai-proxy/internal/logger"
)

/* ─── Repository Interface ──────────────────────────────── */

// Repository defines the data-access contract for providers.
type Repository interface {
	Create(ctx context.Context, input CreateProviderInput) (*Provider, error)
	GetByID(ctx context.Context, id string) (*Provider, error)
	GetByProviderID(ctx context.Context, providerID ProviderID) (*Provider, error)
	List(ctx context.Context, enabledOnly bool) ([]Provider, error)
	Update(ctx context.Context, id string, input UpdateProviderInput) (*Provider, error)
	Delete(ctx context.Context, id string) error
}

/* ─── PostgresRepository ────────────────────────────────── */

// PostgresRepository implements Repository backed by a pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new provider repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const providerColumns = `id, provider_id, name, api_key, base_url, enabled, models, created_at, updated_at`

func scanProvider(row pgx.Row) (*Provider, error) {
	var p Provider
	var modelsJSON []byte

	err := row.Scan(&p.ID, &p.ProviderID, &p.Name, &p.APIKey, &p.BaseURL, &p.Enabled, &modelsJSON, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan provider: %w", err)
	}

	if len(modelsJSON) > 0 {
		if err := json.Unmarshal(modelsJSON, &p.Models); err != nil {
			return nil, fmt.Errorf("unmarshal models: %w", err)
		}
	}
	return &p, nil
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateProviderInput) (*Provider, error) {
	modelsJSON, _ := json.Marshal(input.Models)
	if input.Models == nil {
		modelsJSON = []byte("[]")
	}

	row := r.pool.QueryRow(ctx,
		`INSERT INTO providers (provider_id, name, api_key, base_url, models)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+providerColumns,
		string(input.ProviderID), input.Name, input.APIKey, input.BaseURL, modelsJSON,
	)
	return scanProvider(row)
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Provider, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+providerColumns+` FROM providers WHERE id = $1`, id)
	return scanProvider(row)
}

func (r *PostgresRepository) GetByProviderID(ctx context.Context, providerID ProviderID) (*Provider, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+providerColumns+` FROM providers WHERE provider_id = $1`, string(providerID))
	return scanProvider(row)
}

func (r *PostgresRepository) List(ctx context.Context, enabledOnly bool) ([]Provider, error) {
	query := `SELECT ` + providerColumns + ` FROM providers`
	if enabledOnly {
		query += ` WHERE enabled = true`
	}
	query += ` ORDER BY name ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	var providers []Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		if p != nil {
			providers = append(providers, *p)
		}
	}
	if providers == nil {
		providers = []Provider{}
	}
	return providers, nil
}

func (r *PostgresRepository) Update(ctx context.Context, id string, input UpdateProviderInput) (*Provider, error) {
	args := []any{}
	argIdx := 1
	sets := ""

	if input.Name != nil {
		sets += fmt.Sprintf("name = $%d, ", argIdx)
		args = append(args, *input.Name)
		argIdx++
	}
	if input.APIKey != nil {
		sets += fmt.Sprintf("api_key = $%d, ", argIdx)
		args = append(args, *input.APIKey)
		argIdx++
	}
	if input.BaseURL != nil {
		sets += fmt.Sprintf("base_url = $%d, ", argIdx)
		args = append(args, *input.BaseURL)
		argIdx++
	}
	if input.Enabled != nil {
		sets += fmt.Sprintf("enabled = $%d, ", argIdx)
		args = append(args, *input.Enabled)
		argIdx++
	}
	if input.Models != nil {
		modelsJSON, _ := json.Marshal(*input.Models)
		sets += fmt.Sprintf("models = $%d, ", argIdx)
		args = append(args, modelsJSON)
		argIdx++
	}

	if sets == "" {
		return r.GetByID(ctx, id)
	}

	sets += "updated_at = NOW()"
	args = append(args, id)
	row := r.pool.QueryRow(ctx,
		`UPDATE providers SET `+sets+` WHERE id = $`+itoa(argIdx)+` RETURNING `+providerColumns,
		args...,
	)
	return scanProvider(row)
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM providers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

/* ─── Registry (in-memory + db-backed) ──────────────────── */

// Registry maintains an in-memory view of enabled providers for fast routing.
// It can be refreshed from the database periodically.
type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderID]*Provider // keyed by provider_id slug
	repo      Repository
}

// NewRegistry creates a registry and loads providers from the repository.
func NewRegistry(repo Repository) *Registry {
	return &Registry{
		providers: make(map[ProviderID]*Provider),
		repo:      repo,
	}
}

// Refresh reloads all enabled providers from the database into memory.
func (r *Registry) Refresh(ctx context.Context) error {
	providers, err := r.repo.List(ctx, true)
	if err != nil {
		return fmt.Errorf("refresh registry: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers = make(map[ProviderID]*Provider, len(providers))
	for _, p := range providers {
		r.providers[p.ProviderID] = &p
	}

	logger.FromContext(ctx).Info("provider registry refreshed",
		slog.Int("count", len(r.providers)),
	)
	return nil
}

// Get returns a provider by its slug, or nil if not found / disabled.
func (r *Registry) Get(providerID ProviderID) *Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[providerID]
}

// GetByModel finds the first enabled provider that supports the given model.
// Returns nil if no provider supports the model.
func (r *Registry) GetByModel(model string) (*Provider, ProviderID) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.providers {
		for _, m := range p.Models {
			if m == model {
				return p, p.ProviderID
			}
		}
	}
	return nil, ""
}

// All returns a snapshot of all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, *p)
	}
	return result
}

// Count returns the number of registered providers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
