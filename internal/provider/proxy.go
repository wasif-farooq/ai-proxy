package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"ai-proxy/internal/client"
	"ai-proxy/internal/logger"
)

// Proxy handles forwarding API requests to upstream AI providers.
type Proxy struct {
	client        *http.Client
	registry      *Registry
	providerKeySvc *client.ProviderKeyService
}

// sseBufPool reuses 32KB buffers for SSE streaming to reduce GC pressure.
var sseBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

// NewProxy creates a proxy with a configurable HTTP client.
// The transport is configured for HTTP/2 (h2c) to support multiplexing
// over upstream connections, reducing per-request latency.
func NewProxy(registry *Registry, providerKeySvc *client.ProviderKeyService, timeout time.Duration) *Proxy {
	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	// Enable HTTP/2 for upstream connections (most AI providers support it)
	if err := http2.ConfigureTransport(transport); err != nil {
		slog.Warn("failed to configure HTTP/2 transport", slog.String("error", err.Error()))
	}
	return &Proxy{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		registry:       registry,
		providerKeySvc: providerKeySvc,
	}
}

// Forward sends a request to the upstream AI provider and returns the response.
// It handles both regular (JSON) and streaming (SSE) responses.
func (p *Proxy) Forward(ctx context.Context, providerID ProviderID, model string, body []byte) (*RouteResponse, error) {
	prov := p.registry.Get(providerID)
	if prov == nil {
		return nil, fmt.Errorf("provider %q not found or disabled", providerID)
	}

	apiKey, err := p.resolveAPIKey(ctx, prov, providerID, model)
	if err != nil {
		return nil, err
	}

	// Build upstream URL
	upstreamURL := prov.BaseURL + "/chat/completions"

	// Create upstream request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	start := time.Now()

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	latencyMs := time.Since(start).Milliseconds()

	// Read response body
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	result := &RouteResponse{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
		Body:       respBody,
		Model:      model,
	}

	// Copy relevant response headers
	for _, h := range []string{"Content-Type", "X-Request-ID"} {
		if v := resp.Header.Get(h); v != "" {
			result.Headers[h] = v
		}
	}

	logger.FromContext(ctx).Debug("upstream request completed",
		slog.String(logger.KeyProviderID, string(providerID)),
		slog.String(logger.KeyModel, model),
		slog.Int(logger.KeyStatusCode, resp.StatusCode),
		slog.Int64(logger.KeyLatencyMs, latencyMs),
	)

	return result, nil
}

// ForwardStreaming handles streaming (SSE) responses by copying chunks directly
// to the downstream response writer. The caller must not write to w after this returns.
// resolveAPIKey resolves the API key for a provider, preferring per-client keys
// over the global provider key. Returns an error if the requested model is not
// allowed by the per-client key's model restrictions.
func (p *Proxy) resolveAPIKey(ctx context.Context, prov *Provider, providerID ProviderID, model string) (string, error) {
	apiKey := prov.APIKey
	if cl := extractClient(ctx); cl != nil {
		clientKey, allowedModels, err := p.providerKeySvc.GetDecryptedWithModelsForClient(ctx, cl, string(providerID))
		if err != nil {
			logger.FromContext(ctx).Warn("failed to resolve per-client provider key, falling back to global",
				slog.String(logger.KeyClientID, cl.ClientID),
				slog.String(logger.KeyProviderID, string(providerID)),
				slog.String("error", err.Error()),
			)
		} else if clientKey != "" {
			// Check if the requested model is allowed by this per-client key
			if len(allowedModels) > 0 {
				modelAllowed := false
				for _, m := range allowedModels {
					if m == model {
						modelAllowed = true
						break
					}
				}
				if !modelAllowed {
					return "", fmt.Errorf("model %q is not allowed for this client on provider %q", model, providerID)
				}
			}
			apiKey = clientKey
			logger.FromContext(ctx).Debug("using per-client provider key",
				slog.String(logger.KeyClientID, cl.ClientID),
				slog.String(logger.KeyProviderID, string(providerID)),
			)
		}
	}
	return apiKey, nil
}

func (p *Proxy) ForwardStreaming(ctx context.Context, w http.ResponseWriter, providerID ProviderID, model string, body []byte) error {
	prov := p.registry.Get(providerID)
	if prov == nil {
		return fmt.Errorf("provider %q not found or disabled", providerID)
	}

	apiKey, err := p.resolveAPIKey(ctx, prov, providerID, model)
	if err != nil {
		return err
	}

	upstreamURL := prov.BaseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create streaming request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	start := time.Now()

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	// Set downstream headers for SSE streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	// Stream chunks directly to the client
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support flushing")
	}

	// Use pooled buffer to reduce GC pressure
	bufPtr := sseBufPool.Get().(*[]byte)
	buf := *bufPtr
	written, err := io.CopyBuffer(w, resp.Body, buf)
	sseBufPool.Put(bufPtr)
	flusher.Flush()

	latencyMs := time.Since(start).Milliseconds()

	logger.FromContext(ctx).Debug("upstream streaming completed",
		slog.String(logger.KeyProviderID, string(providerID)),
		slog.String(logger.KeyModel, model),
		slog.Int64(logger.KeyLatencyMs, latencyMs),
		slog.Int64("bytes_written", written),
	)

	if err != nil && err != io.EOF {
		return fmt.Errorf("stream upstream response: %w", err)
	}

	return nil
}

// extractClientID extracts the client_id from the context (set by AuthMiddleware).
func extractClientID(ctx context.Context) string {
	if c, ok := ctx.Value(clientIDKey{}).(string); ok {
		return c
	}
	return ""
}

// extractClient extracts the full *Client from the context (set by AuthMiddleware).
func extractClient(ctx context.Context) *client.Client {
	if cl, ok := ctx.Value(clientKey{}).(*client.Client); ok {
		return cl
	}
	return nil
}

// clientIDKey is a context key type to avoid collisions.
type clientIDKey struct{}

// clientKey is a context key type for the full *Client value.
type clientKey struct{}
