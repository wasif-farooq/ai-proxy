package admin

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"ai-proxy/internal/audit"
	"ai-proxy/internal/client"
	"ai-proxy/internal/config"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/provider"
	"ai-proxy/internal/shared"
)

// Handler holds dependencies for admin HTTP handlers.
type Handler struct {
	cfg              *config.Config
	clientRepo       client.Repository
	clientSvc        *client.Service
	providerKeySvc   *client.ProviderKeyService
	providerRepo     provider.Repository
	providerReg      *provider.Registry
	auditRepo        audit.Repository
	auditSvc         *audit.Service
	db               *pgxpool.Pool
}

// NewHandler creates an admin handler with all required dependencies.
func NewHandler(
	cfg *config.Config,
	db *pgxpool.Pool,
	clientSvc *client.Service,
	clientRepo client.Repository,
	providerKeySvc *client.ProviderKeyService,
	providerRepo provider.Repository,
	providerReg *provider.Registry,
	auditRepo audit.Repository,
	auditSvc *audit.Service,
) *Handler {
	return &Handler{
		cfg:            cfg,
		db:             db,
		clientSvc:      clientSvc,
		clientRepo:     clientRepo,
		providerKeySvc: providerKeySvc,
		providerRepo:   providerRepo,
		providerReg:    providerReg,
		auditRepo:      auditRepo,
		auditSvc:       auditSvc,
	}
}

// RegisterRoutes mounts all admin routes on the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, jwtSecret string) {
	// Public auth routes (no JWT required)
	rg.POST("/auth/login", h.Login)

	// Protected admin routes
	admin := rg.Group("")
	admin.Use(AuthMiddleware(jwtSecret))
	{
		admin.GET("/me", h.Me)

		// Dashboard
		admin.GET("/dashboard/stats", h.DashboardStats)

		// Clients
		admin.GET("/clients", h.ListClients)
		admin.POST("/clients", AdminOnly(), h.CreateClient)
		admin.GET("/clients/:id", h.GetClient)
		admin.PUT("/clients/:id", AdminOnly(), h.UpdateClient)
		admin.DELETE("/clients/:id", SuperAdminOnly(), h.DeleteClient)
		admin.POST("/clients/:id/rotate", AdminOnly(), h.RotateClientKeys)
		admin.GET("/clients/:id/credentials", AdminOnly(), h.GetClientCredentials)
		admin.GET("/clients/:id/provider-keys", h.ListClientProviderKeys)
		admin.PUT("/clients/:id/provider-keys/:provider", AdminOnly(), h.SetClientProviderKey)
		admin.DELETE("/clients/:id/provider-keys/:provider", SuperAdminOnly(), h.DeleteClientProviderKey)

		// Providers
		admin.GET("/providers", h.ListProviders)
		admin.POST("/providers", SuperAdminOnly(), h.CreateProvider)
		admin.GET("/providers/:id", h.GetProvider)
		admin.PUT("/providers/:id", SuperAdminOnly(), h.UpdateProvider)
		admin.DELETE("/providers/:id", SuperAdminOnly(), h.DeleteProvider)

		// Audit Logs
		admin.GET("/audit-logs", h.ListAuditLogs)
		admin.GET("/audit-logs/:id", h.GetAuditLog)
	}
}

/* ─── Auth Handlers ──────────────────────────────────────── */

// Login authenticates an admin user and returns a JWT token.
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.SendValidationError(c, "email and password are required")
		return
	}

	// Validate against database
	var adminID, name, role, passwordHash string
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, name, role, password_hash FROM admin_users WHERE email = $1`,
		strings.ToLower(req.Email),
	).Scan(&adminID, &name, &role, &passwordHash)
	if err != nil {
		logger.FromContext(c.Request.Context()).Warn("admin login failed",
			slog.String("email", req.Email),
		)
		shared.SendError(c, shared.ErrUnauthorized.WithDetail("Invalid email or password"))
		return
	}

	// Verify password (using constant-time comparison of SHA-256 hashes)
	if !verifyPassword(req.Password, passwordHash) {
		shared.SendError(c, shared.ErrUnauthorized.WithDetail("Invalid email or password"))
		return
	}

	// Generate JWT
	claims := &AdminClaims{AdminID: adminID, Email: req.Email, Role: role}
	token, err := generateToken(h.cfg.JWTSecret, claims)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("token generation failed",
			slog.String("error", err.Error()),
		)
		shared.SendError(c, shared.ErrInternal)
		return
	}

	// Update last_login
	_, _ = h.db.Exec(c.Request.Context(),
		`UPDATE admin_users SET last_login = NOW() WHERE id = $1`, adminID,
	)

	shared.SendOK(c, LoginResponse{
		Token:     token,
		AdminID:   adminID,
		Email:     req.Email,
		Name:      name,
		Role:      role,
		ExpiresAt: claims.Exp,
	})
}

// Me returns the current admin's profile.
func (h *Handler) Me(c *gin.Context) {
	adminID, _ := c.Get("admin_id")
	email, _ := c.Get("admin_email")
	role, _ := c.Get("admin_role")

	var name string
	_ = h.db.QueryRow(c.Request.Context(),
		`SELECT name FROM admin_users WHERE id = $1`, adminID,
	).Scan(&name)

	shared.SendOK(c, gin.H{
		"admin_id": adminID,
		"email":    email,
		"name":     name,
		"role":     role,
	})
}

/* ─── Dashboard ──────────────────────────────────────────── */

// DashboardStats returns aggregate metrics for the admin dashboard.
func (h *Handler) DashboardStats(c *gin.Context) {
	var totalClients, activeClients, totalRequests, activeConnections int

	_ = h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM clients`).Scan(&totalClients)
	_ = h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM clients WHERE status = 'active'`).Scan(&activeClients)
	_ = h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM audit_logs WHERE event_type = 'api_request' AND timestamp > NOW() - INTERVAL '30 days'`).Scan(&totalRequests)
	_ = h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM providers WHERE enabled = true`).Scan(&activeConnections)

	shared.SendOK(c, gin.H{
		"total_clients":      totalClients,
		"active_clients":     activeClients,
		"total_requests_30d": totalRequests,
		"active_providers":   activeConnections,
	})
}

/* ─── Client Handlers ────────────────────────────────────── */

// ListClients returns a paginated list of clients.
func (h *Handler) ListClients(c *gin.Context) {
	filter := client.ClientFilter{}
	if s := c.Query("status"); s != "" {
		status := client.ClientStatus(s)
		filter.Status = &status
	}
	filter.Limit = queryInt(c, "limit", 50)
	filter.Offset = queryInt(c, "offset", 0)

	result, err := h.clientRepo.List(c.Request.Context(), filter)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("list clients failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal)
		return
	}

	shared.SendPaginated(c, result.Clients, result.Total, filter.Offset/filter.Limit+1, filter.Limit)
}

// CreateClient handles POST /clients.
func (h *Handler) CreateClient(c *gin.Context) {
	var req struct {
		Name               string                    `json:"name" binding:"required"`
		PreferredProviders []client.ClientPreferredRoute `json:"preferred_providers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.SendValidationError(c, "name is required")
		return
	}

	cl, secret, encKey, encSecret, err := h.clientSvc.Create(c.Request.Context(), req.Name, req.PreferredProviders)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("create client failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal.WithDetail(err.Error()))
		return
	}

	// Audit
	adminID, _ := c.Get("admin_id")
	adminIDStr, _ := adminID.(string)
	h.auditSvc.LogClientCreated(cl.ClientID, adminIDStr, c.ClientIP(), c.GetString("request_id"),
		nil, map[string]string{"name": cl.Name, "status": string(cl.Status)},
	)

	shared.SendCreated(c, gin.H{
		"client":             cl,
		"client_secret":      secret,
		"encryption_key":     encKey,
		"encryption_secret":  encSecret,
		"secret_warning":     "Store these credentials securely. They will not be shown again.",
	})
}

// GetClient returns a single client by ID.
func (h *Handler) GetClient(c *gin.Context) {
	id := c.Param("id")
	cl, err := h.clientSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}
	shared.SendOK(c, cl)
}

// UpdateClient handles PUT /clients/:id.
func (h *Handler) UpdateClient(c *gin.Context) {
	id := c.Param("id")
	var req client.UpdateClientInput
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.SendValidationError(c, "invalid request body")
		return
	}

	cl, err := h.clientSvc.Update(c.Request.Context(), id, req)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}
	shared.SendOK(c, cl)
}

// DeleteClient handles DELETE /clients/:id.
func (h *Handler) DeleteClient(c *gin.Context) {
	id := c.Param("id")
	if err := h.clientSvc.Delete(c.Request.Context(), id); err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}
	shared.SendNoContent(c)
}

// RotateClientKeys handles POST /clients/:id/rotate.
func (h *Handler) RotateClientKeys(c *gin.Context) {
	id := c.Param("id")
	cl, secret, encKey, encSecret, err := h.clientSvc.RotateKeys(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	// Audit
	adminID, _ := c.Get("admin_id")
	adminIDStr, _ := adminID.(string)
	h.auditSvc.LogKeysRotated(cl.ClientID, adminIDStr, c.ClientIP(), c.GetString("request_id"))

	shared.SendOK(c, gin.H{
		"client":            cl,
		"client_secret":     secret,
		"encryption_key":    encKey,
		"encryption_secret": encSecret,
		"secret_warning":    "Store these credentials securely. They will not be shown again.",
	})
}

// GetClientCredentials returns the stored encryption key and encryption secret for a client.
// Note: the client_secret is one-time only (hashed at rest) and cannot be retrieved.
func (h *Handler) GetClientCredentials(c *gin.Context) {
	id := c.Param("id")
	cl, err := h.clientRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}
	if cl == nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	shared.SendOK(c, gin.H{
		"client_id":         cl.ClientID,
		"encryption_key":    cl.EncryptionKey,
		"encryption_secret": cl.EncryptionSecret,
	})
}

/* ─── Client Provider Key Handlers ──────────────────────── */

// ListClientProviderKeys returns which providers have custom keys set for a client.
func (h *Handler) ListClientProviderKeys(c *gin.Context) {
	id := c.Param("id")
	// Look up the client first to get client_id (the user-facing identifier)
	cl, err := h.clientSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	keys, err := h.providerKeySvc.List(c.Request.Context(), cl.ID)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("list provider keys failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal)
		return
	}

	shared.SendOK(c, keys)
}

// SetClientProviderKey sets a custom API key for a provider on a client.
func (h *Handler) SetClientProviderKey(c *gin.Context) {
	id := c.Param("id")
	providerID := c.Param("provider")

	var req struct {
		APIKey  string   `json:"api_key" binding:"required"`
		BaseURL *string  `json:"base_url"`
		Models  []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.SendValidationError(c, "api_key is required")
		return
	}

	// Look up the client
	cl, err := h.clientSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	input := client.SetClientProviderKeyInput{
		ClientID: cl.ID,
		Provider: providerID,
		APIKey:   req.APIKey,
		BaseURL:  req.BaseURL,
		Models:   req.Models,
	}

	stored, rawKey, err := h.providerKeySvc.Set(c.Request.Context(), input)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("set provider key failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal.WithDetail(err.Error()))
		return
	}

	// Audit
	adminID, _ := c.Get("admin_id")
	adminIDStr, _ := adminID.(string)
	h.auditSvc.Log(&audit.AuditEvent{
		EventType:  audit.EventProviderKeySet,
		Severity:   audit.SeverityInfo,
		ClientID:   &cl.ClientID,
		AdminID:    &adminIDStr,
		ActorType:  audit.ActorAdmin,
		Action:     "set_provider_key",
		Resource:   "client_provider_key",
		ResourceID: stored.ID,
		IPAddress:  c.ClientIP(),
		RequestID:  c.GetString("request_id"),
	})

	shared.SendOK(c, gin.H{
		"provider_key": stored,
		"api_key":      rawKey,
		"warning":      "Store this API key securely. It will not be shown again.",
	})
}

// DeleteClientProviderKey removes a custom provider key from a client.
func (h *Handler) DeleteClientProviderKey(c *gin.Context) {
	id := c.Param("id")
	providerID := c.Param("provider")

	cl, err := h.clientSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	if err := h.providerKeySvc.Delete(c.Request.Context(), cl.ID, providerID); err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	shared.SendNoContent(c)
}

/* ─── Provider Handlers ──────────────────────────────────── */

// ListProviders returns all providers.
func (h *Handler) ListProviders(c *gin.Context) {
	enabledOnly := c.Query("enabled") == "true"
	providers, err := h.providerRepo.List(c.Request.Context(), enabledOnly)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("list providers failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal)
		return
	}
	shared.SendOK(c, providers)
}

// CreateProvider handles POST /providers.
func (h *Handler) CreateProvider(c *gin.Context) {
	var req struct {
		ProviderID string   `json:"provider_id" binding:"required"`
		Name       string   `json:"name" binding:"required"`
		APIKey     string   `json:"api_key" binding:"required"`
		BaseURL    string   `json:"base_url"`
		Models     []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.SendValidationError(c, "provider_id, name, and api_key are required")
		return
	}

	if !provider.IsValidProviderID(req.ProviderID) {
		shared.SendValidationError(c, fmt.Sprintf("invalid provider_id: must be one of %v", provider.ValidProviderIDs))
		return
	}

	input := provider.CreateProviderInput{
		ProviderID: provider.ProviderID(req.ProviderID),
		Name:       req.Name,
		APIKey:     req.APIKey,
		BaseURL:    req.BaseURL,
		Models:     req.Models,
	}

	p, err := h.providerRepo.Create(c.Request.Context(), input)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("create provider failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal.WithDetail(err.Error()))
		return
	}

	_ = h.providerReg.Refresh(c.Request.Context())
	shared.SendCreated(c, p)
}

// GetProvider returns a single provider by ID.
func (h *Handler) GetProvider(c *gin.Context) {
	id := c.Param("id")
	p, err := h.providerRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrInternal)
		return
	}
	if p == nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}
	shared.SendOK(c, p)
}

// UpdateProvider handles PUT /providers/:id.
func (h *Handler) UpdateProvider(c *gin.Context) {
	id := c.Param("id")
	var req provider.UpdateProviderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.SendValidationError(c, "invalid request body")
		return
	}

	p, err := h.providerRepo.Update(c.Request.Context(), id, req)
	if err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	_ = h.providerReg.Refresh(c.Request.Context())
	shared.SendOK(c, p)
}

// DeleteProvider handles DELETE /providers/:id.
func (h *Handler) DeleteProvider(c *gin.Context) {
	id := c.Param("id")
	if err := h.providerRepo.Delete(c.Request.Context(), id); err != nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}

	_ = h.providerReg.Refresh(c.Request.Context())
	shared.SendNoContent(c)
}

/* ─── Audit Log Handlers ─────────────────────────────────── */

// ListAuditLogs returns paginated audit events with optional filters.
func (h *Handler) ListAuditLogs(c *gin.Context) {
	filter := audit.AuditFilter{}
	if s := c.Query("event_type"); s != "" {
		et := audit.EventType(s)
		filter.EventType = &et
	}
	if s := c.Query("severity"); s != "" {
		sev := audit.Severity(s)
		filter.Severity = &sev
	}
	if s := c.Query("client_id"); s != "" {
		filter.ClientID = &s
	}
	filter.Limit = queryInt(c, "limit", 50)
	filter.Offset = queryInt(c, "offset", 0)

	result, err := h.auditRepo.List(c.Request.Context(), filter)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("list audit logs failed", slog.String("error", err.Error()))
		shared.SendError(c, shared.ErrInternal)
		return
	}

	shared.SendPaginated(c, result.Events, result.Total, filter.Offset/filter.Limit+1, filter.Limit)
}

// GetAuditLog returns a single audit event by ID.
func (h *Handler) GetAuditLog(c *gin.Context) {
	id := c.Param("id")
	ev, err := h.auditRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		shared.SendError(c, shared.ErrInternal)
		return
	}
	if ev == nil {
		shared.SendError(c, shared.ErrNotFound)
		return
	}
	shared.SendOK(c, ev)
}

/* ─── Helpers ────────────────────────────────────────────── */

// queryInt parses an integer query parameter with a default fallback.
func queryInt(c *gin.Context, key string, defaultVal int) int {
	v := c.Query(key)
	if v == "" {
		return defaultVal
	}
	i := 0
	_, _ = fmt.Sscanf(v, "%d", &i)
	if i < 0 {
		return defaultVal
	}
	return i
}

// verifyPassword compares a plain-text password against a SHA-256 hash.
func verifyPassword(password, hash string) bool {
	// Simple SHA-256 hash comparison for admin auth.
	// In production, upgrade to bcrypt.
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h[:]) == hash
}


