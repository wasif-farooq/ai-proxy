package provider

import (
	"time"
)

// ProviderID is a unique identifier for an upstream AI provider.
type ProviderID string

const (
	ProviderOpenAI    ProviderID = "openai"
	ProviderAnthropic ProviderID = "anthropic"
	ProviderGoogle    ProviderID = "google"
	ProviderAzure     ProviderID = "azure"
	ProviderOllama    ProviderID = "ollama"
	ProviderDeepSeek  ProviderID = "deepseek"
	ProviderCustom    ProviderID = "custom"
)

// ValidProviderIDs contains all known provider identifiers.
var ValidProviderIDs = []ProviderID{
	ProviderOpenAI,
	ProviderAnthropic,
	ProviderGoogle,
	ProviderAzure,
	ProviderOllama,
	ProviderDeepSeek,
	ProviderCustom,
}

// BaseURLs maps provider IDs to their default API base URLs.
var BaseURLs = map[ProviderID]string{
	ProviderOpenAI:    "https://api.openai.com/v1",
	ProviderAnthropic: "https://api.anthropic.com/v1",
	ProviderGoogle:    "https://generativelanguage.googleapis.com/v1",
	ProviderAzure:     "https://YOUR_RESOURCE.openai.azure.com/v1",
	ProviderOllama:    "http://localhost:11434/v1",
	ProviderDeepSeek:  "https://api.deepseek.com/v1",
}

// IsValidProviderID returns true if the given string is a known provider ID.
func IsValidProviderID(s string) bool {
	for _, p := range ValidProviderIDs {
		if string(p) == s {
			return true
		}
	}
	return false
}

// Provider represents an upstream AI provider configuration.
type Provider struct {
	ID         string     `json:"id"`          // internal UUID
	ProviderID ProviderID `json:"provider_id"` // slug: openai, anthropic, etc.
	Name       string     `json:"name"`
	APIKey     string     `json:"-"` // never serialised
	BaseURL    string     `json:"base_url"`
	Enabled    bool       `json:"enabled"`
	Models     []string   `json:"models"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateProviderInput carries fields needed to register a new provider.
type CreateProviderInput struct {
	ProviderID ProviderID
	Name       string
	APIKey     string
	BaseURL    string
	Models     []string
}

// UpdateProviderInput carries optional fields for updating a provider.
type UpdateProviderInput struct {
	Name    *string   `json:"name"`
	APIKey  *string   `json:"api_key"`
	BaseURL *string   `json:"base_url"`
	Enabled *bool     `json:"enabled"`
	Models  *[]string `json:"models"`
}

// RouteResponse is the result of forwarding a request to a provider.
type RouteResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Model      string
}
