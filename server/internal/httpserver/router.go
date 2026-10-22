package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/mcpserver"
	"tavily-proxy/server/internal/services"
	"tavily-proxy/server/internal/util"
)

var donateKeyPattern = regexp.MustCompile(`^tvly-[A-Za-z0-9_-]{16,128}$`)

const maxDonationAliasLength = 64

func NewRouter(deps Dependencies) http.Handler {
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.Use(gin.Logger(), gin.Recovery())

	publicFS, _ := fs.Sub(deps.EmbeddedPublic, "public")

	mcpHandler := mcpserver.NewHandler(mcpserver.Dependencies{
		MasterKey:  deps.MasterKeyService,
		AccessKeys: deps.AccessKeyService,
		Proxy:      deps.TavilyProxy,
		Stats:      deps.StatsService,
		Stateless:  deps.Config.MCPStateless,
		SessionTTL: deps.Config.MCPSessionTTL,
	})
	r.Any("/mcp", gin.WrapH(mcpHandler.Streamable))
	r.Any("/sse/*path", gin.WrapH(mcpHandler.SSE))

	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := deps.SettingsService.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	api := r.Group("/api", masterAuthMiddleware(deps.MasterKeyService))
	{
		api.GET("/keys", func(c *gin.Context) { handleListKeys(c, deps.KeyService) })
		api.POST("/keys", func(c *gin.Context) { handleCreateKey(c, deps.KeyService) })
		api.GET("/keys/export", func(c *gin.Context) { handleExportKeys(c, deps.KeyService) })
		api.GET("/keys/:id/raw", func(c *gin.Context) { handleGetKeyRaw(c, deps.KeyService, c.Param("id")) })
		api.GET("/keys/sync", func(c *gin.Context) { handleGetSyncAllKeys(c, deps.QuotaSyncJob) })
		api.POST("/keys/sync", func(c *gin.Context) { handleStartSyncAllKeys(c, deps.QuotaSyncJob) })
		api.DELETE("/keys/invalid", func(c *gin.Context) { handleDeleteInvalidKeys(c, deps.KeyService) })
		api.PUT("/keys/:id", func(c *gin.Context) { handleUpdateKey(c, deps, c.Param("id")) })
		api.DELETE("/keys/:id", func(c *gin.Context) { handleDeleteKey(c, deps.KeyService, c.Param("id")) })

		api.GET("/logs/status-codes", func(c *gin.Context) { handleLogStatusCodes(c, deps.LogService) })
		api.GET("/logs", func(c *gin.Context) { handleListLogs(c, deps.LogService) })
		api.DELETE("/logs", func(c *gin.Context) { handleClearLogs(c, deps.LogService) })
		api.GET("/stats", func(c *gin.Context) { handleStats(c, deps.StatsService) })
		api.GET("/stats/timeseries", func(c *gin.Context) { handleTimeSeries(c, deps.StatsService) })

		api.GET("/settings/master-key", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"master_key": deps.MasterKeyService.Get()})
		})
		api.POST("/settings/master-key/reset", func(c *gin.Context) {
			newKey, err := deps.MasterKeyService.Reset(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "reset_failed"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"master_key": newKey})
		})

		api.GET("/settings/auto-sync", func(c *gin.Context) { handleGetAutoSync(c, deps.SettingsService) })
		api.PUT("/settings/auto-sync", func(c *gin.Context) { handleSetAutoSync(c, deps.SettingsService) })
		api.GET("/settings/log-cleanup", func(c *gin.Context) { handleGetLogCleanup(c, deps.SettingsService) })
		api.PUT("/settings/log-cleanup", func(c *gin.Context) { handleSetLogCleanup(c, deps.SettingsService) })

		api.GET("/settings/cache", func(c *gin.Context) { handleGetCache(c, deps.SettingsService) })
		api.PUT("/settings/cache", func(c *gin.Context) { handleSetCache(c, deps.SettingsService) })
		api.DELETE("/cache", func(c *gin.Context) { handleClearCache(c, deps.CacheService) })
		api.GET("/cache/stats", func(c *gin.Context) { handleCacheStats(c, deps.SettingsService, deps.CacheService) })
		api.GET("/settings/key-pool", func(c *gin.Context) { handleGetKeyPool(c, deps.SettingsService) })
		api.PUT("/settings/key-pool", func(c *gin.Context) { handleSetKeyPool(c, deps.SettingsService) })

		api.GET("/access-keys", func(c *gin.Context) { handleListAccessKeys(c, deps.AccessKeyService) })
		api.POST("/access-keys", func(c *gin.Context) { handleCreateAccessKey(c, deps.AccessKeyService) })
		api.PUT("/access-keys/:id", func(c *gin.Context) { handleUpdateAccessKey(c, deps.AccessKeyService, c.Param("id")) })
		api.DELETE("/access-keys/:id", func(c *gin.Context) { handleDeleteAccessKey(c, deps.AccessKeyService, c.Param("id")) })
		api.GET("/access-keys/:id/raw", func(c *gin.Context) { handleGetAccessKeyRaw(c, deps.AccessKeyService, c.Param("id")) })
	}

	public := r.Group("/api/public")
	{
		public.GET("/stats", func(c *gin.Context) { handlePublicPoolStats(c, deps.KeyService) })
	}

	donateLimiter := NewRateLimiter(5.0/60.0, 5) // 5 req/min per IP
	publicLimited := r.Group("/api/public")
	publicLimited.Use(bodyLimitMiddleware(16 * 1024))
	publicLimited.Use(donateLimiter.Middleware())
	{
		publicLimited.POST("/donate", func(c *gin.Context) {
			handleDonateKeys(c, deps.KeyService, deps.TavilyProxy, deps.AccessKeyService)
		})
		publicLimited.POST("/donate/query", func(c *gin.Context) { handleQueryDonation(c, deps.KeyService) })
	}

	// 捐赠者自助门户：凭访问密钥（捐赠时签发）鉴权，仅能操作自己名下的数据。
	portal := r.Group("/api/portal", accessKeyAuthMiddleware(deps.AccessKeyService))
	{
		portal.GET("/me", func(c *gin.Context) { handlePortalMe(c, deps.AccessKeyService) })
		portal.GET("/overview", func(c *gin.Context) { handlePortalOverview(c, deps.KeyService, deps.LogService) })
		portal.GET("/timeseries", func(c *gin.Context) { handlePortalTimeSeries(c, deps.KeyService, deps.StatsService) })
		portal.GET("/keys", func(c *gin.Context) { handlePortalListKeys(c, deps.KeyService) })
		portal.POST("/keys", func(c *gin.Context) { handlePortalAddKeys(c, deps.KeyService, deps.TavilyProxy) })
		portal.DELETE("/keys/:id", func(c *gin.Context) { handlePortalDeleteKey(c, deps.KeyService, c.Param("id")) })
		portal.POST("/reset-key", func(c *gin.Context) { handlePortalResetKey(c, deps.AccessKeyService) })
	}

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		if isStaticAssetRequest(c.Request) {
			serveEmbeddedFile(c, publicFS, cleanPublicPath(c.Request.URL.Path))
			return
		}

		body, _ := io.ReadAll(c.Request.Body)

		authHeaderToken := parseBearerToken(c.GetHeader("Authorization"))
		apiKeyFromQuery, sanitizedQuery := stripAPIKeyFromRawQuery(c.Request.URL.RawQuery)
		apiKeyFromBody, sanitizedBody := stripAPIKeyFromJSON(body)

		hasCredential := authHeaderToken != "" || apiKeyFromBody != "" || apiKeyFromQuery != ""
		if deps.MasterKeyService.Authenticate(authHeaderToken) || deps.MasterKeyService.Authenticate(apiKeyFromBody) || deps.MasterKeyService.Authenticate(apiKeyFromQuery) {
			handleProxy(c, deps.TavilyProxy, sanitizedBody, sanitizedQuery, 0, "")
			return
		}

		token := firstNonEmpty(authHeaderToken, apiKeyFromBody, apiKeyFromQuery)
		if deps.AccessKeyService != nil {
			if ak, ok := deps.AccessKeyService.Authenticate(token); ok {
				handleProxy(c, deps.TavilyProxy, sanitizedBody, sanitizedQuery, ak.ID, ak.Alias)
				return
			}
		}

		if hasCredential {
			respondUnauthorized(c)
			return
		}

		if c.Request.Method != http.MethodGet {
			respondUnauthorized(c)
			return
		}
		if !acceptsHTML(c.Request) {
			respondUnauthorized(c)
			return
		}
		serveEmbeddedFile(c, publicFS, "index.html")
	})

	return r
}

func masterAuthMiddleware(master *services.MasterKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := parseBearerToken(c.GetHeader("Authorization"))
		if !master.Authenticate(token) {
			respondUnauthorized(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// 捐赠者门户鉴权上下文键：由 accessKeyAuthMiddleware 写入，供门户处理器读取。
const (
	ctxAccessKeyID    = "portal_access_key_id"
	ctxAccessKeyAlias = "portal_access_key_alias"
)

// accessKeyAuthMiddleware 用捐赠者访问密钥鉴权（区别于 masterAuthMiddleware 的 Master Key）。
// AccessKeyService.Authenticate 只匹配活跃 access key 缓存，停用/未知/重置后的旧 token 一律 401。
func accessKeyAuthMiddleware(svc *services.AccessKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		ak, ok := svc.Authenticate(parseBearerToken(c.GetHeader("Authorization")))
		if !ok {
			respondUnauthorized(c)
			c.Abort()
			return
		}
		// 管理员可单独关闭某访问密钥的「门户登录」，但保留其 API/MCP 调用能力。
		// 代理（NoRoute）与 MCP 复用同一个 Authenticate 且不检查此字段，故不受影响。
		if ak.LoginDisabled {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login_disabled"})
			c.Abort()
			return
		}
		c.Set(ctxAccessKeyID, ak.ID)
		c.Set(ctxAccessKeyAlias, ak.Alias)
		c.Next()
	}
}

func respondUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
}

func bodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 || c.Request == nil || c.Request.Body == nil {
			c.Next()
			return
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBytes+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
			c.Abort()
			return
		}
		if int64(len(body)) > maxBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload_too_large"})
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Next()
	}
}

func parseBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func acceptsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}

func isStaticAssetRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	p := r.URL.Path
	if p == "/" {
		return false
	}
	if strings.HasPrefix(p, "/assets/") {
		return true
	}
	if strings.Contains(path.Base(p), ".") {
		return true
	}
	return false
}

func cleanPublicPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "../")
	return p
}

func serveEmbeddedFile(c *gin.Context, publicFS fs.FS, filePath string) {
	f, err := publicFS.Open(filePath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err == nil && stat.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	data, err := io.ReadAll(f)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, contentTypeByExt(filePath), data)
}

func contentTypeByExt(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".ico":
		return "image/x-icon"
	case ".json":
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func handleListKeys(c *gin.Context, keys *services.KeyService) {
	items, err := keys.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	type keyDTO struct {
		ID         uint   `json:"id"`
		KeyMasked  string `json:"key"`
		Alias      string `json:"alias"`
		TotalQuota int    `json:"total_quota"`
		UsedQuota  int    `json:"used_quota"`
		IsActive   bool   `json:"is_active"`
		IsInvalid  bool   `json:"is_invalid"`
		IsDonated  bool   `json:"is_donated"`
		// AccessKeyID 为 0 表示未归属（管理员添加或升级前的无主捐赠）。
		AccessKeyID uint    `json:"access_key_id"`
		LastUsedAt  *string `json:"last_used_at"`
		CreatedAt   string  `json:"created_at"`
	}

	out := make([]keyDTO, 0, len(items))
	for _, k := range items {
		var lastUsed *string
		if k.LastUsedAt != nil {
			v := k.LastUsedAt.Format(time.RFC3339)
			lastUsed = &v
		}
		out = append(out, keyDTO{
			ID:          k.ID,
			KeyMasked:   util.MaskAPIKey(k.Key),
			Alias:       k.Alias,
			TotalQuota:  k.TotalQuota,
			UsedQuota:   k.UsedQuota,
			IsActive:    k.IsActive,
			IsInvalid:   k.IsInvalid,
			IsDonated:   k.IsDonated,
			AccessKeyID: k.AccessKeyID,
			LastUsedAt:  lastUsed,
			CreatedAt:   k.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func handleExportKeys(c *gin.Context, keys *services.KeyService) {
	items, err := keys.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	var buf bytes.Buffer
	var exported int
	for _, k := range items {
		if k.IsInvalid {
			continue
		}
		key := strings.TrimSpace(k.Key)
		if key == "" {
			continue
		}
		buf.WriteString(key)
		buf.WriteByte('\n')
		exported++
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="tavily-keys.txt"`)
	c.Header("X-Exported-Count", strconv.Itoa(exported))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, &buf)
}

func handleCreateKey(c *gin.Context, keys *services.KeyService) {
	var body struct {
		Key        string `json:"key"`
		Alias      string `json:"alias"`
		TotalQuota int    `json:"total_quota"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}

	key := strings.TrimSpace(body.Key)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_key"})
		return
	}
	if !strings.HasPrefix(key, "tvly-") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_key_format"})
		return
	}

	record, _, err := keys.CreateOrUpdate(c.Request.Context(), strings.TrimSpace(body.Key), strings.TrimSpace(body.Alias), body.TotalQuota)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "create_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"item": gin.H{
			"id":          record.ID,
			"key":         util.MaskAPIKey(record.Key),
			"alias":       record.Alias,
			"total_quota": record.TotalQuota,
			"used_quota":  record.UsedQuota,
			"is_active":   record.IsActive,
			"is_invalid":  record.IsInvalid,
			"created_at":  record.CreatedAt.Format(time.RFC3339),
		},
	})
}

func handleDonateKeys(c *gin.Context, keys *services.KeyService, proxy *services.TavilyProxy, accessKeys *services.AccessKeyService) {
	var body struct {
		Keys  []string `json:"keys"`
		Alias string   `json:"alias"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}

	alias := strings.TrimSpace(body.Alias)
	if alias == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_alias"})
		return
	}
	if len(alias) > maxDonationAliasLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias_too_long"})
		return
	}
	if len(body.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_keys"})
		return
	}
	if len(body.Keys) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too_many_keys"})
		return
	}

	ctx := c.Request.Context()

	// 别名重名校验：alias 全局唯一（去空格、忽略大小写）。重名直接拒绝，
	// 避免归属混淆，并堵住"用已存在 alias 再次捐赠会拿回他人访问密钥"的泄露风险。
	if accessKeys != nil {
		taken, err := accessKeys.AliasExists(ctx, alias, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
		if taken {
			c.JSON(http.StatusConflict, gin.H{"error": "alias_taken"})
			return
		}
	}
	if taken, err := keys.UnownedDonationAliasExists(ctx, alias); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	} else if taken {
		c.JSON(http.StatusConflict, gin.H{"error": "alias_taken"})
		return
	}

	batch, err := validateDonations(ctx, body.Keys, alias, keys, proxy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}

	// 校验（走上游）已结束，入库只剩本地写：脱离请求取消，保证访问密钥与 Key 要么都落库要么回滚。
	ak, err := batch.persistForNewDonor(context.WithoutCancel(ctx), keys, accessKeys)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}

	resp := batch.response()
	if ak != nil {
		resp["access_key"] = ak.Key
		resp["access_key_alias"] = ak.Alias
	}
	c.JSON(http.StatusOK, resp)
}

func handleQueryDonation(c *gin.Context, keys *services.KeyService) {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}

	key := strings.TrimSpace(body.Key)
	if !donateKeyPattern.MatchString(key) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_key"})
		return
	}

	record, err := keys.QueryByKey(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if record == nil {
		c.JSON(http.StatusOK, gin.H{"found": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"found":      true,
		"alias":      record.Alias,
		"is_active":  record.IsActive,
		"donated_at": record.CreatedAt.Format(time.RFC3339),
	})
}

func handleGetKeyRaw(c *gin.Context, keys *services.KeyService, idStr string) {
	id, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	key, err := keys.FindByID(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if key == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"key": key.Key})
}

func handleUpdateKey(c *gin.Context, deps Dependencies, idStr string) {
	id, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var body services.KeyUpdate
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}

	if body.AccessKeyID != nil && *body.AccessKeyID != 0 {
		owner, err := deps.AccessKeyService.FindByID(c.Request.Context(), *body.AccessKeyID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
		if owner == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "access_key_not_found"})
			return
		}
	}

	updated, err := deps.KeyService.Update(c.Request.Context(), uint(id), body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "update_failed"})
		return
	}

	if body.SyncUsage {
		if _, err := deps.QuotaSyncService.SyncOne(c.Request.Context(), uint(id)); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if refreshed, err := deps.KeyService.Get(c.Request.Context(), uint(id)); err == nil && refreshed != nil {
			updated = refreshed
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"item": gin.H{
			"id":          updated.ID,
			"key":         util.MaskAPIKey(updated.Key),
			"alias":       updated.Alias,
			"total_quota": updated.TotalQuota,
			"used_quota":  updated.UsedQuota,
			"is_active":   updated.IsActive,
			"is_invalid":  updated.IsInvalid,
		},
	})
}

func handleDeleteInvalidKeys(c *gin.Context, keys *services.KeyService) {
	deleted, err := keys.DeleteInvalid(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func handleGetSyncAllKeys(c *gin.Context, jobs *services.QuotaSyncJobService) {
	c.JSON(http.StatusOK, jobs.Get())
}

func handleStartSyncAllKeys(c *gin.Context, jobs *services.QuotaSyncJobService) {
	var body struct {
		IntervalMs *int `json:"interval_ms"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}

	interval := time.Duration(0)
	if body.IntervalMs != nil {
		interval = time.Duration(*body.IntervalMs) * time.Millisecond
	}

	result, _, err := jobs.Start(interval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sync_failed"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func handleGetAutoSync(c *gin.Context, settings *services.SettingsService) {
	enabled, err := settings.GetBool(c.Request.Context(), services.SettingAutoSyncEnabled, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	interval, err := settings.GetInt(c.Request.Context(), services.SettingAutoSyncIntervalMinutes, 60)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if interval < 1 {
		interval = 1
	}

	concurrency := 1

	requestIntervalSeconds, err := settings.GetInt(c.Request.Context(), services.SettingAutoSyncRequestIntervalSeconds, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if requestIntervalSeconds < 0 {
		requestIntervalSeconds = 0
	}
	if requestIntervalSeconds > 60 {
		requestIntervalSeconds = 60
	}

	lastRun, _ := settings.GetTime(c.Request.Context(), services.SettingAutoSyncLastRunAt)
	lastSuccess, _ := settings.GetTime(c.Request.Context(), services.SettingAutoSyncLastSuccessAt)
	lastErr, _, _ := settings.Get(c.Request.Context(), services.SettingAutoSyncLastError)

	var lastRunStr *string
	if lastRun != nil {
		v := lastRun.Format(time.RFC3339)
		lastRunStr = &v
	}
	var lastSuccessStr *string
	if lastSuccess != nil {
		v := lastSuccess.Format(time.RFC3339)
		lastSuccessStr = &v
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":                  enabled,
		"interval_minutes":         interval,
		"concurrency":              concurrency,
		"request_interval_seconds": requestIntervalSeconds,
		"last_run_at":              lastRunStr,
		"last_success_at":          lastSuccessStr,
		"last_error":               lastErr,
	})
}

func handleSetAutoSync(c *gin.Context, settings *services.SettingsService) {
	var body struct {
		Enabled                *bool `json:"enabled"`
		IntervalMinutes        *int  `json:"interval_minutes"`
		RequestIntervalSeconds *int  `json:"request_interval_seconds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}

	if body.Enabled == nil && body.IntervalMinutes == nil && body.RequestIntervalSeconds == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_fields"})
		return
	}

	if body.IntervalMinutes != nil {
		if *body.IntervalMinutes < 1 || *body.IntervalMinutes > 1440 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_interval_minutes"})
			return
		}
		if err := settings.SetInt(c.Request.Context(), services.SettingAutoSyncIntervalMinutes, *body.IntervalMinutes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}
	if body.RequestIntervalSeconds != nil {
		if *body.RequestIntervalSeconds < 0 || *body.RequestIntervalSeconds > 60 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_interval_seconds"})
			return
		}
		if err := settings.SetInt(c.Request.Context(), services.SettingAutoSyncRequestIntervalSeconds, *body.RequestIntervalSeconds); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}
	if body.Enabled != nil {
		if err := settings.SetBool(c.Request.Context(), services.SettingAutoSyncEnabled, *body.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}

	c.Status(http.StatusNoContent)
}

func handleGetLogCleanup(c *gin.Context, settings *services.SettingsService) {
	retentionDays, err := settings.GetInt(c.Request.Context(), services.SettingLogRetentionDays, 30)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if retentionDays < 0 {
		retentionDays = 0
	}

	loggingEnabled, err := settings.GetBool(c.Request.Context(), services.SettingRequestLoggingEnabled, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	lastRun, _ := settings.GetTime(c.Request.Context(), services.SettingLogCleanupLastRunAt)
	lastErr, _, _ := settings.Get(c.Request.Context(), services.SettingLogCleanupLastError)

	var lastRunStr *string
	if lastRun != nil {
		v := lastRun.Format(time.RFC3339)
		lastRunStr = &v
	}

	c.JSON(http.StatusOK, gin.H{
		"logging_enabled": loggingEnabled,
		"retention_days":  retentionDays,
		"last_run_at":     lastRunStr,
		"last_error":      lastErr,
	})
}

func handleSetLogCleanup(c *gin.Context, settings *services.SettingsService) {
	var body struct {
		LoggingEnabled *bool `json:"logging_enabled"`
		RetentionDays  *int  `json:"retention_days"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	if body.RetentionDays == nil && body.LoggingEnabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_fields"})
		return
	}

	if body.LoggingEnabled != nil {
		if err := settings.SetBool(c.Request.Context(), services.SettingRequestLoggingEnabled, *body.LoggingEnabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}

	if body.RetentionDays != nil {
		if *body.RetentionDays < 0 || *body.RetentionDays > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_retention_days"})
			return
		}
		if err := settings.SetInt(c.Request.Context(), services.SettingLogRetentionDays, *body.RetentionDays); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func handleGetKeyPool(c *gin.Context, settings *services.SettingsService) {
	strategy, _, err := settings.Get(c.Request.Context(), services.SettingKeyPoolStrategy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if strategy == "" {
		strategy = services.StrategyBestRemaining
	}
	c.JSON(http.StatusOK, gin.H{"strategy": strategy})
}

func handleSetKeyPool(c *gin.Context, settings *services.SettingsService) {
	var body struct {
		Strategy *string `json:"strategy"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	if body.Strategy == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_fields"})
		return
	}

	valid := map[string]bool{
		services.StrategyBestRemaining:  true,
		services.StrategyRoundRobin:     true,
		services.StrategySequentialFill: true,
	}
	if !valid[*body.Strategy] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_strategy"})
		return
	}

	if err := settings.Set(c.Request.Context(), services.SettingKeyPoolStrategy, *body.Strategy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func handleDeleteKey(c *gin.Context, keys *services.KeyService, idStr string) {
	id, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	if err := keys.Delete(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleListLogs(c *gin.Context, logs *services.LogService) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("page_size"))

	var statusCode *int
	if v := c.Query("status_code"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 || parsed > 999 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_status_code"})
			return
		}
		statusCode = &parsed
	}

	var accessKeyID *uint
	if v := c.Query("access_key_id"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_access_key_id"})
			return
		}
		id := uint(parsed)
		accessKeyID = &id
	}

	out, err := logs.List(c.Request.Context(), page, size, statusCode, accessKeyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, out)
}

func handleClearLogs(c *gin.Context, logs *services.LogService) {
	deleted, err := logs.DeleteAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func handleLogStatusCodes(c *gin.Context, logs *services.LogService) {
	out, err := logs.StatusCodeCounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, out)
}

func handleStats(c *gin.Context, stats *services.StatsService) {
	out, err := stats.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, out)
}

func handlePublicPoolStats(c *gin.Context, keys *services.KeyService) {
	out, err := keys.PoolQuotaSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, out)
}

func handleTimeSeries(c *gin.Context, stats *services.StatsService) {
	granularity := c.Query("granularity")
	out, err := stats.TimeSeries(c.Request.Context(), granularity)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_granularity"})
		return
	}
	c.JSON(http.StatusOK, out)
}

func handleProxy(c *gin.Context, proxy *services.TavilyProxy, body []byte, rawQuery string, accessKeyID uint, accessKeyAlias string) {
	resp, err := proxy.Do(c.Request.Context(), services.ProxyRequest{
		Method:         c.Request.Method,
		Path:           c.Request.URL.Path,
		RawQuery:       rawQuery,
		Headers:        c.Request.Header.Clone(),
		Body:           body,
		ClientIP:       c.ClientIP(),
		ContentType:    c.GetHeader("Content-Type"),
		AccessKeyID:    accessKeyID,
		AccessKeyAlias: accessKeyAlias,
	})
	if err != nil {
		if errors.Is(err, services.ErrNoAvailableKeys) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "no_available_keys",
				"message": "No active Tavily API keys with remaining quota.",
			})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream_error"})
		return
	}

	for k, vv := range resp.Headers {
		if isHopByHopHeader(k) || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Header("X-Proxy-Request-ID", resp.ProxyRequestID)
	if resp.TavilyRequestID != "" {
		c.Header("X-Tavily-Request-ID", resp.TavilyRequestID)
	}

	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, bytes.NewReader(resp.Body))
}

func isHopByHopHeader(k string) bool {
	switch strings.ToLower(k) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func stripAPIKeyFromJSON(body []byte) (apiKey string, sanitized []byte) {
	if len(body) == 0 {
		return "", body
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", body
	}

	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return "", body
	}

	changed := false
	if v, ok := m["api_key"]; ok {
		if s, ok := v.(string); ok {
			apiKey = strings.TrimSpace(s)
		}
		delete(m, "api_key")
		changed = true
	}
	if v, ok := m["apiKey"]; ok {
		if apiKey == "" {
			if s, ok := v.(string); ok {
				apiKey = strings.TrimSpace(s)
			}
		}
		delete(m, "apiKey")
		changed = true
	}

	if !changed {
		return "", body
	}

	out, err := json.Marshal(m)
	if err != nil {
		return apiKey, body
	}
	return apiKey, out
}

func stripAPIKeyFromRawQuery(rawQuery string) (apiKey string, sanitized string) {
	if rawQuery == "" {
		return "", rawQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", rawQuery
	}

	apiKey = strings.TrimSpace(values.Get("api_key"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(values.Get("apiKey"))
	}
	values.Del("api_key")
	values.Del("apiKey")

	return apiKey, values.Encode()
}

func handleGetCache(c *gin.Context, settings *services.SettingsService) {
	enabled, err := settings.GetBool(c.Request.Context(), services.SettingCacheEnabled, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	ttl, err := settings.GetInt(c.Request.Context(), services.SettingCacheTTLSeconds, 43200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":     enabled,
		"ttl_seconds": ttl,
	})
}

func handleSetCache(c *gin.Context, settings *services.SettingsService) {
	var body struct {
		Enabled    *bool `json:"enabled"`
		TTLSeconds *int  `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	if body.Enabled == nil && body.TTLSeconds == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_fields"})
		return
	}
	if body.TTLSeconds != nil {
		if *body.TTLSeconds < 60 || *body.TTLSeconds > 604800 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ttl_seconds"})
			return
		}
		if err := settings.SetInt(c.Request.Context(), services.SettingCacheTTLSeconds, *body.TTLSeconds); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}
	if body.Enabled != nil {
		if err := settings.SetBool(c.Request.Context(), services.SettingCacheEnabled, *body.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func handleClearCache(c *gin.Context, cache *services.CacheService) {
	deleted, err := cache.ClearAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func handleCacheStats(c *gin.Context, settings *services.SettingsService, cache *services.CacheService) {
	stats, err := cache.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	enabled, _ := settings.GetBool(c.Request.Context(), services.SettingCacheEnabled, false)
	stats.Enabled = enabled
	c.JSON(http.StatusOK, stats)
}

func parseUintParam(v string) (uint64, error) {
	return strconv.ParseUint(v, 10, 64)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func handleListAccessKeys(c *gin.Context, svc *services.AccessKeyService) {
	items, err := svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	type dto struct {
		ID            uint    `json:"id"`
		KeyMasked     string  `json:"key"`
		Alias         string  `json:"alias"`
		IsActive      bool    `json:"is_active"`
		LoginDisabled bool    `json:"login_disabled"`
		LastUsedAt    *string `json:"last_used_at"`
		CreatedAt     string  `json:"created_at"`
	}
	out := make([]dto, 0, len(items))
	for _, k := range items {
		var lastUsed *string
		if k.LastUsedAt != nil {
			v := k.LastUsedAt.Format(time.RFC3339)
			lastUsed = &v
		}
		out = append(out, dto{
			ID:            k.ID,
			KeyMasked:     util.MaskAPIKey(k.Key),
			Alias:         k.Alias,
			IsActive:      k.IsActive,
			LoginDisabled: k.LoginDisabled,
			LastUsedAt:    lastUsed,
			CreatedAt:     k.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func handleCreateAccessKey(c *gin.Context, svc *services.AccessKeyService) {
	var body struct {
		Alias string `json:"alias"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	if strings.TrimSpace(body.Alias) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_alias"})
		return
	}
	if taken, err := svc.AliasExists(c.Request.Context(), body.Alias, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	} else if taken {
		c.JSON(http.StatusConflict, gin.H{"error": "alias_taken"})
		return
	}
	created, err := svc.Create(c.Request.Context(), strings.TrimSpace(body.Alias))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"item": gin.H{
			"id":         created.ID,
			"key":        created.Key,
			"alias":      created.Alias,
			"is_active":  created.IsActive,
			"created_at": created.CreatedAt.Format(time.RFC3339),
		},
	})
}

func handleUpdateAccessKey(c *gin.Context, svc *services.AccessKeyService, idStr string) {
	id, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	var body services.AccessKeyUpdate
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	// 仅在 alias 相对当前值发生变化时才查重：编辑弹窗每次都会带上原 alias，
	// 否则历史重名的访问密钥连启停/门户登录开关都改不了。
	if body.Alias != nil {
		current, err := svc.FindByID(c.Request.Context(), uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
		if current == nil || current.Alias != *body.Alias {
			if taken, err := svc.AliasExists(c.Request.Context(), *body.Alias, uint(id)); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
				return
			} else if taken {
				c.JSON(http.StatusConflict, gin.H{"error": "alias_taken"})
				return
			}
		}
	}
	updated, err := svc.Update(c.Request.Context(), uint(id), body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "update_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"item": gin.H{
			"id":             updated.ID,
			"key":            util.MaskAPIKey(updated.Key),
			"alias":          updated.Alias,
			"is_active":      updated.IsActive,
			"login_disabled": updated.LoginDisabled,
		},
	})
}

func handleDeleteAccessKey(c *gin.Context, svc *services.AccessKeyService, idStr string) {
	id, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	if err := svc.Delete(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleGetAccessKeyRaw(c *gin.Context, svc *services.AccessKeyService, idStr string) {
	id, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	raw, err := svc.GetRawKey(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": raw})
}

// ---- 捐赠者门户处理器（鉴权由 accessKeyAuthMiddleware 完成）----

// handlePortalMe 返回当前捐赠者的基本信息，同时兼作前端登录探针。
func handlePortalMe(c *gin.Context, svc *services.AccessKeyService) {
	ak, err := svc.FindByID(c.Request.Context(), c.GetUint(ctxAccessKeyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if ak == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	var lastUsed *string
	if ak.LastUsedAt != nil {
		v := ak.LastUsedAt.Format(time.RFC3339)
		lastUsed = &v
	}
	c.JSON(http.StatusOK, gin.H{
		"alias":        ak.Alias,
		"key":          util.MaskAPIKey(ak.Key),
		"is_active":    ak.IsActive,
		"last_used_at": lastUsed,
		"created_at":   ak.CreatedAt.Format(time.RFC3339),
	})
}

// handlePortalOverview 汇总该捐赠者名下 Key 的额度用量，以及他本人发起的请求数（今日/累计）。
func handlePortalOverview(c *gin.Context, keys *services.KeyService, logs *services.LogService) {
	ctx := c.Request.Context()
	id := c.GetUint(ctxAccessKeyID)

	summary, err := keys.DonatedSummaryByAccessKey(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayRequests, err := logs.CountByAccessKey(ctx, id, &startOfDay)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	totalRequests, err := logs.CountByAccessKey(ctx, id, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_quota":    summary.TotalQuota,
		"used_quota":     summary.TotalUsed,
		"remaining":      summary.TotalRemaining,
		"key_count":      summary.ActiveKeyCount,
		"today_requests": todayRequests,
		"total_requests": totalRequests,
	})
}

// handlePortalTimeSeries 返回该捐赠者的双系列时序（名下 Key 被调用 / 本人发起请求）。
func handlePortalTimeSeries(c *gin.Context, keys *services.KeyService, stats *services.StatsService) {
	ctx := c.Request.Context()
	id := c.GetUint(ctxAccessKeyID)

	owned, err := keys.ListByAccessKey(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	ids := make([]uint, 0, len(owned))
	for _, k := range owned {
		ids = append(ids, k.ID)
	}

	out, err := stats.PortalTimeSeries(ctx, c.Query("granularity"), id, ids)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_granularity"})
		return
	}
	c.JSON(http.StatusOK, out)
}

// handlePortalListKeys 列出该捐赠者名下的全部 Key（掩码）。
func handlePortalListKeys(c *gin.Context, keys *services.KeyService) {
	items, err := keys.ListByAccessKey(c.Request.Context(), c.GetUint(ctxAccessKeyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	type keyDTO struct {
		ID         uint    `json:"id"`
		KeyMasked  string  `json:"key"`
		Alias      string  `json:"alias"`
		TotalQuota int     `json:"total_quota"`
		UsedQuota  int     `json:"used_quota"`
		IsActive   bool    `json:"is_active"`
		IsInvalid  bool    `json:"is_invalid"`
		IsDonated  bool    `json:"is_donated"`
		LastUsedAt *string `json:"last_used_at"`
		CreatedAt  string  `json:"created_at"`
	}
	out := make([]keyDTO, 0, len(items))
	for _, k := range items {
		var lastUsed *string
		if k.LastUsedAt != nil {
			v := k.LastUsedAt.Format(time.RFC3339)
			lastUsed = &v
		}
		out = append(out, keyDTO{
			ID:         k.ID,
			KeyMasked:  util.MaskAPIKey(k.Key),
			Alias:      k.Alias,
			TotalQuota: k.TotalQuota,
			UsedQuota:  k.UsedQuota,
			IsActive:   k.IsActive,
			IsInvalid:  k.IsInvalid,
			IsDonated:  k.IsDonated,
			LastUsedAt: lastUsed,
			CreatedAt:  k.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// handlePortalAddKeys 让捐赠者把更多 Tavily Key 加到自己名下（复用捐赠校验/激活流程）。
func handlePortalAddKeys(c *gin.Context, keys *services.KeyService, proxy *services.TavilyProxy) {
	id := c.GetUint(ctxAccessKeyID)
	alias := c.GetString(ctxAccessKeyAlias)

	var body struct {
		Keys []string `json:"keys"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	if len(body.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_keys"})
		return
	}
	if len(body.Keys) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too_many_keys"})
		return
	}

	ctx := c.Request.Context()
	batch, err := validateDonations(ctx, body.Keys, alias, keys, proxy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}
	if _, err := batch.persist(context.WithoutCancel(ctx), keys, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}
	c.JSON(http.StatusOK, batch.response())
}

// handlePortalDeleteKey 删除捐赠者名下的某个 Key；非己 Key 返回 403。
func handlePortalDeleteKey(c *gin.Context, keys *services.KeyService, idStr string) {
	keyID, err := parseUintParam(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	owned, err := keys.FindOwnedByID(c.Request.Context(), c.GetUint(ctxAccessKeyID), uint(keyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if owned == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := keys.Delete(c.Request.Context(), uint(keyID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handlePortalResetKey 轮换当前捐赠者的访问密钥，旧 token 立即失效，返回新 token（仅此一次可见）。
func handlePortalResetKey(c *gin.Context, svc *services.AccessKeyService) {
	token, err := svc.ResetKey(c.Request.Context(), c.GetUint(ctxAccessKeyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reset_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_key": token})
}
