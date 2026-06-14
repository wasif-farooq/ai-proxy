package client

import "time"

// ClientStatus represents the lifecycle state of an API client.
type ClientStatus string

const (
	ClientStatusActive    ClientStatus = "active"
	ClientStatusSuspended ClientStatus = "suspended"
	ClientStatusRevoked   ClientStatus = "revoked"
)

// ValidClientStatuses contains all valid client status values.
var ValidClientStatuses = []ClientStatus{
	ClientStatusActive,
	ClientStatusSuspended,
	ClientStatusRevoked,
}

// IsValidStatus returns true if the given status is valid.
func IsValidStatus(s string) bool {
	for _, v := range ValidClientStatuses {
		if string(v) == s {
			return true
		}
	}
	return false
}

// ClientPreferredRoute binds a specific model to a provider for routing priority.
type ClientPreferredRoute struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Client represents a registered API client.
type Client struct {
	ID                 string                 `json:"id"`
	ClientID           string                 `json:"client_id"`
	ClientSecretHash   string                 `json:"-"` // never serialised
	Name               string                 `json:"name"`
	Status             ClientStatus           `json:"status"`
	EncryptionKey      string                 `json:"-"`
	EncryptionSecret   string                 `json:"-"`
	PreferredProviders []ClientPreferredRoute `json:"preferred_providers"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	LastRotatedAt      *time.Time             `json:"last_rotated_at,omitempty"`
}

// CreateClientInput carries fields needed to register a new client.
type CreateClientInput struct {
	Name               string
	PreferredProviders []ClientPreferredRoute
	ClientSecret       string // plain-text secret to hash before persisting
	EncryptionKey      string
	EncryptionSecret   string
}

// UpdateClientInput carries optional fields for updating a client.
type UpdateClientInput struct {
	Name               *string                `json:"name"`
	Status             *ClientStatus          `json:"status"`
	PreferredProviders *[]ClientPreferredRoute `json:"preferred_providers"`
}

// ClientFilter carries optional filter fields for listing clients.
type ClientFilter struct {
	Status *ClientStatus
	Limit  int
	Offset int
}

// ClientList is the result of a paginated client query.
type ClientList struct {
	Clients []Client `json:"clients"`
	Total   int      `json:"total"`
}
