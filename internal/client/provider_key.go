package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ai-proxy/internal/client/encryption"
)

/* ─── Model ──────────────────────────────────────────────── */

// ClientProviderKey represents a per-client API key for a specific provider.
// The API key is encrypted at rest using the client's encryption material.
type ClientProviderKey struct {
	ID        string     `json:"id"`
	ClientID  string     `json:"client_id"`
	Provider  string     `json:"provider"`
	APIKey    string     `json:"-"` // never serialised; returned once on create/update
	BaseURL   *string    `json:"base_url,omitempty"`
	Models    []string   `json:"models,omitempty"` // nil = all models allowed
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SetClientProviderKeyInput carries fields needed to set a provider key for a client.
type SetClientProviderKeyInput struct {
	ClientID string
	Provider string
	APIKey   string // raw plaintext key (will be encrypted before storage)
	BaseURL  *string
	Models   []string // nil or empty = all models allowed
}

// ClientProviderKeyListItem is the public representation returned in lists (no secrets).
type ClientProviderKeyListItem struct {
	Provider    string     `json:"provider"`
	HasKey      bool       `json:"has_key"`
	BaseURL     *string    `json:"base_url,omitempty"`
	Models      []string   `json:"models,omitempty"` // nil = all models allowed; empty = no models
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

/* ─── Repository Interface ───────────────────────────────── */

// ClientProviderKeyRepository defines the data-access contract for client provider keys.
type ClientProviderKeyRepository interface {
	Set(ctx context.Context, input SetClientProviderKeyInput, encryptedKey string) (*ClientProviderKey, error)
	Get(ctx context.Context, clientID, provider string) (*ClientProviderKey, error)
	Delete(ctx context.Context, clientID, provider string) error
	List(ctx context.Context, clientID string) ([]ClientProviderKeyListItem, error)
	DeleteAllForClient(ctx context.Context, clientID string) error
}

/* ─── PostgresRepository ─────────────────────────────────── */

// PostgresClientProviderKeyRepository implements ClientProviderKeyRepository.
type PostgresClientProviderKeyRepository struct {
	pool *pgxpool.Pool
}

// NewClientProviderKeyRepository creates a new repository.
func NewClientProviderKeyRepository(pool *pgxpool.Pool) *PostgresClientProviderKeyRepository {
	return &PostgresClientProviderKeyRepository{pool: pool}
}

const providerKeyColumns = `id, client_id, provider, api_key, base_url, models, created_at, updated_at`

func scanProviderKey(row pgx.Row) (*ClientProviderKey, error) {
	var k ClientProviderKey
	var modelsJSON []byte

	err := row.Scan(&k.ID, &k.ClientID, &k.Provider, &k.APIKey, &k.BaseURL, &modelsJSON, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan provider key: %w", err)
	}

	if len(modelsJSON) > 0 {
		if err := json.Unmarshal(modelsJSON, &k.Models); err != nil {
			return nil, fmt.Errorf("unmarshal models: %w", err)
		}
	}
	return &k, nil
}

func (r *PostgresClientProviderKeyRepository) Set(ctx context.Context, input SetClientProviderKeyInput, encryptedKey string) (*ClientProviderKey, error) {
	var modelsJSON []byte
	if input.Models != nil {
		modelsJSON, _ = json.Marshal(input.Models)
	}

	row := r.pool.QueryRow(ctx,
		`INSERT INTO client_provider_keys (client_id, provider, api_key, base_url, models)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (client_id, provider) DO UPDATE SET
		   api_key = EXCLUDED.api_key,
		   base_url = COALESCE(EXCLUDED.base_url, client_provider_keys.base_url),
		   models = COALESCE(EXCLUDED.models, client_provider_keys.models),
		   updated_at = NOW()
		 RETURNING `+providerKeyColumns,
		input.ClientID, input.Provider, encryptedKey, input.BaseURL, modelsJSON,
	)
	return scanProviderKey(row)
}

func (r *PostgresClientProviderKeyRepository) Get(ctx context.Context, clientID, provider string) (*ClientProviderKey, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+providerKeyColumns+` FROM client_provider_keys WHERE client_id = $1 AND provider = $2`,
		clientID, provider,
	)
	return scanProviderKey(row)
}

func (r *PostgresClientProviderKeyRepository) Delete(ctx context.Context, clientID, provider string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM client_provider_keys WHERE client_id = $1 AND provider = $2`,
		clientID, provider,
	)
	if err != nil {
		return fmt.Errorf("delete provider key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("provider key not found")
	}
	return nil
}

func (r *PostgresClientProviderKeyRepository) List(ctx context.Context, clientID string) ([]ClientProviderKeyListItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT provider, api_key <> '' AS has_key, base_url, models, created_at, updated_at
		 FROM client_provider_keys
		 WHERE client_id = $1
		 ORDER BY provider ASC`,
		clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("list provider keys: %w", err)
	}
	defer rows.Close()

	var items []ClientProviderKeyListItem
	for rows.Next() {
		var item ClientProviderKeyListItem
		var modelsJSON []byte
		if err := rows.Scan(&item.Provider, &item.HasKey, &item.BaseURL, &modelsJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan provider key list item: %w", err)
		}
		if len(modelsJSON) > 0 {
			if err := json.Unmarshal(modelsJSON, &item.Models); err != nil {
				return nil, fmt.Errorf("unmarshal models: %w", err)
			}
		}
		items = append(items, item)
	}
	if items == nil {
		items = []ClientProviderKeyListItem{}
	}
	return items, nil
}

func (r *PostgresClientProviderKeyRepository) DeleteAllForClient(ctx context.Context, clientID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM client_provider_keys WHERE client_id = $1`,
		clientID,
	)
	if err != nil {
		return fmt.Errorf("delete all provider keys: %w", err)
	}
	return nil
}

/* ─── Service ────────────────────────────────────────────── */

// ProviderKeyService handles business logic for per-client provider API keys.
type ProviderKeyService struct {
	keyRepo   ClientProviderKeyRepository
	clientSvc *Service // for looking up client encryption material
	masterKey string
}

// NewProviderKeyService creates a new provider key service.
func NewProviderKeyService(keyRepo ClientProviderKeyRepository, clientSvc *Service, masterKey string) *ProviderKeyService {
	return &ProviderKeyService{
		keyRepo:   keyRepo,
		clientSvc: clientSvc,
		masterKey: masterKey,
	}
}

// deriveKey derives an AES key from the master key and client-specific encryption key.
func (s *ProviderKeyService) deriveKey(client *Client) []byte {
	return encryption.DeriveKey(s.masterKey, client.EncryptionKey)
}

// Set sets a provider API key for a client. Returns the raw key in the response (shown once).
func (s *ProviderKeyService) Set(ctx context.Context, input SetClientProviderKeyInput) (*ClientProviderKey, string, error) {
	client, err := s.clientSvc.GetByClientID(ctx, input.ClientID)
	if err != nil {
		return nil, "", fmt.Errorf("lookup client: %w", err)
	}
	if client == nil {
		return nil, "", fmt.Errorf("client not found")
	}

	// Encrypt the API key using the client's encryption material
	encryptedKey, err := encryption.Encrypt(s.deriveKey(client), []byte(input.APIKey))
	if err != nil {
		return nil, "", fmt.Errorf("encrypt provider key: %w", err)
	}

	stored, err := s.keyRepo.Set(ctx, input, encryptedKey)
	if err != nil {
		return nil, "", fmt.Errorf("set provider key: %w", err)
	}

	return stored, input.APIKey, nil
}

// GetDecrypted retrieves and decrypts a provider key for a client.
// Returns the decrypted key, or empty string if not found.
func (s *ProviderKeyService) GetDecrypted(ctx context.Context, clientID, provider string) (string, error) {
	key, _, err := s.GetDecryptedWithModels(ctx, clientID, provider)
	return key, err
}

// GetDecryptedWithModels retrieves and decrypts a provider key for a client,
// along with any model restrictions. An empty models slice means all models are allowed.
func (s *ProviderKeyService) GetDecryptedWithModels(ctx context.Context, clientID, provider string) (string, []string, error) {
	client, err := s.clientSvc.GetByClientID(ctx, clientID)
	if err != nil {
		return "", nil, fmt.Errorf("lookup client for decryption: %w", err)
	}
	if client == nil {
		return "", nil, fmt.Errorf("client not found")
	}

	stored, err := s.keyRepo.Get(ctx, clientID, provider)
	if err != nil {
		return "", nil, err
	}
	if stored == nil || stored.APIKey == "" {
		return "", nil, nil
	}

	plain, err := encryption.Decrypt(s.deriveKey(client), stored.APIKey)
	if err != nil {
		return "", nil, fmt.Errorf("decrypt provider key: %w", err)
	}

	// Return allowed models (nil = all allowed, empty slice sent as nil)
	allowedModels := stored.Models
	if allowedModels == nil {
		allowedModels = []string{}
	}
	return string(plain), allowedModels, nil
}

// Delete removes a provider key for a client.
func (s *ProviderKeyService) Delete(ctx context.Context, clientID, provider string) error {
	return s.keyRepo.Delete(ctx, clientID, provider)
}

// List returns the public list of provider keys for a client (no secrets).
func (s *ProviderKeyService) List(ctx context.Context, clientID string) ([]ClientProviderKeyListItem, error) {
	return s.keyRepo.List(ctx, clientID)
}

// DeleteAllForClient removes all provider keys when a client is deleted.
func (s *ProviderKeyService) DeleteAllForClient(ctx context.Context, clientID string) error {
	return s.keyRepo.DeleteAllForClient(ctx, clientID)
}
