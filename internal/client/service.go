package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"ai-proxy/internal/client/encryption"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/shared"
)

// Service implements business logic for client management, orchestrating
// the repository (persistent storage) and cache (fast-path lookups).
type Service struct {
	repo      Repository
	cache     *Cache
	masterKey string
}

// NewService creates a new client service.
func NewService(repo Repository, masterKey string) *Service {
	return &Service{
		repo:      repo,
		cache:     NewCache(5 * time.Minute),
		masterKey: masterKey,
	}
}

// Stop shuts down the background cache eviction goroutine.
func (s *Service) Stop() {
	s.cache.Stop()
}

// generateClientSecret creates a cryptographically random client secret.
func generateClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate client secret: %w", err)
	}
	return "sk-" + base64.RawURLEncoding.EncodeToString(b), nil
}

// Create registers a new client and returns the full entity with its
// plain-text secret, encryption key, and encryption secret (all shown once).
// The secret is hashed before storage.
func (s *Service) Create(ctx context.Context, name string, providers []ClientPreferredRoute) (*Client, string, string, string, error) {
	if name == "" {
		return nil, "", "", "", shared.ErrValidation.WithDetail("client name is required")
	}

	// Generate credentials
	secret, err := generateClientSecret()
	if err != nil {
		return nil, "", "", "", shared.ErrInternal.WithDetail(err.Error())
	}
	secretHash := encryption.HashClientSecret(secret)

	// Generate encryption material
	encKey, err := encryption.GenerateSecret()
	if err != nil {
		return nil, "", "", "", shared.ErrInternal.WithDetail(err.Error())
	}
	encSecret, err := encryption.GenerateSecret()
	if err != nil {
		return nil, "", "", "", shared.ErrInternal.WithDetail(err.Error())
	}

	input := CreateClientInput{
		Name:               name,
		PreferredProviders: providers,
		ClientSecret:       secretHash,
		EncryptionKey:      encKey,
		EncryptionSecret:   encSecret,
	}

	client, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("create client: %w", err)
	}

	// Cache the new client
	s.cache.Set(client)

	logger.FromContext(ctx).Info("client created",
		slog.String(logger.KeyClientID, client.ClientID),
		slog.String("name", client.Name),
	)
	return client, secret, encKey, encSecret, nil
}

// GetByClientID retrieves a client by its client_id, checking cache first.
func (s *Service) GetByClientID(ctx context.Context, clientID string) (*Client, error) {
	// Fast path: check cache
	if cached := s.cache.Get(clientID); cached != nil {
		return cached, nil
	}

	// Slow path: query database
	client, err := s.repo.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, shared.ErrNotFound.WithDetail("client not found")
	}

	// Populate cache
	s.cache.Set(client)
	return client, nil
}

// GetByID retrieves a client by its internal UUID.
func (s *Service) GetByID(ctx context.Context, id string) (*Client, error) {
	client, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, shared.ErrNotFound.WithDetail("client not found")
	}
	return client, nil
}

// List returns a paginated list of clients.
func (s *Service) List(ctx context.Context, filter ClientFilter) (*ClientList, error) {
	return s.repo.List(ctx, filter)
}

// Update applies partial updates to a client.
func (s *Service) Update(ctx context.Context, id string, input UpdateClientInput) (*Client, error) {
	// Verify client exists
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, shared.ErrNotFound.WithDetail("client not found")
	}

	client, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return nil, fmt.Errorf("update client: %w", err)
	}

	// Refresh cache
	s.cache.Set(client)
	return client, nil
}

// UpdateStatus changes a client's lifecycle status.
func (s *Service) UpdateStatus(ctx context.Context, id string, status ClientStatus) (*Client, error) {
	if !IsValidStatus(string(status)) {
		return nil, shared.ErrValidation.WithDetail(
			fmt.Sprintf("invalid status: %q; must be one of %v", status, ValidClientStatuses),
		)
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, shared.ErrNotFound.WithDetail("client not found")
	}

	client, err := s.repo.UpdateStatus(ctx, id, status)
	if err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	// If revoked, remove from cache; otherwise refresh
	if status == ClientStatusRevoked {
		s.cache.Delete(client.ClientID)
	} else {
		s.cache.Set(client)
	}

	logger.FromContext(ctx).Info("client status updated",
		slog.String(logger.KeyClientID, client.ClientID),
		slog.String("status", string(status)),
	)
	return client, nil
}

// RotateKeys generates new credentials for a client and updates the database.
func (s *Service) RotateKeys(ctx context.Context, id string) (*Client, string, string, string, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, "", "", "", err
	}
	if existing == nil {
		return nil, "", "", "", shared.ErrNotFound.WithDetail("client not found")
	}

	// Generate new credentials
	secret, err := generateClientSecret()
	if err != nil {
		return nil, "", "", "", shared.ErrInternal.WithDetail(err.Error())
	}
	secretHash := encryption.HashClientSecret(secret)

	encKey, err := encryption.GenerateSecret()
	if err != nil {
		return nil, "", "", "", shared.ErrInternal.WithDetail(err.Error())
	}
	encSecret, err := encryption.GenerateSecret()
	if err != nil {
		return nil, "", "", "", shared.ErrInternal.WithDetail(err.Error())
	}

	client, err := s.repo.RotateKeys(ctx, id, secretHash, encKey, encSecret)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("rotate keys: %w", err)
	}

	s.cache.Set(client)

	logger.FromContext(ctx).Info("client keys rotated",
		slog.String(logger.KeyClientID, client.ClientID),
	)
	return client, secret, encKey, encSecret, nil
}

// ValidateClientSecret compares a plain-text secret against the stored hash.
func (s *Service) ValidateClientSecret(client *Client, plainSecret string) bool {
	return encryption.HashClientSecret(plainSecret) == client.ClientSecretHash
}

// Delete permanently removes a client.
func (s *Service) Delete(ctx context.Context, id string) error {
	client, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if client == nil {
		return shared.ErrNotFound.WithDetail("client not found")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete client: %w", err)
	}

	s.cache.Delete(client.ClientID)
	logger.FromContext(ctx).Info("client deleted",
		slog.String(logger.KeyClientID, client.ClientID),
	)
	return nil
}
