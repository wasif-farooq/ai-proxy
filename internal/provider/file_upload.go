package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-proxy/internal/logger"
	"ai-proxy/internal/shared"
)

// FileUploadResponse contains the result of a file upload to an upstream provider.
type FileUploadResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	ProviderID ProviderID
}

// FileUploadMiddleware returns a Gin handler that accepts multipart file uploads
// and forwards them to the appropriate AI provider's Files API.
//
// The consumer sends a multipart/form-data POST with:
//   - file:     The file binary
//   - provider: Target provider slug (e.g. "openai", "anthropic")
//   - purpose:  File purpose (optional, provider-specific; e.g. "assistants", "user_data")
//
// The handler authenticates the client first (via DualAuthMiddleware), then resolves
// the provider's API key and base URL, and forwards the file upload upstream.
//
// Supported providers:
//   - OpenAI:   POST {base_url}/files  (Authorization: Bearer)
//   - Anthropic: POST {base_url}/files  (x-api-key + anthropic-beta header)
//   - Google:   POST {base_url}/files  (x-goog-api-key)
func FileUploadMiddleware(proxy *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse multipart form (32MB max, temp files for larger)
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			shared.SendValidationError(c, "invalid multipart form: "+err.Error())
			return
		}

		providerID := strings.TrimSpace(c.Request.FormValue("provider"))
		if providerID == "" {
			shared.SendValidationError(c, "provider field is required (e.g. openai, anthropic, google)")
			return
		}
		if !IsValidProviderID(providerID) {
			shared.SendValidationError(c, fmt.Sprintf("invalid provider %q; must be one of %v", providerID, ValidProviderIDs))
			return
		}

		// Read the file from the multipart form
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			shared.SendValidationError(c, "file field is required: "+err.Error())
			return
		}
		defer file.Close()

		fileBytes, err := io.ReadAll(file)
		if err != nil {
			shared.SendError(c, shared.ErrInternal.WithDetail("failed to read file: "+err.Error()))
			return
		}
		if len(fileBytes) == 0 {
			shared.SendValidationError(c, "file is empty")
			return
		}

		purpose := c.Request.FormValue("purpose")

		result, err := proxy.ForwardFileUpload(c.Request.Context(), ProviderID(providerID), fileBytes, header.Filename, purpose)
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("file upload failed",
				slog.String(logger.KeyProviderID, providerID),
				slog.String("error", err.Error()),
			)
			shared.SendError(c, shared.ErrBadGateway.WithDetail(err.Error()))
			return
		}

		// Copy response headers
		for k, v := range result.Headers {
			c.Header(k, v)
		}

		c.Data(result.StatusCode, "application/json", result.Body)
	}
}

// ForwardFileUpload sends a file to the upstream provider's Files API.
// It determines the correct auth method and endpoint path per provider.
func (p *Proxy) ForwardFileUpload(ctx context.Context, providerID ProviderID, fileBytes []byte, filename, purpose string) (*FileUploadResponse, error) {
	prov := p.registry.Get(providerID)
	if prov == nil {
		return nil, fmt.Errorf("provider %q not found or disabled", providerID)
	}

	// Use the provider's API key directly
	// TODO: integrate resolveAPIKey to support per-client provider keys for file uploads
	apiKey := prov.APIKey

	// Build upstream URL
	// OpenAI/Anthropic: {base_url}/files  (e.g. https://api.openai.com/v1/files)
	// Google: uses /v1beta/files (different version prefix)
	var upstreamURL string
	if providerID == ProviderGoogle {
		// Google's file upload endpoint is at /v1beta/files, not /v1/files
		upstreamURL = "https://generativelanguage.googleapis.com/v1beta/files"
	} else {
		upstreamURL = strings.TrimRight(prov.BaseURL, "/") + "/files"
	}

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Write the file field
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	// Write the purpose field (optional, provider-specific)
	if purpose != "" {
		if err := writer.WriteField("purpose", purpose); err != nil {
			return nil, fmt.Errorf("write purpose: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart: %w", err)
	}

	// Create upstream request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Set provider-specific auth headers
	switch providerID {
	case ProviderOpenAI, ProviderDeepSeek, ProviderAzure, ProviderCustom:
		req.Header.Set("Authorization", "Bearer "+apiKey)

	case ProviderAnthropic:
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("anthropic-beta", "files-api-2025-04-14")

	case ProviderGoogle:
		req.Header.Set("x-goog-api-key", apiKey)

	case ProviderOllama:
		// Ollama doesn't require auth for local instances
		// but may use Authorization for remote instances
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream file upload failed: %w", err)
	}
	defer resp.Body.Close()

	latencyMs := time.Since(start).Milliseconds()

	// Read response body
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	result := &FileUploadResponse{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
		Body:       respBody,
		ProviderID: providerID,
	}

	for _, h := range []string{"Content-Type", "X-Request-ID"} {
		if v := resp.Header.Get(h); v != "" {
			result.Headers[h] = v
		}
	}

	logger.FromContext(ctx).Debug("file upload completed",
		slog.String(logger.KeyProviderID, string(providerID)),
		slog.Int("file_bytes", len(fileBytes)),
		slog.String("filename", filename),
		slog.Int(logger.KeyStatusCode, resp.StatusCode),
		slog.Int64(logger.KeyLatencyMs, latencyMs),
	)

	return result, nil
}
