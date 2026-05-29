package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"tavily-proxy/server/internal/models"
)

type TavilyProxy struct {
	baseURL string
	client  *http.Client

	settings *SettingsService
	cache    *CacheService
	keys     *KeyService
	logs     *LogService
	stats    *StatsService
	logger   *slog.Logger
}

type ProxyRequest struct {
	Method         string
	Path           string
	RawQuery       string
	Headers        http.Header
	Body           []byte
	ClientIP       string
	ContentType    string
	AccessKeyID    uint
	AccessKeyAlias string
}

type ProxyResponse struct {
	StatusCode      int
	Headers         http.Header
	Body            []byte
	ProxyRequestID  string
	TavilyRequestID string
}

func NewTavilyProxy(baseURL string, timeout time.Duration, keys *KeyService, logs *LogService, stats *StatsService, logger *slog.Logger) *TavilyProxy {
	return &TavilyProxy{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
		keys:     keys,
		logs:     logs,
		stats:    stats,
		logger:   logger,
		settings: nil,
	}
}

var ErrNoAvailableKeys = errors.New("no available keys")

func (p *TavilyProxy) WithSettings(settings *SettingsService) *TavilyProxy {
	p.settings = settings
	return p
}

func (p *TavilyProxy) WithCache(cache *CacheService) *TavilyProxy {
	p.cache = cache
	return p
}

func (p *TavilyProxy) isRequestLoggingEnabled(ctx context.Context) bool {
	if p.settings == nil {
		return true
	}
	enabled, err := p.settings.GetBool(ctx, SettingRequestLoggingEnabled, true)
	if err != nil {
		return true
	}
	return enabled
}

func (p *TavilyProxy) Do(ctx context.Context, req ProxyRequest) (ProxyResponse, error) {
	const maxLogBytes = 32 * 1024

	proxyReqID := uuid.NewString()
	startTime := time.Now()

	p.logger.Info("proxy request started",
		"request_id", proxyReqID,
		"method", req.Method,
		"path", req.Path,
		"client_ip", req.ClientIP,
	)

	loggingEnabled := p.logs != nil && p.isRequestLoggingEnabled(ctx)
	captureBodies := strings.EqualFold(req.Method, http.MethodPost) && req.Path == "/search"
	requestBody, requestTruncated := "", false
	if loggingEnabled && captureBodies && len(req.Body) > 0 {
		requestBody, requestTruncated = truncateForLog(req.Body, maxLogBytes)
	}

	// Pool usage summary request
	if isPoolUsageSummaryRequest(req) {
		return p.poolUsageSummaryResponse(ctx, req, proxyReqID, loggingEnabled)
	}

	// Cache lookup for POST /search
	noCache := false
	if captureBodies && req.RawQuery != "" {
		if vals, err := url.ParseQuery(req.RawQuery); err == nil {
			noCache = strings.EqualFold(vals.Get("no_cache"), "true")
			vals.Del("no_cache")
			req.RawQuery = vals.Encode()
		}
	}

	if captureBodies && !noCache && p.cache != nil && p.isCacheEnabled(ctx) {
		cacheKey, query := p.cache.BuildCacheKey(req.Body)
		if entry, hit, err := p.cache.Lookup(ctx, cacheKey); err == nil && hit {
			createdAt := time.Now()
			if loggingEnabled {
				responseBody, responseTruncated := truncateForLog([]byte(entry.ResponseBody), maxLogBytes)
				_ = p.logs.Create(ctx, &models.RequestLog{
					RequestID:         proxyReqID,
					KeyUsed:           0,
					KeyAlias:          "",
					Endpoint:          req.Path,
					StatusCode:        entry.StatusCode,
					LatencyMs:         0,
					RequestBody:       requestBody,
					RequestTruncated:  requestTruncated,
					ResponseBody:      responseBody,
					ResponseTruncated: responseTruncated,
					CacheHit:          true,
					ClientIP:          req.ClientIP,
					CreatedAt:         createdAt,
				})
			}
			if p.stats != nil {
				_ = p.stats.RecordRequest(ctx, req.Path, createdAt)
			}
			p.logger.Info("cache hit", "query", query, "cache_key", cacheKey[:12])
			return ProxyResponse{
				StatusCode:     entry.StatusCode,
				Headers:        http.Header{"Content-Type": {"application/json"}},
				Body:           []byte(entry.ResponseBody),
				ProxyRequestID: proxyReqID,
			}, nil
		}
	}

	candidates, err := p.keys.Candidates(ctx)
	if err != nil {
		return ProxyResponse{}, err
	}

	if len(candidates) == 0 {
		p.logger.Warn("no available keys",
			"request_id", proxyReqID,
			"path", req.Path,
		)
		if captureBodies {
			createdAt := time.Now()
			if loggingEnabled {
				_ = p.logs.Create(ctx, &models.RequestLog{
					RequestID:         proxyReqID,
					KeyUsed:           0,
					KeyAlias:          "",
					Endpoint:          req.Path,
					StatusCode:        http.StatusServiceUnavailable,
					LatencyMs:         0,
					RequestBody:       requestBody,
					RequestTruncated:  requestTruncated,
					ResponseBody:      `{"error":"no_available_keys","message":"No active Tavily API keys with remaining quota."}`,
					ResponseTruncated: false,
					ClientIP:          req.ClientIP,
					AccessKeyID:       req.AccessKeyID,
					AccessKeyAlias:    req.AccessKeyAlias,
					CreatedAt:         createdAt,
				})
			}
			if p.stats != nil {
				_ = p.stats.RecordRequest(ctx, req.Path, createdAt)
			}
		}
		return ProxyResponse{}, ErrNoAvailableKeys
	}

	var lastErr error
	var lastRateLimitedResp ProxyResponse
	var hasRateLimitedResp bool
	var lastRateLimitedKeyID uint
	var lastRateLimitedKeyAlias string
	var lastRateLimitedLatency int64
	for _, key := range candidates {
		resp, status, latencyMs, tavilyReqID, err := p.tryKey(ctx, key.ID, key.Key, req, proxyReqID)

		if err != nil {
			p.logger.Warn("upstream request failed",
				"request_id", proxyReqID,
				"key_id", key.ID,
				"key_alias", key.Alias,
				"err", err,
			)
			lastErr = err
			continue
		}

		switch status {
		case http.StatusUnauthorized:
			p.logger.Warn("key marked invalid",
				"request_id", proxyReqID,
				"key_id", key.ID,
				"key_alias", key.Alias,
			)
			_ = p.keys.MarkInvalid(ctx, key.ID)
			continue
		case http.StatusTooManyRequests:
			p.logger.Warn("key rate limited",
				"request_id", proxyReqID,
				"key_id", key.ID,
				"key_alias", key.Alias,
				"status", status,
			)
			resp.ProxyRequestID = proxyReqID
			resp.TavilyRequestID = tavilyReqID
			lastRateLimitedResp = resp
			hasRateLimitedResp = true
			lastRateLimitedKeyID = key.ID
			lastRateLimitedKeyAlias = key.Alias
			lastRateLimitedLatency = latencyMs
			continue
		case http.StatusForbidden, 432, 433:
			p.logger.Warn("key forbidden or quota exhausted",
				"request_id", proxyReqID,
				"key_id", key.ID,
				"key_alias", key.Alias,
				"status", status,
			)
			_ = p.keys.MarkExhausted(ctx, key.ID)
			continue
		}

		if status == http.StatusOK && !strings.EqualFold(req.Method, http.MethodGet) {
			credits := resolveCredits(resp.Body, req)
			if err := p.keys.IncrementUsed(ctx, key.ID, credits); err != nil {
				p.logger.Warn("increment used quota failed",
					"request_id", proxyReqID, "key_id", key.ID, "credits", credits, "err", err)
			}
		}

		createdAt := time.Now()
		if loggingEnabled {
			if captureBodies {
				responseBody, responseTruncated := truncateForLog(resp.Body, maxLogBytes)
				_ = p.logs.Create(ctx, &models.RequestLog{
					RequestID:         proxyReqID,
					KeyUsed:           key.ID,
					KeyAlias:          key.Alias,
					Endpoint:          req.Path,
					StatusCode:        status,
					LatencyMs:         latencyMs,
					RequestBody:       requestBody,
					RequestTruncated:  requestTruncated,
					ResponseBody:      responseBody,
					ResponseTruncated: responseTruncated,
					ClientIP:          req.ClientIP,
					AccessKeyID:       req.AccessKeyID,
					AccessKeyAlias:    req.AccessKeyAlias,
					CreatedAt:         createdAt,
				})
			} else {
				_ = p.logs.Create(ctx, &models.RequestLog{
					RequestID:      proxyReqID,
					KeyUsed:        key.ID,
					KeyAlias:       key.Alias,
					Endpoint:       req.Path,
					StatusCode:     status,
					LatencyMs:      latencyMs,
					ClientIP:       req.ClientIP,
					AccessKeyID:    req.AccessKeyID,
					AccessKeyAlias: req.AccessKeyAlias,
					CreatedAt:      createdAt,
				})
			}
		}
		if p.stats != nil {
			_ = p.stats.RecordRequest(ctx, req.Path, createdAt)
		}

		// Store in cache after successful upstream response
		if captureBodies && status == http.StatusOK && p.cache != nil && p.isCacheEnabled(ctx) {
			cacheKey, query := p.cache.BuildCacheKey(req.Body)
			ttl := p.getCacheTTL(ctx)
			if err := p.cache.Store(ctx, cacheKey, query, string(req.Body), string(resp.Body), status, ttl); err != nil {
				p.logger.Warn("cache store failed", "err", err)
			}
		}

		resp.ProxyRequestID = proxyReqID
		resp.TavilyRequestID = tavilyReqID
		p.logger.Info("proxy request completed",
			"request_id", proxyReqID,
			"key_id", key.ID,
			"status", status,
			"latency_ms", time.Since(startTime).Milliseconds(),
		)
		return resp, nil
	}

	if hasRateLimitedResp {
		createdAt := time.Now()
		if loggingEnabled {
			if captureBodies {
				responseBody, responseTruncated := truncateForLog(lastRateLimitedResp.Body, maxLogBytes)
				_ = p.logs.Create(ctx, &models.RequestLog{
					RequestID:         proxyReqID,
					KeyUsed:           lastRateLimitedKeyID,
					KeyAlias:          lastRateLimitedKeyAlias,
					Endpoint:          req.Path,
					StatusCode:        lastRateLimitedResp.StatusCode,
					LatencyMs:         lastRateLimitedLatency,
					RequestBody:       requestBody,
					RequestTruncated:  requestTruncated,
					ResponseBody:      responseBody,
					ResponseTruncated: responseTruncated,
					ClientIP:          req.ClientIP,
					CreatedAt:         createdAt,
				})
			} else {
				_ = p.logs.Create(ctx, &models.RequestLog{
					RequestID:  proxyReqID,
					KeyUsed:    lastRateLimitedKeyID,
					KeyAlias:   lastRateLimitedKeyAlias,
					Endpoint:   req.Path,
					StatusCode: lastRateLimitedResp.StatusCode,
					LatencyMs:  lastRateLimitedLatency,
					ClientIP:   req.ClientIP,
					CreatedAt:  createdAt,
				})
			}
		}
		if p.stats != nil {
			_ = p.stats.RecordRequest(ctx, req.Path, createdAt)
		}
		p.logger.Warn("proxy request completed with rate limit",
			"request_id", proxyReqID,
			"key_id", lastRateLimitedKeyID,
			"status", lastRateLimitedResp.StatusCode,
			"latency_ms", time.Since(startTime).Milliseconds(),
		)
		return lastRateLimitedResp, nil
	}

	if captureBodies && lastErr != nil {
		p.logger.Error("all keys exhausted",
			"request_id", proxyReqID,
			"path", req.Path,
			"last_err", lastErr,
		)
		createdAt := time.Now()
		if loggingEnabled {
			_ = p.logs.Create(ctx, &models.RequestLog{
				RequestID:         proxyReqID,
				KeyUsed:           0,
				KeyAlias:          "",
				Endpoint:          req.Path,
				StatusCode:        http.StatusBadGateway,
				LatencyMs:         0,
				RequestBody:       requestBody,
				RequestTruncated:  requestTruncated,
				ResponseBody:      lastErr.Error(),
				ResponseTruncated: false,
				ClientIP:          req.ClientIP,
				AccessKeyID:       req.AccessKeyID,
				AccessKeyAlias:    req.AccessKeyAlias,
				CreatedAt:         createdAt,
			})
		}
		if p.stats != nil {
			_ = p.stats.RecordRequest(ctx, req.Path, createdAt)
		}
	} else if lastErr == nil {
		p.logger.Error("all keys exhausted",
			"request_id", proxyReqID,
			"path", req.Path,
		)
	}

	return ProxyResponse{}, ErrNoAvailableKeys
}

func isPoolUsageSummaryRequest(req ProxyRequest) bool {
	return strings.EqualFold(req.Method, http.MethodGet) && req.Path == "/usage"
}

func (p *TavilyProxy) poolUsageSummaryResponse(ctx context.Context, req ProxyRequest, proxyReqID string, loggingEnabled bool) (ProxyResponse, error) {
	summary, err := p.keys.PoolQuotaSummary(ctx)
	if err != nil {
		return ProxyResponse{}, err
	}

	body, err := json.Marshal(map[string]any{
		"key": map[string]any{
			"usage": summary.TotalUsed,
			"limit": summary.TotalQuota,
		},
		"account": map[string]any{
			"plan_usage": summary.TotalUsed,
			"plan_limit": summary.TotalQuota,
		},
	})
	if err != nil {
		return ProxyResponse{}, err
	}

	createdAt := time.Now()
	if loggingEnabled {
		_ = p.logs.Create(ctx, &models.RequestLog{
			RequestID:      proxyReqID,
			KeyUsed:        0,
			KeyAlias:       "pool",
			Endpoint:       req.Path,
			StatusCode:     http.StatusOK,
			LatencyMs:      0,
			ClientIP:       req.ClientIP,
			AccessKeyID:    req.AccessKeyID,
			AccessKeyAlias: req.AccessKeyAlias,
			CreatedAt:      createdAt,
		})
	}
	if p.stats != nil {
		_ = p.stats.RecordRequest(ctx, req.Path, createdAt)
	}

	return ProxyResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": []string{"application/json; charset=utf-8"},
		},
		Body:           body,
		ProxyRequestID: proxyReqID,
	}, nil
}

func truncateForLog(data []byte, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(data) <= maxBytes {
		return string(data), false
	}
	return string(data[:maxBytes]), true
}

func (p *TavilyProxy) tryKey(ctx context.Context, keyID uint, tavilyKey string, req ProxyRequest, proxyReqID string) (ProxyResponse, int, int64, string, error) {
	targetURL := p.baseURL + req.Path
	if req.RawQuery != "" {
		targetURL += "?" + req.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, bytes.NewReader(req.Body))
	if err != nil {
		return ProxyResponse{}, 0, 0, "", err
	}

	copyHeaders(upstreamReq.Header, req.Headers)
	upstreamReq.Header.Del("Authorization")
	upstreamReq.Header.Set("Authorization", "Bearer "+tavilyKey)
	if req.ContentType != "" && upstreamReq.Header.Get("Content-Type") == "" {
		upstreamReq.Header.Set("Content-Type", req.ContentType)
	}
	upstreamReq.Header.Set("X-Proxy-Request-Id", proxyReqID)

	p.logger.Debug("sending upstream request",
		"request_id", proxyReqID,
		"method", req.Method,
		"url", targetURL,
		"key_id", keyID,
	)

	start := time.Now()
	upstreamResp, err := p.client.Do(upstreamReq)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		p.logger.Warn("upstream call error",
			"request_id", proxyReqID,
			"url", targetURL,
			"key_id", keyID,
			"latency_ms", latencyMs,
			"err", err,
		)
		return ProxyResponse{}, 0, latencyMs, "", err
	}
	defer upstreamResp.Body.Close()

	p.logger.Debug("upstream response received",
		"request_id", proxyReqID,
		"status", upstreamResp.StatusCode,
		"latency_ms", latencyMs,
		"key_id", keyID,
	)

	body, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		p.logger.Warn("upstream response read error",
			"request_id", proxyReqID,
			"key_id", keyID,
			"status", upstreamResp.StatusCode,
			"err", err,
		)
		return ProxyResponse{}, upstreamResp.StatusCode, latencyMs, "", err
	}

	requestID := extractRequestID(body)

	return ProxyResponse{
		StatusCode:     upstreamResp.StatusCode,
		Headers:        upstreamResp.Header.Clone(),
		Body:           body,
		ProxyRequestID: proxyReqID,
	}, upstreamResp.StatusCode, latencyMs, requestID, nil
}

func copyHeaders(dst http.Header, src http.Header) {
	for k, vv := range src {
		switch strings.ToLower(k) {
		case "host", "content-length":
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func extractRequestID(body []byte) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if v, ok := m["request_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := m["requestId"].(string); ok && v != "" {
		return v
	}
	return ""
}

// resolveCredits determines the credit cost of a successful request.
// It first attempts to read the actual credit consumption from the upstream
// response body (usage.credits). If unavailable, it falls back to estimating
// based on the request path and parameters.
func resolveCredits(respBody []byte, req ProxyRequest) int {
	if credits := parseCreditsFromResponse(respBody); credits > 0 {
		return credits
	}
	return estimateCreditsFromRequest(req)
}

// parseCreditsFromResponse tries to extract the credits field from an
// upstream Tavily response body: {"usage": {"credits": N}}.
func parseCreditsFromResponse(body []byte) int {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return 0
	}
	usage, ok := m["usage"].(map[string]any)
	if !ok {
		return 0
	}
	credits, ok := usage["credits"]
	if !ok {
		return 0
	}
	// JSON numbers unmarshal as float64
	switch v := credits.(type) {
	case float64:
		return int(math.Ceil(v))
	case int:
		return v
	}
	return 0
}

// estimateCreditsFromRequest provides a conservative credit estimate
// based on endpoint path and request body parameters.
//
// Official credit costs (docs.tavily.com/documentation/api-credits):
//   - /search: basic/fast/ultra-fast = 1 credit, advanced = 2 credits
//   - /extract: basic = 1 credit per 5 URLs, advanced = 2 credits per 5 URLs
//   - /crawl: variable (mapping + extraction), cannot predict from request
//   - /map: 1 credit per 10 pages (2 with instructions), cannot predict
func estimateCreditsFromRequest(req ProxyRequest) int {
	var m map[string]any
	_ = json.Unmarshal(req.Body, &m)
	if m == nil {
		m = map[string]any{}
	}

	switch {
	case strings.EqualFold(req.Path, "/search"):
		depth, _ := m["search_depth"].(string)
		if strings.EqualFold(depth, "advanced") {
			return 2
		}
		return 1

	case strings.EqualFold(req.Path, "/extract"):
		advanced := strings.EqualFold(getString(m, "extract_depth"), "advanced")
		urlCount := getURLCount(m)
		if urlCount <= 0 {
			return 1
		}
		perUnit := 1
		if advanced {
			perUnit = 2
		}
		return int(math.Ceil(float64(urlCount)/5.0)) * perUnit

	default:
		// /crawl, /map, and unknown endpoints: conservative default
		return 1
	}
}

func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func getURLCount(m map[string]any) int {
	urls, ok := m["urls"]
	if !ok {
		return 0
	}
	switch v := urls.(type) {
	case []any:
		return len(v)
	case string:
		if v == "" {
			return 0
		}
		return 1
	}
	return 0
}

type usageResponse struct {
	Key struct {
		Usage         int  `json:"usage"`
		Limit         *int `json:"limit"`
		SearchUsage   int  `json:"search_usage"`
		ExtractUsage  int  `json:"extract_usage"`
		CrawlUsage    int  `json:"crawl_usage"`
		MapUsage      int  `json:"map_usage"`
		ResearchUsage int  `json:"research_usage"`
	} `json:"key"`
	Account *struct {
		CurrentPlan   string `json:"current_plan"`
		PlanUsage     int    `json:"plan_usage"`
		PlanLimit     *int   `json:"plan_limit"`
		PaygoUsage    int    `json:"paygo_usage"`
		PaygoLimit    *int   `json:"paygo_limit"`
		SearchUsage   int    `json:"search_usage"`
		ExtractUsage  int    `json:"extract_usage"`
		CrawlUsage    int    `json:"crawl_usage"`
		MapUsage      int    `json:"map_usage"`
		ResearchUsage int    `json:"research_usage"`
	} `json:"account"`
}

type UpstreamStatusError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamStatusError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("upstream status %d", e.StatusCode)
	}
	return fmt.Sprintf("upstream status %d: %s", e.StatusCode, body)
}

func (p *TavilyProxy) isCacheEnabled(ctx context.Context) bool {
	if p.settings == nil {
		return false
	}
	enabled, err := p.settings.GetBool(ctx, SettingCacheEnabled, false)
	if err != nil {
		return false
	}
	return enabled
}

func (p *TavilyProxy) getCacheTTL(ctx context.Context) time.Duration {
	if p.settings == nil {
		return 43200 * time.Second
	}
	seconds, err := p.settings.GetInt(ctx, SettingCacheTTLSeconds, 43200)
	if err != nil || seconds < 60 {
		return 43200 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func (p *TavilyProxy) GetUsage(ctx context.Context, tavilyKey string) (int, *int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/usage", nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tavilyKey)

	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Warn("upstream usage request failed", "err", err)
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, &UpstreamStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var out usageResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, nil, err
	}

	// Prefer account-level stats (official Tavily API) over key-level stats.
	if out.Account != nil {
		usage := out.Account.PlanUsage + out.Account.PaygoUsage
		limit := out.Account.PlanLimit
		if usage == 0 && out.Key.Usage > 0 {
			usage = out.Key.Usage
		}
		if (limit == nil || *limit == 0) && out.Key.Limit != nil && *out.Key.Limit > 0 {
			limit = out.Key.Limit
		}
		// Add PAYGO limit to effective limit if present
		if limit != nil && out.Account.PaygoLimit != nil && *out.Account.PaygoLimit > 0 {
			combined := *limit + *out.Account.PaygoLimit
			limit = &combined
		}
		return usage, limit, nil
	}
	return out.Key.Usage, out.Key.Limit, nil
}
