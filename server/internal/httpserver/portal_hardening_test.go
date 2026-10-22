package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/models"
	"tavily-proxy/server/internal/services"
)

// 门户加 Key 会逐个调用上游校验，必须与公开捐赠接口一样限制请求体大小与频率（按访问密钥计）。
func TestPortalAddKeysRoute_BodyLimitAndRateLimit(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	access := services.NewAccessKeyService(database, logger)
	t.Cleanup(access.Wait)
	proxy := services.NewTavilyProxy("http://127.0.0.1:0", time.Second, keys, nil, nil, logger)
	ak, err := access.Create(context.Background(), "donor")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Dependencies{KeyService: keys, AccessKeyService: access, TavilyProxy: proxy})

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/portal/keys", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+ak.Key)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	// 超大请求体在限流之前被拒绝，不消耗令牌。
	if code := post(`{"keys":["` + strings.Repeat("x", 17*1024) + `"]}`); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: got %d want 413", code)
	}
	// 格式非法的 Key 不会触达上游，用来在不依赖网络的情况下消耗令牌。
	for i := 0; i < 5; i++ {
		if code := post(`{"keys":["bad"]}`); code != http.StatusOK {
			t.Fatalf("request %d within burst: got %d want 200", i+1, code)
		}
	}
	if code := post(`{"keys":["bad"]}`); code != http.StatusTooManyRequests {
		t.Fatalf("request over burst: got %d want 429", code)
	}
}

// 概览口径：额度只统计已启用且未失效的 Key；请求数一次查询给出日志内总数与今日数。
func TestHandlePortalOverview_QuotaAndRequestCounts(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	logs := services.NewLogService(database, logger)
	access := services.NewAccessKeyService(database, logger)
	ctx := context.Background()

	ak, err := access.Create(ctx, "donor")
	if err != nil {
		t.Fatal(err)
	}
	total := 100
	active, _, err := keys.DonateOwnedKey(ctx, "tvly-overview-active-0001", "donor", ak.ID, 10, &total)
	if err != nil || active == nil {
		t.Fatalf("seed active key: %v", err)
	}
	disabled, _, _ := keys.DonateOwnedKey(ctx, "tvly-overview-disabled-001", "donor", ak.ID, 0, &total)
	invalid, _, _ := keys.DonateOwnedKey(ctx, "tvly-overview-invalid-0001", "donor", ak.ID, 0, &total)
	if err := keys.MarkInactive(ctx, disabled.ID); err != nil {
		t.Fatal(err)
	}
	if err := keys.MarkInvalid(ctx, invalid.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for _, entry := range []models.RequestLog{
		{RequestID: "r1", Endpoint: "/search", AccessKeyID: ak.ID, CreatedAt: now},
		{RequestID: "r2", Endpoint: "/search", AccessKeyID: ak.ID, CreatedAt: now},
		{RequestID: "r3", Endpoint: "/search", AccessKeyID: ak.ID, CreatedAt: now.AddDate(0, 0, -2)},
		{RequestID: "r4", Endpoint: "/search", AccessKeyID: ak.ID + 100, CreatedAt: now}, // 他人的请求
	} {
		entry := entry
		if err := logs.Create(ctx, &entry); err != nil {
			t.Fatal(err)
		}
	}

	c, w := portalCtx(http.MethodGet, "/api/portal/overview", "", ak.ID, ak.Alias)
	handlePortalOverview(c, keys, logs)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var out map[string]int64
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		"key_count": 1, "total_quota": 100, "used_quota": 10, "remaining": 90,
		"today_requests": 2, "total_requests": 3,
	}
	for k, v := range want {
		if out[k] != v {
			t.Errorf("%s: got %d want %d (all=%v)", k, out[k], v, out)
		}
	}
}

// 时序接口：粒度非法 → 400；其它（数据库）错误 → 500，不能误报成粒度非法。
func TestRespondTimeSeriesError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"invalid_granularity", services.ErrInvalidGranularity, http.StatusBadRequest},
		{"wrapped_invalid_granularity", errors.Join(errors.New("ctx"), services.ErrInvalidGranularity), http.StatusBadRequest},
		{"database_error", errors.New("database is locked"), http.StatusInternalServerError},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondTimeSeriesError(c, tc.err)
		if w.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, w.Code, tc.want)
		}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	stats := services.NewStatsService(database)
	c, w := portalCtx(http.MethodGet, "/api/portal/timeseries?granularity=bogus", "", 1, "x")
	handlePortalTimeSeries(c, keys, stats)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bogus granularity: got %d want 400", w.Code)
	}
}
